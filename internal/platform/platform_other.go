//go:build !darwin

package platform

import (
	"errors"
	"path/filepath"
	"syscall"
)

var ErrUnsupported = errors.New("not supported on this platform")

func DiskSpace(path string) (total, free uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bsize := uint64(st.Bsize)
	return st.Blocks * bsize, uint64(st.Bavail) * bsize, nil
}

func Reveal(path string) error {
	return runDetached("xdg-open", filepath.Dir(path))
}

func Open(path string) error {
	return runDetached("xdg-open", path)
}

func MoveToTrash(string) error { return ErrUnsupported }
