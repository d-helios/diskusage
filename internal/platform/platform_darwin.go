//go:build darwin

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// DiskSpace returns the total and available bytes of the volume holding path.
func DiskSpace(path string) (total, free uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bsize := uint64(st.Bsize)
	return st.Blocks * bsize, st.Bavail * bsize, nil
}

func Reveal(path string) error {
	return runDetached("open", "-R", path)
}

func Open(path string) error {
	return runDetached("open", path)
}

// MoveToTrash moves path into ~/.Trash.
func MoveToTrash(path string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	trash := filepath.Join(home, ".Trash")
	if err := os.MkdirAll(trash, 0o700); err != nil {
		return err
	}
	dest := filepath.Join(trash, filepath.Base(path))
	if _, err := os.Lstat(dest); err == nil {
		dest = filepath.Join(trash, fmt.Sprintf("%s %s", filepath.Base(path), time.Now().Format("2006-01-02-150405")))
	}
	if err := os.Rename(path, dest); err != nil {
		return fmt.Errorf("%w (cross-volume moves into the Trash are not supported, delete permanently instead)", err)
	}
	return nil
}
