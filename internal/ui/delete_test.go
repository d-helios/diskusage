package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"diskusage/internal/scan"
)

func TestSpaceMarksAndMovesOn(t *testing.T) {
	m := testModel(t)
	press(t, m, " ")
	if !m.marked[child(t, m.root, "big")] {
		t.Fatal("space did not mark the highlighted entry")
	}
	if got := m.selectedNode().Name; got != "mid" {
		t.Errorf("selection = %q, want it to move on to mid", got)
	}
	press(t, m, " ")
	if !m.marked[child(t, m.root, "mid")] {
		t.Error("space did not mark the second entry")
	}
	if count, size := m.markedTotals(); count != 2 || size != 950 {
		t.Errorf("marked = %d entries, %d bytes; want 2 and 950", count, size)
	}

	press(t, m, "up", " ")
	if m.marked[child(t, m.root, "mid")] {
		t.Error("space did not unmark an already marked entry")
	}
	if count, _ := m.markedTotals(); count != 1 {
		t.Errorf("marked = %d, want 1", count)
	}
}

func TestMarkAllAndClear(t *testing.T) {
	m := testModel(t)
	press(t, m, "a")
	if count, size := m.markedTotals(); count != 3 || size != 1000 {
		t.Fatalf("marked = %d entries, %d bytes; want all three for 1000", count, size)
	}
	press(t, m, "a")
	if count, _ := m.markedTotals(); count != 0 {
		t.Errorf("marked = %d, want none after a second `a`", count)
	}

	press(t, m, "a", "c")
	if count, _ := m.markedTotals(); count != 0 {
		t.Errorf("marked = %d, want none after `c`", count)
	}
}

func TestMarkAllSkipsWhatCannotBeDeleted(t *testing.T) {
	m := testModel(t)
	m.root.Children = append(m.root.Children, &scan.Node{Name: "root-again", Path: "/root/root-again", Parent: nil})
	press(t, m, "a")
	for n := range m.marked {
		if m.deleteRefusal(n) != "" {
			t.Errorf("marked %q, which cannot be deleted", n.Name)
		}
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func containsName(nodes []*scan.Node, name string) bool {
	for _, n := range nodes {
		if n.Name == name {
			return true
		}
	}
	return false
}

func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	return lines[len(lines)-1]
}

func TestMarkedRootsDropsNestedEntries(t *testing.T) {
	m := testModel(t)
	big := child(t, m.root, "big")
	inner := child(t, big, "inner")
	m.marked[big] = true
	m.marked[inner] = true
	m.marked[child(t, m.root, "mid")] = true

	roots := m.markedRoots()
	if len(roots) != 2 {
		t.Fatalf("markedRoots = %v, want big and mid only", nodeNames(roots))
	}
	for _, n := range roots {
		if n == inner {
			t.Error("a marked entry inside another marked entry must not be listed")
		}
	}
	if roots[0].Name != "big" {
		t.Errorf("roots[0] = %q, want the biggest first", roots[0].Name)
	}
}

func TestDeleteTargetsPreferTheMarkedEntries(t *testing.T) {
	m := testModel(t)
	m.marked[child(t, m.root, "mid")] = true
	targets := m.deleteTargets()
	if len(targets) != 1 || targets[0].Name != "mid" {
		t.Fatalf("targets = %v, want just the marked mid", nodeNames(targets))
	}

	m.marked = map[*scan.Node]bool{}
	press(t, m, "down") // mid is highlighted
	targets = m.deleteTargets()
	if len(targets) != 1 || targets[0].Name != "mid" {
		t.Fatalf("targets = %v, want the highlighted entry", nodeNames(targets))
	}
}

func TestConfirmShowsEveryMarkedEntry(t *testing.T) {
	m := testModel(t)
	press(t, m, " ", " ") // marks big and mid
	press(t, m, "d")
	if m.mode != modeConfirm {
		t.Fatalf("mode = %v, want a confirmation", m.mode)
	}
	if len(m.pending) != 2 {
		t.Fatalf("pending = %v, want both marked entries", nodeNames(m.pending))
	}
	m.w, m.h = 120, 30
	box := ansi.Strip(m.View())
	for _, want := range []string{"2 entries", "big", "mid", "950 B", "confirm", "cancel"} {
		if !contains(box, want) {
			t.Errorf("the confirmation does not mention %q:\n%s", want, box)
		}
	}
	if contains(box, "/root/big") {
		t.Errorf("the confirmation should name entries relative to the scanned root:\n%s", box)
	}
	press(t, m, "n")
	if len(m.pending) != 0 || m.mode != modeBrowsing {
		t.Error("cancelling the multi-delete did not return to browsing")
	}
	if count, _ := m.markedTotals(); count != 2 {
		t.Errorf("cancelling should keep the selection, got %d", count)
	}
}

func TestDeleteMarkedEntriesOnDisk(t *testing.T) {
	root := scanFor(t, buildFixture(t))
	scan.SortTree(root, scan.SortBySize)

	m := &model{
		opts:     Options{Workers: 4, Apparent: true},
		mode:     modeBrowsing,
		sortMode: scan.SortBySize,
		root:     root,
		path:     []*scan.Node{root},
		marked:   map[*scan.Node]bool{},
		w:        100,
		h:        30,
	}
	sub := child(t, root, "sub")
	file := child(t, root, "a.bin")
	m.marked[sub] = true
	m.marked[file] = true
	m.marked[child(t, sub, "deep")] = true // nested, must be folded into sub

	targets := m.deleteTargets()
	if len(targets) != 2 {
		t.Fatalf("targets = %v, want sub and a.bin", nodeNames(targets))
	}

	raw := deleteCmd(targets, false)()
	msg, ok := raw.(deleteDoneMsg)
	if !ok {
		t.Fatalf("deleteCmd returned %T", raw)
	}
	if msg.err != nil || len(msg.failed) != 0 {
		t.Fatalf("delete failed: err=%v failed=%v", msg.err, nodeNames(msg.failed))
	}
	if len(msg.deleted) != 2 {
		t.Fatalf("deleted = %v, want both", nodeNames(msg.deleted))
	}
	for _, n := range targets {
		if _, err := os.Lstat(n.Path); !os.IsNotExist(err) {
			t.Errorf("%s still exists on disk", n.Path)
		}
	}

	if cmd := m.applyDelete(msg); cmd != nil {
		t.Error("a clean delete needs no follow-up refresh")
	}
	if len(m.marked) != 0 {
		t.Errorf("marks survived the delete: %v", nodeNames(m.markedNodes()))
	}
	// 6503 total, minus sub (5000) and a.bin (1000).
	if root.Size != 503 {
		t.Errorf("root.Size = %d, want 503", root.Size)
	}
	if root.Files != 3 {
		t.Errorf("root.Files = %d, want 3", root.Files)
	}
	if root.Dirs != 2 {
		t.Errorf("root.Dirs = %d, want 2", root.Dirs)
	}
	if !containsName(root.Children, "hl1") || !containsName(root.Children, "link") {
		t.Errorf("the remaining entries are wrong: %v", nodeNames(root.Children))
	}
	if !strings.Contains(m.status, "2 entries") || !strings.Contains(m.status, "freed") {
		t.Errorf("status = %q", m.status)
	}
}

func (m *model) markedNodes() []*scan.Node {
	out := make([]*scan.Node, 0, len(m.marked))
	for n := range m.marked {
		out = append(out, n)
	}
	return out
}

func TestFailedDeleteKeepsWhatSurvivedAndRefreshes(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	locked := dir + "/locked"
	if err := os.MkdirAll(locked+"/inner", 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, locked+"/inner/keep.bin", 2048)
	if err := os.Chmod(locked, 0o500); err != nil { // readable, not writable
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	root := scanFor(t, dir)
	m := &model{
		opts: Options{Workers: 2, Apparent: true}, mode: modeBrowsing, sortMode: scan.SortBySize,
		root: root, path: []*scan.Node{root}, marked: map[*scan.Node]bool{}, w: 100, h: 30,
	}
	node := child(t, root, "locked")

	raw := deleteCmd([]*scan.Node{node}, false)()
	msg := raw.(deleteDoneMsg)
	if len(msg.failed) != 1 || len(msg.deleted) != 0 {
		t.Fatalf("deleted=%v failed=%v, want the delete to fail", nodeNames(msg.deleted), nodeNames(msg.failed))
	}
	if msg.err == nil {
		t.Error("a failed delete should report an error")
	}
	cmd := m.applyDelete(msg)
	if cmd == nil {
		t.Fatal("a failed delete should trigger a refresh of what survived")
	}
	if !m.statusErr {
		t.Errorf("status = %q (err=%v), want an error", m.status, m.statusErr)
	}

	// The refresh must reflect what survived the failed delete.
	refreshMsg, ok := cmd().(refreshDoneMsg)
	if !ok {
		t.Fatalf("the follow-up returned %T", cmd())
	}
	if len(refreshMsg.pairs) != 1 || refreshMsg.pairs[0].fresh == nil {
		t.Fatalf("refresh pairs = %+v", refreshMsg.pairs)
	}
	fresh := refreshMsg.pairs[0].fresh
	if fresh.Size != 0 {
		t.Errorf("refreshed size = %d, want 0 (the file was removed before the failure)", fresh.Size)
	}

	m.Update(refreshMsg)
	if len(m.marked) != 0 {
		t.Errorf("marks survived the refresh: %v", nodeNames(m.markedNodes()))
	}
	got := child(t, m.root, "locked")
	if got == node {
		t.Error("the subtree was not swapped for the freshly scanned one")
	}
	if got.Size != fresh.Size {
		t.Errorf("the stale subtree is still on screen: %d vs %d", got.Size, fresh.Size)
	}
	if m.root.Size != fresh.Size {
		t.Errorf("root.Size = %d, want it to match the refreshed subtree (%d)", m.root.Size, fresh.Size)
	}
}

func TestPruneMarksDropsStaleNodes(t *testing.T) {
	m := testModel(t)
	big := child(t, m.root, "big")
	orphan := &scan.Node{Name: "gone", Path: "/root/gone"}
	m.marked[big] = true
	m.marked[orphan] = true
	m.pruneMarks()
	if len(m.marked) != 1 || !m.marked[big] {
		t.Errorf("marked = %v, want only big", nodeNames(m.markedNodes()))
	}
}

func TestMarksAreShownInTheColumns(t *testing.T) {
	m := testModel(t)
	press(t, m, " ")
	m.w, m.h = 100, 24
	if !strings.Contains(ansi.Strip(m.View()), "●") {
		t.Error("a marked entry is not shown as marked")
	}
	press(t, m, "c")
	if strings.Contains(ansi.Strip(m.View()), "●") {
		t.Error("clearing the selection left a mark behind")
	}
}

func TestFooterSummarisesTheSelection(t *testing.T) {
	m := testModel(t)
	m.w, m.h = 120, 30
	press(t, m, " ")
	footer := ansi.Strip(lastLine(m.View()))
	for _, want := range []string{"1 selected", "700 B", "delete"} {
		if !strings.Contains(footer, want) {
			t.Errorf("footer %q does not mention %q", footer, want)
		}
	}
}

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

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

func scanFor(t *testing.T, path string) *scan.Node {
	t.Helper()
	s := scan.NewScanner(8, false, true)
	root, err := s.Scan(path)
	if err != nil {
		t.Fatalf("scan(%s): %v", path, err)
	}
	return root
}
