package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"diskusage/internal/platform"
	"diskusage/internal/scan"
)

type Options struct {
	Cross    bool
	Apparent bool
	Workers  int
	Trash    bool
}

// Run starts the interactive browser.
func Run(path string, opts Options) error {
	p := tea.NewProgram(newModel(path, opts), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}

type mode int

const (
	modeScanning mode = iota
	modeBrowsing
	modeFiltering
	modeConfirm
	modeDeleting
	modeHelp
)

type scanTickMsg struct{}

type scanDoneMsg struct {
	root *scan.Node
	err  error
}

type deleteDoneMsg struct {
	deleted []*scan.Node // entries that are gone from disk
	failed  []*scan.Node // entries that are still (partly) there
	err     error
	trashed bool
}

type subtreePair struct {
	old   *scan.Node
	fresh *scan.Node
}

type refreshDoneMsg struct {
	pairs []subtreePair
	err   error
}

type model struct {
	opts Options
	path []*scan.Node // path[0] is the scan root, path[len-1] the directory being viewed
	sel  int          // selection inside the active (last) column
	root *scan.Node

	mode      mode
	sortMode  scan.SortMode
	filter    string
	pending   []*scan.Node // entries awaiting delete confirmation
	marked    map[*scan.Node]bool
	status    string
	statusErr bool

	sc       *scan.Scanner
	progress scan.Stats
	spinner  int

	diskTotal uint64
	diskFree  uint64

	w, h   int
	layout layout
}

func newModel(path string, opts Options) *model {
	m := &model{
		opts:     opts,
		mode:     modeScanning,
		sortMode: scan.SortBySize,
		marked:   map[*scan.Node]bool{},
		w:        100,
		h:        30,
	}
	m.refreshDiskSpace()
	m.sc = scan.NewScanner(opts.Workers, opts.Cross, opts.Apparent)
	m.sc.Start(path)
	return m
}

func (m *model) refreshDiskSpace() {
	root := "."
	if len(m.path) > 0 {
		root = m.path[0].Path
	}
	if total, free, err := platform.DiskSpace(root); err == nil {
		m.diskTotal, m.diskFree = total, free
	}
}

func (m *model) Init() tea.Cmd {
	return waitScan(m.sc)
}

// waitScan waits for the scan to finish or a progress tick.
func waitScan(sc *scan.Scanner) tea.Cmd {
	return func() tea.Msg {
		if sc.Finished() {
			return scanDoneMsg{root: sc.Root(), err: sc.Err()}
		}
		time.Sleep(80 * time.Millisecond)
		return scanTickMsg{}
	}
}

// deleteCmd deletes the nodes one by one and reports what survived.
func deleteCmd(nodes []*scan.Node, trash bool) tea.Cmd {
	return func() tea.Msg {
		msg := deleteDoneMsg{trashed: trash}
		for _, n := range nodes {
			var err error
			if trash {
				err = platform.MoveToTrash(n.Path)
			} else {
				err = os.RemoveAll(n.Path)
			}
			if err == nil {
				msg.deleted = append(msg.deleted, n)
				continue
			}
			// Part of the tree may be gone even when the delete failed.
			if _, statErr := os.Lstat(n.Path); errors.Is(statErr, fs.ErrNotExist) {
				msg.deleted = append(msg.deleted, n)
				continue
			}
			msg.failed = append(msg.failed, n)
			if msg.err == nil {
				msg.err = fmt.Errorf("%s: %w", filepath.Base(n.Path), err)
			}
		}
		return msg
	}
}

// refreshCmd re-reads directories after a partial delete.
func refreshCmd(nodes []*scan.Node, opts Options) tea.Cmd {
	return func() tea.Msg {
		var msg refreshDoneMsg
		for _, n := range nodes {
			sc := scan.NewScanner(opts.Workers, opts.Cross, opts.Apparent)
			fresh, err := sc.Scan(n.Path)
			if err != nil {
				if msg.err == nil {
					msg.err = err
				}
				continue
			}
			msg.pairs = append(msg.pairs, subtreePair{old: n, fresh: fresh})
		}
		return msg
	}
}

func rescanCmd(path string, opts Options) tea.Cmd {
	return func() tea.Msg {
		sc := scan.NewScanner(opts.Workers, opts.Cross, opts.Apparent)
		fresh, err := sc.Scan(path)
		return refreshDoneMsg{pairs: []subtreePair{{fresh: fresh}}, err: err}
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		m.handleMouse(msg)
		return m, nil

	case scanTickMsg:
		if m.mode != modeScanning {
			return m, nil
		}
		m.spinner++
		if m.sc == nil {
			return m, nil
		}
		m.progress = m.sc.Stats()
		// waitScan reports scanDoneMsg once the scan is finished.
		return m, waitScan(m.sc)

	case scanDoneMsg:
		if msg.err != nil {
			m.status = "scan failed: " + msg.err.Error()
			m.statusErr = true
			m.mode = modeBrowsing
			return m, nil
		}
		m.root = msg.root
		m.path = []*scan.Node{m.root}
		m.sel = 0
		m.mode = modeBrowsing
		scan.SortTree(m.root, m.sortMode)
		if len(m.root.Children) == 0 {
			m.status = "nothing to show here"
		}
		m.refreshDiskSpace()
		return m, nil

	case deleteDoneMsg:
		return m, m.applyDelete(msg)

	case refreshDoneMsg:
		m.mode = modeBrowsing
		for _, p := range msg.pairs {
			if p.fresh == nil {
				continue
			}
			scan.SortTree(p.fresh, m.sortMode)
			switch {
			case p.old == nil:
				if m.root != nil {
					m.root.Parent = nil
				}
				m.root = p.fresh
				m.path = []*scan.Node{m.root}
				m.sel = 0
			case p.old.Parent == nil:
				m.root = p.fresh
				m.path = []*scan.Node{m.root}
				m.sel = 0
			default:
				scan.ReplaceSubtree(p.old, p.fresh)
			}
		}
		m.clampSel()
		m.pruneMarks()
		m.refreshDiskSpace()
		switch {
		case msg.err != nil:
			m.setError("refresh failed: " + msg.err.Error())
		case len(msg.pairs) > 0:
			m.status = "refreshed"
		}
		return m, nil
	}
	return m, nil
}

func (m *model) applyDelete(msg deleteDoneMsg) tea.Cmd {
	m.mode = modeBrowsing
	var freed int64
	for _, n := range msg.deleted {
		freed += scan.Detach(n)
		delete(m.marked, n)
	}
	m.pending = nil
	m.clampSel()
	m.pruneMarks()
	m.refreshDiskSpace()

	verb := "deleted"
	if msg.trashed {
		verb = "moved to Trash"
	}
	switch {
	case len(msg.deleted) == 1:
		m.status = fmt.Sprintf("%s %s: %s freed", verb, msg.deleted[0].Name, humanBytes(freed))
	case len(msg.deleted) > 1:
		m.status = fmt.Sprintf("%s %d entries: %s freed", verb, len(msg.deleted), humanBytes(freed))
	default:
		m.status = "nothing was deleted"
		m.statusErr = true
	}
	if msg.err != nil {
		m.setError(m.status + " · " + msg.err.Error())
	}
	if len(msg.failed) > 0 {
		return refreshCmd(msg.failed, m.opts)
	}
	return nil
}

func (m *model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.mode == modeScanning {
		switch key {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
		return m, nil
	}

	if key == "ctrl+c" {
		return m, tea.Quit
	}

	switch m.mode {
	case modeConfirm:
		switch key {
		case "y", "Y", "enter":
			nodes := m.pending
			m.pending = nil
			m.mode = modeDeleting
			switch len(nodes) {
			case 0:
				m.mode = modeBrowsing
				return m, nil
			case 1:
				m.status = "deleting " + nodes[0].Name + "…"
			default:
				m.status = fmt.Sprintf("deleting %d entries…", len(nodes))
			}
			m.statusErr = false
			return m, deleteCmd(nodes, m.opts.Trash)
		case "n", "N", "esc", "q":
			m.pending = nil
			m.mode = modeBrowsing
			m.status = "cancelled"
			return m, nil
		}
		return m, nil

	case modeFiltering:
		return m.handleFilterKey(msg)

	case modeHelp:
		switch key {
		case "?", "esc", "q", "enter":
			m.mode = modeBrowsing
		}
		return m, nil
	}

	m.status = ""
	m.statusErr = false

	switch key {
	case "q":
		return m, tea.Quit
	case "?":
		m.mode = modeHelp
	case "up", "k":
		m.moveSel(-1)
	case "down", "j":
		m.moveSel(1)
	case "pgup":
		m.moveSel(-max(1, m.bodyHeight()/2))
	case "pgdown":
		m.moveSel(max(1, m.bodyHeight()/2))
	case "home", "g":
		m.sel = 0
	case "end", "G":
		m.sel = len(m.activeNodes()) - 1
		if m.sel < 0 {
			m.sel = 0
		}
	case "right", "l", "enter":
		m.descend()
	case "left", "h", "backspace":
		m.pop()
	case "o", "O":
		m.openSelected()
	case "R":
		m.revealSelected()
	case " ", "space":
		m.toggleMark()
	case "a":
		m.markAll()
	case "c":
		m.clearMarks()
	case "d", "x", "delete":
		m.askDelete()
	case "s":
		m.sortMode = m.sortMode.Next()
		scan.SortTree(m.root, m.sortMode)
		m.clampSel()
		m.status = "sorted by " + m.sortMode.String()
	case "/":
		m.mode = modeFiltering
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.clampSel()
			m.status = "filter cleared"
		}
	case "r":
		if m.root != nil {
			m.mode = modeScanning
			m.progress = scan.Stats{}
			m.sc = scan.NewScanner(m.opts.Workers, m.opts.Cross, m.opts.Apparent)
			m.sc.Start(m.root.Path)
			m.status = ""
			return m, waitScan(m.sc)
		}
	}
	m.clampSel()
	return m, nil
}

func (m *model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filter = ""
		m.mode = modeBrowsing
	case "enter", "ctrl+c":
		m.mode = modeBrowsing
	case "backspace", "ctrl+h":
		if m.filter != "" {
			m.filter = m.filter[:len(m.filter)-1]
		}
	case "ctrl+u":
		m.filter = ""
	default:
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
			m.filter += string(msg.Runes)
		}
	}
	m.clampSel()
	return m, nil
}

func (m *model) activeNodes() []*scan.Node {
	if len(m.path) == 0 {
		return nil
	}
	return m.childrenOf(m.path[len(m.path)-1])
}

// childrenOf applies the active filter.
func (m *model) childrenOf(n *scan.Node) []*scan.Node {
	if n == nil || !n.IsDir {
		return nil
	}
	if m.filter == "" {
		return n.Children
	}
	q := strings.ToLower(m.filter)
	out := make([]*scan.Node, 0, 16)
	for _, c := range n.Children {
		if strings.Contains(strings.ToLower(c.Name), q) {
			out = append(out, c)
		}
	}
	return out
}

func (m *model) selectedNode() *scan.Node {
	nodes := m.activeNodes()
	if m.sel < 0 || m.sel >= len(nodes) {
		return nil
	}
	return nodes[m.sel]
}

func (m *model) moveSel(delta int) {
	n := len(m.activeNodes())
	if n == 0 {
		m.sel = 0
		return
	}
	m.sel += delta
	if m.sel < 0 {
		m.sel = 0
	}
	if m.sel > n-1 {
		m.sel = n - 1
	}
}

func (m *model) clampSel() {
	n := len(m.activeNodes())
	if n == 0 {
		m.sel = 0
		return
	}
	if m.sel > n-1 {
		m.sel = n - 1
	}
	if m.sel < 0 {
		m.sel = 0
	}
}

func (m *model) descend() {
	n := m.selectedNode()
	if n == nil {
		return
	}
	if !n.IsDir {
		m.revealSelected()
		return
	}
	m.path = append(m.path, n)
	m.sel = 0
}

func (m *model) pop() {
	if len(m.path) <= 1 {
		return
	}
	last := m.path[len(m.path)-1]
	m.path = m.path[:len(m.path)-1]
	m.sel = scan.IndexOf(m.childrenOf(m.path[len(m.path)-1]), last)
	if m.sel < 0 {
		m.sel = 0
	}
}

func (m *model) openSelected() {
	n := m.selectedNode()
	if n == nil {
		return
	}
	var err error
	if n.IsDir {
		err = platform.Open(n.Path)
	} else {
		err = platform.Reveal(n.Path)
	}
	if err != nil {
		m.setError("could not open in Finder: " + err.Error())
		return
	}
	m.status = "opened " + n.Name + " in Finder"
}

func (m *model) revealSelected() {
	n := m.selectedNode()
	if n == nil {
		return
	}
	if err := platform.Reveal(n.Path); err != nil {
		m.setError("could not reveal in Finder: " + err.Error())
		return
	}
	m.status = "revealed " + n.Name + " in Finder"
}

// deleteRefusal reports why n cannot be deleted, or "".
func (m *model) deleteRefusal(n *scan.Node) string {
	switch {
	case n == nil:
		return "nothing is selected"
	case n == m.root || n.Parent == nil:
		return "refusing to delete the directory being scanned"
	case n.Path == "/" || n.Path == "":
		return "refusing to delete " + n.Path
	}
	return ""
}

// askDelete asks for confirmation on the marked entries or the selection.
func (m *model) askDelete() {
	targets := m.deleteTargets()
	if len(targets) == 0 {
		m.setError("nothing is selected")
		return
	}
	for _, n := range targets {
		if why := m.deleteRefusal(n); why != "" {
			m.setError(why)
			return
		}
	}
	m.pending = targets
	m.mode = modeConfirm
	m.status = ""
}

// deleteTargets returns the marked roots, or the selected entry.
func (m *model) deleteTargets() []*scan.Node {
	if roots := m.markedRoots(); len(roots) > 0 {
		return roots
	}
	if n := m.selectedNode(); n != nil {
		return []*scan.Node{n}
	}
	return nil
}

// markedRoots drops marks nested inside another marked entry.
func (m *model) markedRoots() []*scan.Node {
	if len(m.marked) == 0 {
		return nil
	}
	out := make([]*scan.Node, 0, len(m.marked))
	for n := range m.marked {
		nested := false
		for a := n.Parent; a != nil; a = a.Parent {
			if m.marked[a] {
				nested = true
				break
			}
		}
		if !nested {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Size != out[j].Size {
			return out[i].Size > out[j].Size
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func (m *model) markedTotals() (count int, size int64) {
	for n := range m.marked {
		count++
		size += n.Size
	}
	return count, size
}

// toggleMark marks the selected entry and moves on.
func (m *model) toggleMark() {
	n := m.selectedNode()
	if n == nil {
		return
	}
	if why := m.deleteRefusal(n); why != "" {
		m.setError(why)
		return
	}
	if m.marked == nil {
		m.marked = map[*scan.Node]bool{}
	}
	if m.marked[n] {
		delete(m.marked, n)
	} else {
		m.marked[n] = true
	}
	m.moveSel(1)
}

func (m *model) markAll() {
	nodes := m.activeNodes()
	if len(nodes) == 0 {
		return
	}
	if m.marked == nil {
		m.marked = map[*scan.Node]bool{}
	}
	all := true
	for _, n := range nodes {
		if m.deleteRefusal(n) == "" && !m.marked[n] {
			all = false
			break
		}
	}
	for _, n := range nodes {
		if m.deleteRefusal(n) != "" {
			continue
		}
		if all {
			delete(m.marked, n)
		} else {
			m.marked[n] = true
		}
	}
	count, size := m.markedTotals()
	if count == 0 {
		m.status = "nothing selected"
		return
	}
	m.status = fmt.Sprintf("%d entries selected, %s", count, humanBytes(size))
}

func (m *model) clearMarks() {
	if len(m.marked) == 0 {
		return
	}
	m.marked = map[*scan.Node]bool{}
	m.status = "selection cleared"
}

// pruneMarks drops marks on nodes that are no longer in the tree.
func (m *model) pruneMarks() {
	if len(m.marked) == 0 {
		return
	}
	keep := make(map[*scan.Node]bool, len(m.marked))
	var walk func(*scan.Node)
	walk = func(n *scan.Node) {
		if len(keep) == len(m.marked) {
			return
		}
		if m.marked[n] {
			keep[n] = true
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	if m.root != nil {
		walk(m.root)
	}
	m.marked = keep
}

func (m *model) setError(s string) {
	m.status = s
	m.statusErr = true
}

func (m *model) handleMouse(msg tea.MouseMsg) {
	if m.mode != modeBrowsing {
		return
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.moveSel(-3)
		return
	case tea.MouseButtonWheelDown:
		m.moveSel(3)
		return
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return
	}
	l := m.layout
	if len(l.columns) == 0 || l.colWidth <= 0 {
		return
	}
	col := msg.X / l.colWidth
	if col < 0 || col >= len(l.columns) {
		return
	}
	cl := l.columns[col]
	if msg.Y < l.bodyTop || msg.Y >= l.bodyTop+l.bodyHeight || len(cl.nodes) == 0 {
		return
	}
	row := msg.Y - l.bodyTop + cl.offset
	if row < 0 || row >= len(cl.nodes) {
		return
	}
	switch {
	case cl.level > len(m.path)-1:
		m.descend()
		m.sel = row
	case cl.level < len(m.path)-1:
		m.path = m.path[:cl.level+1]
		m.sel = row
	default:
		m.sel = row
		if n := m.selectedNode(); n != nil && n.IsDir {
			m.descend()
		}
	}
	m.clampSel()
}
