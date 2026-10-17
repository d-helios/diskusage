package ui

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"diskspace/internal/scan"
)

// fixture builds a small in-memory tree.
func fixture() *scan.Node {
	root := &scan.Node{Name: "root", Path: "/root", IsDir: true, Size: 1000, Files: 4, Dirs: 5}
	big := &scan.Node{Name: "big", Path: "/root/big", IsDir: true, Parent: root, Size: 700, Files: 2, Dirs: 3}
	inner := &scan.Node{Name: "inner", Path: "/root/big/inner", IsDir: true, Parent: big, Size: 600, Files: 1, Dirs: 1}
	tail := &scan.Node{Name: "tail", Path: "/root/big/tail", IsDir: true, Parent: big, Size: 100, Files: 1, Dirs: 1}
	mid := &scan.Node{Name: "mid", Path: "/root/mid", IsDir: true, Parent: root, Size: 250, Files: 1, Dirs: 1}
	file := &scan.Node{Name: "file", Path: "/root/file", Parent: root, Size: 50, Files: 1}

	big.Children = []*scan.Node{inner, tail}
	root.Children = []*scan.Node{big, mid, file}
	return root
}

func testModel(t *testing.T) *model {
	t.Helper()
	root := fixture()
	scan.SortTree(root, scan.SortBySize)
	return &model{
		opts:     Options{Workers: 2, Apparent: true},
		mode:     modeBrowsing,
		sortMode: scan.SortBySize,
		root:     root,
		path:     []*scan.Node{root},
		sel:      0,
		marked:   map[*scan.Node]bool{},
		w:        100,
		h:        30,
	}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEscape}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func child(t *testing.T, n *scan.Node, name string) *scan.Node {
	t.Helper()
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("%s has no child %q", n.Name, name)
	return nil
}

func press(t *testing.T, m *model, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, cmd := m.Update(key(k)); cmd != nil {
			_ = cmd
		}
	}
}

func TestSortOrder(t *testing.T) {
	root := fixture()
	scan.SortTree(root, scan.SortBySize)
	want := []string{"big", "mid", "file"}
	assertOrder(t, root, want)

	scan.SortTree(root, scan.SortByName)
	assertOrder(t, root, []string{"big", "file", "mid"})

	scan.SortTree(root, scan.SortByCount)
	assertOrder(t, root, []string{"big", "mid", "file"})
}

func assertOrder(t *testing.T, n *scan.Node, want []string) {
	t.Helper()
	if len(n.Children) != len(want) {
		t.Fatalf("got %d children, want %d", len(n.Children), len(want))
	}
	for i, w := range want {
		if n.Children[i].Name != w {
			t.Fatalf("child %d = %q, want %q", i, n.Children[i].Name, w)
		}
	}
}

func TestNavigation(t *testing.T) {
	m := testModel(t)

	if got := m.selectedNode().Name; got != "big" {
		t.Fatalf("initial selection = %q, want big", got)
	}
	press(t, m, "down")
	if got := m.selectedNode().Name; got != "mid" {
		t.Errorf("after down = %q, want mid", got)
	}
	press(t, m, "down", "down", "down") // clamped at the end
	if got := m.selectedNode().Name; got != "file" {
		t.Errorf("after clamping = %q, want file", got)
	}
	press(t, m, "up", "up", "up", "up")
	if got := m.selectedNode().Name; got != "big" {
		t.Errorf("after clamping up = %q, want big", got)
	}

	press(t, m, "right")
	if len(m.path) != 2 || m.path[1].Name != "big" {
		t.Fatalf("path = %v, want to be inside big", pathNames(m))
	}
	if got := m.selectedNode().Name; got != "inner" {
		t.Errorf("inside big the selection = %q, want inner", got)
	}

	press(t, m, "left")
	if len(m.path) != 1 {
		t.Fatalf("path = %v, want back at the root", pathNames(m))
	}
	if got := m.selectedNode().Name; got != "big" {
		t.Errorf("the selection was not restored: got %q, want big", got)
	}

	press(t, m, "enter")
	if len(m.path) != 2 {
		t.Fatalf("enter did not descend: %v", pathNames(m))
	}
	press(t, m, "backspace")
	if len(m.path) != 1 {
		t.Fatalf("backspace did not ascend: %v", pathNames(m))
	}
}

func pathNames(m *model) []string {
	var out []string
	for _, n := range m.path {
		out = append(out, n.Name)
	}
	return out
}

func TestDescendAtRootCannotAscend(t *testing.T) {
	m := testModel(t)
	press(t, m, "left", "left", "h", "backspace")
	if len(m.path) != 1 {
		t.Errorf("path = %v, want stayed at the root", pathNames(m))
	}
}

func TestSortKeyCycles(t *testing.T) {
	m := testModel(t)
	press(t, m, "s")
	if m.sortMode != scan.SortByName {
		t.Errorf("sortMode = %v, want name", m.sortMode)
	}
	press(t, m, "s")
	if m.sortMode != scan.SortByCount {
		t.Errorf("sortMode = %v, want items", m.sortMode)
	}
	press(t, m, "s")
	if m.sortMode != scan.SortBySize {
		t.Errorf("sortMode = %v, want size", m.sortMode)
	}
}

func TestFilter(t *testing.T) {
	m := testModel(t)
	press(t, m, "/", "m", "i", "d", "enter")
	if m.mode != modeBrowsing {
		t.Fatalf("mode = %v, want browsing after enter", m.mode)
	}
	if nodes := m.activeNodes(); len(nodes) != 1 || nodes[0].Name != "mid" {
		t.Fatalf("filtered nodes = %v, want just mid", nodeNames(nodes))
	}
	if got := m.selectedNode().Name; got != "mid" {
		t.Errorf("selection = %q, want mid", got)
	}

	press(t, m, "/", "z", "z", "z", "enter")
	if nodes := m.activeNodes(); len(nodes) != 0 {
		t.Errorf("nodes = %v, want none", nodeNames(nodes))
	}
	if m.selectedNode() != nil {
		t.Error("selection should be nil with no matches")
	}
	press(t, m, "down", "up", "right", "left") // must not panic

	press(t, m, "esc")
	if m.filter != "" {
		t.Errorf("filter = %q, want cleared", m.filter)
	}
	if nodes := m.activeNodes(); len(nodes) != 3 {
		t.Errorf("nodes = %v, want all three back", nodeNames(nodes))
	}
}

func TestFilterBackspaceAndCtrlU(t *testing.T) {
	m := testModel(t)
	press(t, m, "/", "m", "i", "x")
	if m.filter != "mix" {
		t.Fatalf("filter = %q, want mix", m.filter)
	}
	press(t, m, "backspace")
	if m.filter != "mi" {
		t.Fatalf("filter = %q, want mi", m.filter)
	}
	press(t, m, "ctrl+u")
	if m.filter != "" {
		t.Fatalf("filter = %q, want empty", m.filter)
	}
	press(t, m, "esc")
}

func TestDeleteAsksForConfirmationAndCanBeCancelled(t *testing.T) {
	m := testModel(t)
	press(t, m, "down") // mid
	press(t, m, "d")
	if m.mode != modeConfirm || len(m.pending) != 1 || m.pending[0].Name != "mid" {
		t.Fatalf("mode = %v pending = %v, want a confirmation for mid", m.mode, m.pending)
	}
	press(t, m, "n")
	if m.mode != modeBrowsing || m.pending != nil {
		t.Fatalf("cancel left mode = %v pending = %v", m.mode, m.pending)
	}
	if len(m.root.Children) != 3 {
		t.Error("cancelling changed the tree")
	}
}

func TestDeleteRefusesTheScannedRoot(t *testing.T) {
	m := testModel(t)
	if why := m.deleteRefusal(m.root); why == "" {
		t.Fatal("the scan root must not be deletable")
	}
	if why := m.deleteRefusal(&scan.Node{Name: "orphan", Path: "/orphan"}); why == "" {
		t.Fatal("a node without a parent must not be deletable")
	}
	if why := m.deleteRefusal(child(t, m.root, "mid")); why != "" {
		t.Fatalf("a regular entry should be deletable, got %q", why)
	}
}

func TestApplyDeleteKeepsSizesConsistent(t *testing.T) {
	m := testModel(t)
	mid := child(t, m.root, "mid")

	press(t, m, "down")
	// Apply the delete directly; deleteCmd is covered elsewhere.
	m.mode = modeDeleting
	if cmd := m.applyDelete(deleteDoneMsg{deleted: []*scan.Node{mid}}); cmd != nil {
		t.Error("a successful delete should not need a follow-up command")
	}

	if len(m.root.Children) != 2 {
		t.Errorf("children = %d, want 2", len(m.root.Children))
	}
	if m.root.Size != 750 {
		t.Errorf("root.Size = %d, want 750", m.root.Size)
	}
	if m.root.Files != 3 {
		t.Errorf("root.Files = %d, want 3", m.root.Files)
	}
	if m.root.Dirs != 4 {
		t.Errorf("root.Dirs = %d, want 4", m.root.Dirs)
	}
	if m.mode != modeBrowsing {
		t.Errorf("mode = %v, want browsing", m.mode)
	}
	if m.selectedNode() == nil {
		t.Error("the selection should stay usable after a delete")
	}
}

func TestApplyDeleteTracksThePathWhenInside(t *testing.T) {
	m := testModel(t)
	press(t, m, "right", "down") // inside big, on tail
	tail := child(t, child(t, m.root, "big"), "tail")
	m.mode = modeDeleting
	m.applyDelete(deleteDoneMsg{deleted: []*scan.Node{tail}})

	if len(m.path) != 2 {
		t.Fatalf("path = %v, want still inside big", pathNames(m))
	}
	if got := child(t, m.root, "big").Size; got != 600 {
		t.Errorf("big.Size = %d, want 600", got)
	}
	if m.selectedNode() == nil {
		t.Error("selection should fall back to inner")
	}
}

func TestReplaceSubtreeAdjustsAncestors(t *testing.T) {
	m := testModel(t)
	old := child(t, m.root, "big")
	fresh := &scan.Node{Name: "big", Path: old.Path, IsDir: true, Size: 100, Files: 1, Dirs: 5}
	scan.ReplaceSubtree(old, fresh)

	if m.root.Size != 400 { // 1000 - 700 + 100
		t.Errorf("root.Size = %d, want 400", m.root.Size)
	}
	if m.root.Files != 3 { // 4 - 2 + 1
		t.Errorf("root.Files = %d, want 3", m.root.Files)
	}
	if m.root.Dirs != 7 { // 5 - 3 + 5
		t.Errorf("root.Dirs = %d, want 7", m.root.Dirs)
	}
	if got := m.root.Children[0]; got != fresh {
		t.Error("the fresh subtree was not put in place")
	}
	if fresh.Parent != m.root {
		t.Error("the fresh subtree did not get a parent")
	}
}

func TestMouseClickSelectsAndEnters(t *testing.T) {
	m := testModel(t)
	press(t, m, "down") // mid, so the preview shows nothing
	m.View()            // establish the layout
	if m.layout.colWidth == 0 {
		t.Fatal("no layout recorded")
	}
	m.handleMouse(tea.MouseMsg{
		X:      m.layout.colWidth / 2,
		Y:      m.layout.bodyTop + 1,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if len(m.path) != 2 || m.path[1].Name != "mid" {
		t.Fatalf("click did not enter mid: %v", pathNames(m))
	}
}

func TestMouseWheelScrollsSelection(t *testing.T) {
	m := testModel(t)
	m.root.Children = append(m.root.Children, &scan.Node{Name: "aaa", Path: "/root/aaa", Parent: m.root, Size: 1, Files: 1})
	m.handleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	if m.sel != 3 { // clamped at the last of the four entries
		t.Errorf("sel = %d, want 3 after a wheel down", m.sel)
	}
	m.handleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	if m.sel != 0 {
		t.Errorf("sel = %d, want 0 after a wheel up", m.sel)
	}
}

func TestMouseIsIgnoredWhileConfirming(t *testing.T) {
	m := testModel(t)
	press(t, m, "d")
	before := m.sel
	m.handleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	if m.sel != before {
		t.Error("the wheel moved the selection behind the confirmation")
	}
	if m.mode != modeConfirm {
		t.Error("the confirmation was dismissed")
	}
}

func TestHelpModeOpensAndCloses(t *testing.T) {
	m := testModel(t)
	press(t, m, "?")
	if m.mode != modeHelp {
		t.Fatalf("mode = %v, want help", m.mode)
	}
	press(t, m, "esc")
	if m.mode != modeBrowsing {
		t.Fatalf("mode = %v, want browsing", m.mode)
	}
}

func TestScanDoneBuildsTheTree(t *testing.T) {
	root := fixture()
	m := &model{mode: modeScanning, w: 100, h: 30, sc: scan.NewScanner(2, false, true)}
	m.Update(scanDoneMsg{root: root})
	if m.mode != modeBrowsing {
		t.Fatalf("mode = %v, want browsing", m.mode)
	}
	if len(m.path) != 1 || m.path[0] != root {
		t.Fatal("the root was not installed as the path")
	}
	if got := m.selectedNode().Name; got != "big" {
		t.Errorf("selection = %q, want the biggest entry, big", got)
	}
}

func TestScanFailureIsReported(t *testing.T) {
	m := &model{mode: modeScanning, w: 100, h: 30, sc: scan.NewScanner(2, false, true)}
	m.Update(scanDoneMsg{err: errors.New("boom")})
	if m.mode != modeBrowsing || !m.statusErr {
		t.Fatalf("mode = %v statusErr = %v, want a reported failure", m.mode, m.statusErr)
	}
}

func nodeNames(nodes []*scan.Node) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, n.Name)
	}
	return out
}

func TestViewFitsEveryTerminal(t *testing.T) {
	sizes := []struct{ w, h int }{{20, 6}, {30, 10}, {40, 12}, {60, 20}, {100, 30}, {120, 40}, {200, 50}, {26, 8}, {27, 7}}
	situations := []struct {
		name    string
		prepare func(m *model)
	}{
		{"browsing", func(m *model) {}},
		{"deep", func(m *model) { press(t, m, "right", "right") }},
		{"filtered", func(m *model) { press(t, m, "/", "i", "n", "n", "enter") }},
		{"no matches", func(m *model) { press(t, m, "/", "q", "q", "q", "enter", "esc") }},
		{"confirm", func(m *model) { press(t, m, "d") }},
		{"help", func(m *model) { press(t, m, "?") }},
		{"filtering", func(m *model) { press(t, m, "/") }},
		{"empty root", func(m *model) {
			m.root.Children = nil
			m.root.Size = 0
			m.path = []*scan.Node{m.root}
			m.sel = 0
		}},
		{"root is a file", func(m *model) {
			f := &scan.Node{Name: "one.bin", Path: "/one.bin", Size: 10, Files: 1}
			m.root = f
			m.path = []*scan.Node{f}
			m.sel = 0
		}},
		{"long names", func(m *model) {
			m.root.Children = []*scan.Node{
				{Name: strings.Repeat("long", 20), Path: "/root/long", Parent: m.root, Size: 5, Files: 1},
				{Name: "日本語のとても長い名前", Path: "/root/jp", Parent: m.root, IsDir: true, Size: 4, Dirs: 1},
			}
			m.sel = 0
		}},
	}

	for _, sit := range situations {
		for _, sz := range sizes {
			m := testModel(t)
			sit.prepare(m)
			m.w, m.h = sz.w, sz.h
			view := m.View()
			lines := strings.Split(view, "\n")
			if len(lines) != sz.h {
				t.Errorf("%s at %dx%d: %d lines, want %d", sit.name, sz.w, sz.h, len(lines), sz.h)
			}
			for i, l := range lines {
				if got := ansi.StringWidth(l); got > sz.w {
					t.Errorf("%s at %dx%d: line %d is %d cells wide, want <= %d: %q",
						sit.name, sz.w, sz.h, i, got, sz.w, ansi.Strip(l))
				}
			}
		}
	}
}

func TestViewScanningFits(t *testing.T) {
	m := &model{
		mode:     modeScanning,
		w:        80,
		h:        24,
		spinner:  3,
		progress: scan.Stats{Files: 1200, Dirs: 40, Bytes: 1 << 30, Errs: 2, Cur: "/very/long/path/that/needs/folding/into/the/line"},
		sc:       scan.NewScanner(2, false, true),
	}
	lines := strings.Split(m.View(), "\n")
	if len(lines) != 24 {
		t.Fatalf("%d lines, want 24", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > 80 {
			t.Errorf("line %d is %d cells wide: %q", i, w, ansi.Strip(l))
		}
	}
}

func TestSkippedMountPointIsMarked(t *testing.T) {
	m := testModel(t)
	m.root.Children = []*scan.Node{
		{Name: "aaa", Path: "/root/aaa", Size: 10, Files: 1, Parent: m.root},
		{Name: "Volumes", Path: "/root/Volumes", IsDir: true, Parent: m.root, Dirs: 1, Skipped: true},
	}
	m.sel = 0
	m.w, m.h = 100, 20
	lines := strings.Split(ansi.Strip(m.View()), "\n")

	if row := lines[3]; !strings.Contains(row, "▸") || !strings.Contains(row, "aaa") {
		t.Errorf("the selection caret is missing: %q", row)
	}
	if row := lines[4]; !strings.Contains(row, "⊘") || !strings.Contains(row, "other fs") {
		t.Errorf("a mount point should say it was not scanned: %q", row)
	}
}

func TestRowGeometryIsExact(t *testing.T) {
	names := []string{"a", "big", "日本語のとても長い名前", "trailing.dots.", strings.Repeat("x", 60)}
	sizes := []int64{0, 1, 999, 1536, 1 << 20, 1 << 34}
	widths := []int{17, 20, 26, 32, 39}
	states := []rowStyle{{}, {selected: true}, {preview: true}, {marked: true}, {selected: true, marked: true}}

	for _, w := range widths {
		for _, n := range names {
			for _, size := range sizes {
				for _, st := range states {
					node := &scan.Node{Name: n, Path: "/x/" + n, Size: size, Files: 1}
					row := renderRow(node, 4096, w, st)
					plain := ansi.Strip(row)
					if got := ansi.StringWidth(plain); got != w {
						t.Fatalf("width %d, size %d, state %+v: row is %d cells, want %d: %q",
							w, size, st, got, w, plain)
					}
					if w >= 21 && !strings.Contains(plain, "%") {
						t.Fatalf("width %d, state %+v: the percentage was cut off: %q", w, st, plain)
					}
				}
			}
		}
	}
}

func TestSelectedRowKeepsItsBarColour(t *testing.T) {
	// Only the caret and the selected text change style.
	n := &scan.Node{Name: "big", Path: "/root/big", IsDir: true, Size: 700, Files: 1}
	selected := renderRow(n, 1000, 32, rowStyle{selected: true})
	idle := renderRow(n, 1000, 32, rowStyle{})

	bg := regexp.MustCompile(`48;5;(\d+)`)
	selBg := bg.FindStringSubmatch(selected)
	idleBg := bg.FindStringSubmatch(idle)
	if len(selBg) < 2 || len(idleBg) < 2 {
		t.Fatalf("expected a bar background on both rows: %q / %q", selected, idle)
	}
	if selBg[1] != idleBg[1] {
		t.Errorf("the bar colour changes with the selection: %s vs %s", selBg[1], idleBg[1])
	}
	if strings.Contains(selected, "48;5;111") {
		t.Error("the selected row is still painted with the old full-row blue background")
	}
	if !regexp.MustCompile(`\x1b\[1[;\[]`).MatchString(selected) {
		t.Errorf("the selected row should emphasise its text: %q", selected)
	}
	if got := ansi.Strip(selected); !strings.HasPrefix(got, "▸") {
		t.Errorf("the selected row lost its caret: %q", got)
	}
	if got := ansi.Strip(idle); !strings.HasPrefix(got, "▾") {
		t.Errorf("an idle directory row should keep its own marker: %q", got)
	}
}

func TestMarkedRowShowsADot(t *testing.T) {
	n := &scan.Node{Name: "mid", Path: "/root/mid", IsDir: true, Size: 250, Files: 1}
	row := ansi.Strip(renderRow(n, 1000, 32, rowStyle{marked: true}))
	if !strings.Contains(row, "●") {
		t.Errorf("a marked row should show a dot: %q", row)
	}
	plain := ansi.Strip(renderRow(n, 1000, 32, rowStyle{}))
	if strings.Contains(plain, "●") {
		t.Errorf("an unmarked row should not show a dot: %q", plain)
	}
}

// TestOnlyTheActiveColumnCarriesTheCaret checks the caret stays in one column.
func TestOnlyTheActiveColumnCarriesTheCaret(t *testing.T) {
	root := &scan.Node{Name: "root", Path: "/root", IsDir: true, Size: 100, Files: 1, Dirs: 4}
	d1 := &scan.Node{Name: "d1", Path: "/root/d1", IsDir: true, Parent: root, Size: 100, Files: 1, Dirs: 3}
	d2 := &scan.Node{Name: "d2", Path: "/root/d1/d2", IsDir: true, Parent: d1, Size: 100, Files: 1, Dirs: 2}
	leaf := &scan.Node{Name: "leaf", Path: "/root/d1/d2/leaf", Parent: d2, Size: 100, Files: 1}
	other := &scan.Node{Name: "other", Path: "/root/d1/d2/other", Parent: d2, Size: 10, Files: 1}
	d2.Children = []*scan.Node{leaf, other}
	d1.Children = []*scan.Node{d2}
	root.Children = []*scan.Node{d1}

	m := &model{
		opts: Options{Workers: 2, Apparent: true}, mode: modeBrowsing, sortMode: scan.SortBySize,
		root: root, path: []*scan.Node{root}, marked: map[*scan.Node]bool{}, w: 120, h: 24,
	}
	press(t, m, "right", "right")
	if len(m.path) != 3 {
		t.Fatalf("path = %v, want to be two levels in", pathNames(m))
	}

	lines := strings.Split(ansi.Strip(m.View()), "\n")
	body := lines[3 : len(lines)-2]
	colW := m.colWidth()

	// cellPositions returns the cell offset of every marker on a line.
	cellPositions := func(line, glyph string) []int {
		var out []int
		for off := 0; ; {
			i := strings.Index(line[off:], glyph)
			if i < 0 {
				break
			}
			pos := off + i
			out = append(out, ansi.StringWidth(line[:pos]))
			off = pos + len(glyph)
		}
		return out
	}

	carets := map[int]int{} // column index -> markers found
	paths := map[int]int{}
	for _, line := range body {
		for _, pos := range cellPositions(line, "▸") {
			carets[pos/colW]++
		}
		for _, pos := range cellPositions(line, "›") {
			paths[pos/colW]++
		}
	}

	if len(carets) != 1 || carets[2] != 1 {
		t.Errorf("the caret should be in the active column (2) exactly once, got %v:\n%s",
			carets, strings.Join(body, "\n"))
	}
	if paths[0] != 1 || paths[1] != 1 {
		t.Errorf("both ancestor columns should point at the path entry, got %v:\n%s",
			paths, strings.Join(body, "\n"))
	}
	if paths[2] != 0 || carets[0] != 0 || carets[1] != 0 {
		t.Errorf("markers leaked between columns: carets=%v paths=%v", carets, paths)
	}
}

func TestActiveColumnTitleIsTheHighlightedOne(t *testing.T) {
	m := testModel(t)
	m.w, m.h = 120, 24
	press(t, m, "right") // inside big

	active := 0
	for _, c := range m.columns() {
		if c.active {
			active++
		}
	}
	if active != 1 {
		t.Fatalf("%d columns think they are active, want exactly one", active)
	}

	titles := m.columnTitles(m.columns(), m.colWidth()-1)
	style := activeTitleEscape()
	if !strings.Contains(titles, style) {
		t.Errorf("no title uses the active background %q: %q", style, titles)
	}
	if got := strings.Count(titles, style); got != 1 {
		t.Errorf("%d titles use the active background, want 1: %q", got, titles)
	}
	if !strings.Contains(ansi.Strip(titles), "big") {
		t.Errorf("the active column should be titled after the directory being viewed: %q", ansi.Strip(titles))
	}
}

// activeTitleEscape is the escape sequence of the active title background.
func activeTitleEscape() string {
	return "\x1b[1;38;5;231;48;5;60m"
}

// deepTree builds root → a → b → c, with files inside c.
func deepTree(deepFiles int) *scan.Node {
	root := &scan.Node{Name: "root", Path: "/root", IsDir: true}
	a := &scan.Node{Name: "a", Path: "/root/a", IsDir: true, Parent: root}
	b := &scan.Node{Name: "b", Path: "/root/a/b", IsDir: true, Parent: a}
	c := &scan.Node{Name: "c", Path: "/root/a/b/c", IsDir: true, Parent: b}
	for i := 0; i < deepFiles; i++ {
		name := fmt.Sprintf("file%02d.bin", i)
		c.Children = append(c.Children, &scan.Node{
			Name: name, Path: "/root/a/b/c/" + name, Parent: c, Files: 1, Size: int64(100 - i),
		})
	}
	var size, files int64
	for _, n := range c.Children {
		size += n.Size
		files += n.Files
	}
	c.Size, c.Files, c.Dirs = size, files, 1
	b.Size, b.Files, b.Dirs = size, files, 2
	a.Size, a.Files, a.Dirs = size, files, 3
	root.Size, root.Files, root.Dirs = size, files, 4
	a.Children = []*scan.Node{b}
	b.Children = []*scan.Node{c}
	root.Children = []*scan.Node{a}
	return root
}

// TestColumnsStayAlignedWhenAncestorsAreShort checks short columns do not shift the rest.
func TestColumnsStayAlignedWhenAncestorsAreShort(t *testing.T) {
	for _, size := range []struct{ w, h int }{{120, 24}, {100, 24}, {200, 40}, {80, 20}} {
		m := &model{
			opts: Options{Workers: 2, Apparent: true}, mode: modeBrowsing, sortMode: scan.SortBySize,
			root: deepTree(30), marked: map[*scan.Node]bool{}, w: size.w, h: size.h,
		}
		m.path = []*scan.Node{m.root}
		press(t, m, "right", "right", "right")
		if len(m.path) != 4 {
			t.Fatalf("path = %v, want to be inside c", pathNames(m))
		}

		body := bodyRows(t, m)
		colW := m.colWidth()
		active := activeColumnIndex(m)
		if active < 0 {
			t.Fatalf("%dx%d: the active column is not on screen", size.w, size.h)
		}
		start := active * colW

		for i, line := range body {
			if got := ansi.StringWidth(line); got > colW*(active+1) {
				t.Fatalf("%dx%d row %d is %d cells, wider than its columns allow: %q",
					size.w, size.h, i, got, line)
			}
			if i == 0 {
				continue
			}
			if strings.TrimSpace(ansi.Truncate(line, start, "")) != "" {
				t.Fatalf("%dx%d row %d has content before the active column at %d: %q",
					size.w, size.h, i, start, line)
			}
		}

		caret := -1
		for _, line := range body {
			if j := strings.Index(line, "▸"); j >= 0 {
				caret = ansi.StringWidth(line[:j])
				if caret != start {
					t.Errorf("%dx%d: the caret is at cell %d, but the active column starts at %d: %q",
						size.w, size.h, caret, start, line)
				}
				break
			}
		}
		if caret < 0 {
			t.Errorf("%dx%d: no caret drawn", size.w, size.h)
		}
	}
}

// activeColumnIndex is the position of the browsed column in the window.
func activeColumnIndex(m *model) int {
	for i, cl := range m.layout.columns {
		if cl.level == len(m.path)-1 {
			return i
		}
	}
	return -1
}

func bodyRows(t *testing.T, m *model) []string {
	t.Helper()
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if len(lines) < 5 {
		t.Fatalf("view has only %d lines", len(lines))
	}
	return lines[3 : len(lines)-2]
}

func TestViewShowsTheSpaceHogsWithBars(t *testing.T) {
	m := testModel(t)
	m.w, m.h = 100, 24
	view := ansi.Strip(m.View())
	body := bodyRegion(t, m)

	for _, want := range []string{"big", "mid", "file"} {
		if !strings.Contains(body, want) {
			t.Errorf("the column does not list %q:\n%s", want, body)
		}
	}
	if !strings.Contains(body, "70%") {
		t.Errorf("expected the share of the parent directory to be shown:\n%s", body)
	}
	if !strings.Contains(body, "100 B") || !strings.Contains(body, "600 B") {
		t.Errorf("expected human readable sizes of the entries and the preview:\n%s", body)
	}
	if !strings.Contains(view, "1.00 KiB") && !strings.Contains(view, "1000 B") {
		t.Errorf("the header should report the scanned total:\n%s", view)
	}
}

// bodyRegion returns the column area without header and footer.
func bodyRegion(t *testing.T, m *model) string {
	t.Helper()
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if len(lines) < 6 {
		t.Fatalf("view has only %d lines", len(lines))
	}
	return strings.Join(lines[2:len(lines)-2], "\n")
}

func TestViewHeaderReportsTotals(t *testing.T) {
	m := testModel(t)
	m.w, m.h = 120, 30
	header := ansi.Strip(strings.Split(m.View(), "\n")[0])
	if !strings.Contains(header, "/root") {
		t.Errorf("header %q does not show the current directory", header)
	}
	if !strings.Contains(header, "4 files") || !strings.Contains(header, "5 dirs") {
		t.Errorf("header %q does not report the entry counts", header)
	}
	if !strings.Contains(header, "1000 B") && !strings.Contains(header, "1.00 KiB") {
		t.Errorf("header %q does not mention the total size", header)
	}
}

func TestViewReflectsSortAndFilter(t *testing.T) {
	m := testModel(t)
	m.w, m.h = 120, 30
	press(t, m, "s") // by name
	body := bodyRegion(t, m)
	if strings.Index(body, "big") > strings.Index(body, "mid") {
		t.Error("sorting by name did not reorder the column")
	}

	press(t, m, "/", "m", "i", "d", "enter")
	body = bodyRegion(t, m)
	if strings.Contains(body, "file") {
		t.Errorf("the filter did not hide file:\n%s", body)
	}
	if !strings.Contains(body, "mid") {
		t.Errorf("the filter hid too much:\n%s", body)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0:          "0 B",
		999:        "999 B",
		1024:       "1.00 KiB",
		1536:       "1.50 KiB",
		10 * 1024:  "10.0 KiB",
		999 * 1024: "999 KiB",
		1 << 30:    "1.00 GiB",
		1 << 40:    "1.00 TiB",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanCount(t *testing.T) {
	cases := map[int64]string{0: "0", 12: "12", 1234: "1,234", 1234567: "1,234,567"}
	for in, want := range cases {
		if got := humanCount(in); got != want {
			t.Errorf("humanCount(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestPercentInt(t *testing.T) {
	cases := []struct {
		part, whole int64
		want        int
	}{
		{0, 0, 0},
		{1, 3, 33},
		{2, 3, 67},
		{5, 5, 100},
		{7, 5, 100},
		{-1, 5, 0},
		{0, 100, 0},
	}
	for _, c := range cases {
		if got := percentInt(c.part, c.whole); got != c.want {
			t.Errorf("percentInt(%d, %d) = %d, want %d", c.part, c.whole, got, c.want)
		}
	}
}

func TestTruncateAndPad(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("hello world", 8); !strings.HasSuffix(got, "…") || ansi.StringWidth(got) > 8 {
		t.Errorf("truncate = %q", got)
	}
	if got := pad("ab", 5); got != "ab   " {
		t.Errorf("pad = %q", got)
	}
	if got := pad("abcdef", 3); ansi.StringWidth(got) > 3 {
		t.Errorf("pad = %q, too wide", got)
	}
}

func TestFoldPathKeepsTheTail(t *testing.T) {
	long := "/Users/someone/with/a/very/long/path/to/some/directory/deep/inside"
	got := foldPath(long, 30)
	if len(got) > 30 {
		t.Errorf("foldPath = %q, %d chars, want <= 30", got, len(got))
	}
	if !strings.Contains(got, "inside") {
		t.Errorf("foldPath = %q, want the tail preserved", got)
	}
	if got := foldPath("/short", 30); got != "/short" {
		t.Errorf("foldPath = %q, want unchanged", got)
	}
}
