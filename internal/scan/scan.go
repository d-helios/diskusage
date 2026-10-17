package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
)

// Scanner walks a directory tree in parallel and builds a Node tree.
type Scanner struct {
	sem        chan struct{}
	allowCross bool
	apparent   bool
	crossDev   uint64

	seen  sync.Map
	files atomic.Int64
	dirs  atomic.Int64
	bytes atomic.Int64
	errs  atomic.Int64
	cur   atomic.Pointer[string]

	done chan struct{}
	root *Node
	err  error
}

type hardlinkKey struct {
	dev uint64
	ino uint64
}

func NewScanner(workers int, allowCross, apparent bool) *Scanner {
	if workers < 1 {
		workers = 1
	}
	return &Scanner{
		sem:        make(chan struct{}, workers),
		allowCross: allowCross,
		apparent:   apparent,
		done:       make(chan struct{}),
	}
}

func (s *Scanner) Finished() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

type Stats struct {
	Files int64
	Dirs  int64
	Bytes int64
	Errs  int64
	Cur   string
}

func (s *Scanner) Stats() Stats {
	var cur string
	if p := s.cur.Load(); p != nil {
		cur = *p
	}
	return Stats{
		Files: s.files.Load(),
		Dirs:  s.dirs.Load(),
		Bytes: s.bytes.Load(),
		Errs:  s.errs.Load(),
		Cur:   cur,
	}
}

// Start scans path in the background. Use Stats for progress and Finished to
// wait for the result.
func (s *Scanner) Start(path string) {
	go func() {
		defer close(s.done)
		s.root, s.err = s.Scan(path)
	}()
}

func (s *Scanner) Root() *Node { return s.root }
func (s *Scanner) Err() error  { return s.err }

func (s *Scanner) Scan(path string) (*Node, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	if path == string(filepath.Separator) {
		name = path
	}
	n := &Node{Name: name, Path: path, IsDir: info.IsDir()}
	s.crossDev = devOf(info)

	if !info.IsDir() {
		n.Size = s.fileSize(info)
		n.Files = 1
		s.files.Add(1)
		s.bytes.Add(n.Size)
		return n, nil
	}

	s.sem <- struct{}{}
	s.scanDir(n, devOf(info), ownSize(info, s.apparent))
	return n, nil
}

func (s *Scanner) scanDir(n *Node, dev uint64, own int64) {
	entries, err := os.ReadDir(n.Path)
	<-s.sem

	if d := s.dirs.Add(1); d%64 == 1 || err != nil {
		p := n.Path
		s.cur.Store(&p)
	}
	if err != nil {
		s.errs.Add(1)
		n.Errors++
		n.Dirs = 1
		n.Size = own
		return
	}

	var wg sync.WaitGroup
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			s.errs.Add(1)
			n.Errors++
			continue
		}
		childPath := filepath.Join(n.Path, e.Name())

		if info.IsDir() {
			c := &Node{Name: e.Name(), Path: childPath, IsDir: true, Parent: n}
			n.Children = append(n.Children, c)
			if !s.allowCross && devOf(info) != s.crossDev {
				c.Dirs = 1
				c.Skipped = true
				c.Size = ownSize(info, s.apparent)
				continue
			}
			s.sem <- struct{}{}
			wg.Add(1)
			go func(c *Node, own int64) {
				defer wg.Done()
				s.scanDir(c, devOf(info), own)
			}(c, ownSize(info, s.apparent))
			continue
		}

		c := &Node{Name: e.Name(), Path: childPath, Parent: n}
		c.Size = s.fileSize(info)
		c.Files = 1
		s.files.Add(1)
		s.bytes.Add(c.Size)
		n.Children = append(n.Children, c)
	}
	wg.Wait()

	var size, files, dirs, errs int64
	for _, c := range n.Children {
		size += c.Size
		files += c.Files
		dirs += c.Dirs
		errs += c.Errors
	}
	n.Size = size + own
	n.Files = files
	n.Dirs = dirs + 1
	n.Errors += errs
}

// fileSize returns the space info occupies, counting hard links once.
func (s *Scanner) fileSize(info fs.FileInfo) int64 {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return info.Size()
	}
	if st.Nlink > 1 {
		key := hardlinkKey{dev: uint64(st.Dev), ino: uint64(st.Ino)}
		if _, loaded := s.seen.LoadOrStore(key, struct{}{}); loaded {
			return 0
		}
	}
	if s.apparent {
		return info.Size()
	}
	if st.Blocks > 0 {
		return st.Blocks * 512
	}
	return info.Size()
}

func ownSize(info fs.FileInfo, apparent bool) int64 {
	if apparent {
		return 0
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Blocks > 0 {
		return st.Blocks * 512
	}
	return 0
}

func devOf(info fs.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Dev)
	}
	return 0
}
