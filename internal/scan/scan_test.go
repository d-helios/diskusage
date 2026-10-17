package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

// buildFixture creates files, a symlink, a hard link pair, and an empty dir.
func buildFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.bin"), 1000)
	writeFile(t, filepath.Join(root, "sub", "b.bin"), 2000)
	writeFile(t, filepath.Join(root, "sub", "deep", "c.bin"), 3000)
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("sub", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "hl1"), 500)
	if err := os.Link(filepath.Join(root, "hl1"), filepath.Join(root, "hl2")); err != nil {
		t.Fatal(err)
	}
	return root
}

func scanFor(t *testing.T, path string) *Node {
	t.Helper()
	s := NewScanner(8, false, true)
	root, err := s.Scan(path)
	if err != nil {
		t.Fatalf("scan(%s): %v", path, err)
	}
	return root
}

func child(t *testing.T, n *Node, name string) *Node {
	t.Helper()
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no child %q under %s", name, n.Path)
	return nil
}

func TestScanAggregatesSizesAndCounts(t *testing.T) {
	root := scanFor(t, buildFixture(t))

	// 3 real files, the symlink, and both hard links.
	if root.Files != 6 {
		t.Errorf("Files = %d, want 6", root.Files)
	}
	// root, sub, sub/deep, empty
	if root.Dirs != 4 {
		t.Errorf("Dirs = %d, want 4", root.Dirs)
	}
	want := int64(1000 + 2000 + 3000 + len("sub") + 500)
	if root.Size != want {
		t.Errorf("Size = %d, want %d", root.Size, want)
	}

	sub := child(t, root, "sub")
	if sub.Size != 5000 {
		t.Errorf("sub.Size = %d, want 5000", sub.Size)
	}
	if sub.Files != 2 || sub.Dirs != 2 {
		t.Errorf("sub files/dirs = %d/%d, want 2/2", sub.Files, sub.Dirs)
	}
	deep := child(t, sub, "deep")
	if deep.Size != 3000 {
		t.Errorf("deep.Size = %d, want 3000", deep.Size)
	}
	if e := child(t, root, "empty"); e.Size != 0 || e.Files != 0 || e.Dirs != 1 {
		t.Errorf("empty = %+v, want a zero-size directory", e)
	}

	// The symlink must not pull in sub twice.
	size := int64(0)
	for _, c := range root.Children {
		size += c.Size
	}
	if size != root.Size {
		t.Errorf("children sum to %d, root says %d", size, root.Size)
	}
}

func TestScanDoesNotFollowSymlinkedDirectories(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	writeFile(t, filepath.Join(target, "big.bin"), 4096)
	linkRoot := filepath.Join(base, "linked")
	if err := os.MkdirAll(linkRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(linkRoot, "loop")); err != nil {
		t.Fatal(err)
	}

	n := scanFor(t, linkRoot)
	if n.Size >= 4096 {
		t.Errorf("Size = %d, the symlink was followed", n.Size)
	}
	if n.Files != 1 { // the symlink entry itself
		t.Errorf("Files = %d, want 1", n.Files)
	}
}

func TestScanSingleFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "one.bin")
	writeFile(t, p, 1234)

	n := scanFor(t, p)
	if n.IsDir {
		t.Error("IsDir = true for a file")
	}
	if n.Size != 1234 {
		t.Errorf("Size = %d, want 1234", n.Size)
	}
	if n.Files != 1 || n.Dirs != 0 || len(n.Children) != 0 {
		t.Errorf("counts = %d/%d, children=%d", n.Files, n.Dirs, len(n.Children))
	}
}

func TestScanMissingPath(t *testing.T) {
	s := NewScanner(2, false, true)
	if _, err := s.Scan(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected an error for a missing path")
	}
}

func TestScanUnreadableDirectoryIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ok.bin"), 10)
	locked := filepath.Join(root, "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(locked, "hidden.bin"), 99)
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	n := scanFor(t, root)
	if n.Errors == 0 {
		t.Error("Errors = 0, want at least one for the unreadable directory")
	}
	if got := child(t, n, "locked"); got.Errors == 0 {
		t.Error("the unreadable directory did not record its error")
	}
}

func TestScanDoesNotCrossFilesystemsByDefault(t *testing.T) {
	// The fixture is on one filesystem; this only checks the flag is accepted.
	root := NewScanner(2, true, true)
	n, err := root.Scan(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if n == nil {
		t.Fatal("nil root")
	}
}
