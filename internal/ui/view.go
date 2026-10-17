package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"diskusage/internal/scan"
)

var (
	styleTitleBar  = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("236"))
	styleTitleName = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("62"))
	styleDivider   = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	styleColTitleActive   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("60"))
	styleColTitleInactive = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("238"))
	styleColTitlePreview  = lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Background(lipgloss.Color("236"))

	styleRowText = lipgloss.NewStyle()
	styleRowDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	styleBarLow  = lipgloss.NewStyle().Background(lipgloss.Color("28")).Foreground(lipgloss.Color("120"))
	styleBarMid  = lipgloss.NewStyle().Background(lipgloss.Color("94")).Foreground(lipgloss.Color("222"))
	styleBarHigh = lipgloss.NewStyle().Background(lipgloss.Color("88")).Foreground(lipgloss.Color("217"))
	styleSelText = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231"))

	styleCaret     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	styleCaretIdle = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	styleMarked    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))

	styleKey       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("110"))
	styleHint      = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	styleOK        = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	styleBad       = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	styleWarn      = lipgloss.NewStyle().Foreground(lipgloss.Color("222"))
	styleWarnOnBar = lipgloss.NewStyle().Foreground(lipgloss.Color("222")).Background(lipgloss.Color("236")).Bold(true)
	styleBox       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("203")).Padding(0, 2)
	styleHelpBox   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("60")).Padding(0, 2)
)

// layout maps mouse events back to rows and columns.
type layout struct {
	colWidth   int
	bodyTop    int // first screen line that holds selectable rows
	bodyHeight int // how many of those rows there are
	columns    []colLayout
}

type colLayout struct {
	level  int
	offset int
	nodes  []*scan.Node
}

// column is one Miller column.
type column struct {
	title   string
	nodes   []*scan.Node
	sel     int
	level   int
	active  bool
	preview bool
}

const (
	minColWidth    = 20
	maxColWidth    = 44
	targetColWidth = 36
	sizeField      = 8
	pctField       = 4
	chromeRows     = 4 // header, divider, divider, footer
)

func (m *model) bodyHeight() int {
	return max(1, m.h-chromeRows)
}

// colWidth fits the available width over a sensible number of columns.
func (m *model) colWidth() int {
	if m.w <= 0 {
		return targetColWidth
	}
	visible := min(6, max(1, (m.w+targetColWidth/2)/targetColWidth))
	return min(maxColWidth, max(minColWidth, m.w/visible))
}

// columns returns the path columns plus a preview of the selected directory.
func (m *model) columns() []column {
	cols := make([]column, 0, len(m.path)+1)
	for i, n := range m.path {
		nodes := m.childrenOf(n)
		active := i == len(m.path)-1
		sel := -1
		if active {
			if len(nodes) > 0 && m.sel >= 0 && m.sel < len(nodes) {
				sel = m.sel
			}
		} else {
			sel = scan.IndexOf(nodes, m.path[i+1])
		}
		cols = append(cols, column{title: n.Name, nodes: nodes, sel: sel, level: i, active: active})
	}
	if s := m.selectedNode(); s != nil && s.IsDir {
		if nodes := m.childrenOf(s); len(nodes) > 0 {
			cols = append(cols, column{title: s.Name, nodes: nodes, sel: -1, level: len(m.path), preview: true})
		}
	}
	return cols
}

func (m *model) View() string {
	if m.w <= 0 || m.h <= 0 {
		return ""
	}
	if m.mode == modeScanning {
		return m.viewScanning()
	}
	if m.root == nil {
		return "nothing to show"
	}

	bodyH := m.bodyHeight()
	body := m.viewBody(bodyH)

	switch m.mode {
	case modeConfirm:
		body = centreBox(m.viewConfirm(), bodyH, m.w)
	case modeHelp:
		body = centreBox(m.viewHelp(), bodyH, m.w)
	}

	var b strings.Builder
	b.WriteString(m.viewHeader())
	b.WriteString("\n")
	b.WriteString(styleDivider.Render(strings.Repeat("─", m.w)))
	b.WriteString("\n")
	b.WriteString(strings.Join(body, "\n"))
	b.WriteString("\n")
	b.WriteString(styleDivider.Render(strings.Repeat("─", m.w)))
	b.WriteString("\n")
	b.WriteString(m.viewFooter())
	return b.String()
}

// viewBody renders exactly height lines and records the layout.
func (m *model) viewBody(height int) []string {
	cols := m.columns()
	colW := m.colWidth()
	visible := max(1, m.w/colW)

	// Show the tail so the active column stays on screen.
	end := len(cols)
	if visible == 1 {
		end = min(len(cols), max(1, len(m.path)))
	}
	start := max(0, end-visible)
	if start >= len(cols) {
		start = max(0, len(cols)-1)
	}
	end = min(end, len(cols))
	window := cols[start:end]
	if len(window) == 0 {
		window = cols[len(cols)-1:]
	}

	contentW := max(8, colW-1)
	rows := height - 1 // the column titles take one line
	m.layout = layout{colWidth: colW, bodyTop: 3, bodyHeight: rows}

	rendered := make([][]string, 0, len(window))
	for _, c := range window {
		lines, cl := m.renderColumn(c, contentW, rows, m.parentSize(c))
		rendered = append(rendered, lines)
		m.layout.columns = append(m.layout.columns, cl)
	}

	out := make([]string, 0, height)
	out = append(out, clip(m.columnTitles(window, contentW), m.w))

	for row := 0; row < rows; row++ {
		var b strings.Builder
		for _, lines := range rendered {
			// Padding keeps short columns from shifting the rest.
			b.WriteString(pad(lines[row], contentW))
			b.WriteString(" ")
		}
		out = append(out, clip(b.String(), m.w))
	}
	return out
}

// parentSize is the size percentages are relative to.
func (m *model) parentSize(c column) int64 {
	switch {
	case c.preview:
		var total int64
		for _, n := range c.nodes {
			total += n.Size
		}
		return total
	case c.level < len(m.path):
		return m.path[c.level].Size
	default:
		return 0
	}
}

// renderColumn renders one column of name/size/percentage rows.
func (m *model) renderColumn(c column, width, rows int, parentSize int64) ([]string, colLayout) {
	cl := colLayout{level: c.level, nodes: c.nodes}
	lines := make([]string, 0, rows)

	if len(c.nodes) == 0 {
		lines = append(lines, styleRowDim.Render("  (empty)"))
		for len(lines) < rows {
			lines = append(lines, "")
		}
		return lines, cl
	}

	offset := 0
	if c.sel >= rows {
		offset = c.sel - rows + 1
	}
	cl.offset = offset

	for i := 0; i < rows; i++ {
		idx := offset + i
		if idx >= len(c.nodes) {
			lines = append(lines, "")
			continue
		}
		n := c.nodes[idx]
		st := rowStyle{preview: c.preview, marked: m.marked[n]}
		if c.active {
			st.selected = idx == c.sel
		} else {
			// Ancestor columns point at the entry on the path.
			st.onPath = idx == c.sel
		}
		lines = append(lines, renderRow(n, parentSize, width, st))
	}
	return lines, cl
}

// rowStyle controls how a row is drawn.
type rowStyle struct {
	selected bool // the caret is on this row: it is the active highlight
	onPath   bool // the row leads into the directory being viewed
	preview  bool // the row is in the preview column
	marked   bool // the row is queued for deletion
}

func renderRow(n *scan.Node, parentSize int64, width int, st rowStyle) string {
	const leadCells = 2 // the caret and the mark
	nameW := width - 17
	showPct := nameW >= 4
	if !showPct {
		nameW += pctField + 1
	}
	if nameW < 1 {
		nameW = 1
	}

	cursor := " "
	switch {
	case st.selected:
		cursor = "▸"
	case st.onPath:
		cursor = "›"
	case n.Skipped:
		cursor = "⊘"
	case n.IsDir:
		cursor = "▾"
	}
	mark := " "
	if st.marked {
		mark = "●"
	}

	pct := percentInt(n.Size, parentSize)
	sizeText := humanBytes(n.Size)
	if n.Skipped {
		sizeText = "other fs"
	}
	raw := cursor + mark + " " + pad(truncate(n.Name, nameW), nameW) + " " +
		fmt.Sprintf("%*s", sizeField, sizeText)
	if showPct {
		raw += " " + fmt.Sprintf("%3d%%", pct)
	}
	raw = pad(raw, width) // always exactly width cells

	// The bar is the row background, proportional to the share.
	barW := min(max(pct*width/100, 0), width)
	bodyBarW := min(max(barW-leadCells, 0), ansi.StringWidth(raw)-leadCells)
	tail := raw[len(cursor)+len(mark):]
	prefix, rest := splitAtCells(tail, max(0, bodyBarW))

	barStyle, textStyle := barStyleFor(pct), styleRowText
	if st.preview {
		textStyle = styleRowDim
	}
	if st.selected {
		textStyle = styleSelText
	}

	cursorStyle := styleCaretIdle
	if st.selected {
		cursorStyle = styleCaret
	}
	markStyle := lipgloss.NewStyle()
	if st.marked {
		markStyle = markStyle.Bold(true).Foreground(markColor)
	}
	if barW >= leadCells {
		markStyle = markStyle.Background(barColorFor(pct))
	}
	row := cursorStyle.Render(cursor) + markStyle.Render(mark) +
		barStyle.Render(prefix) + textStyle.Render(rest)
	return clip(row, width)
}

var markColor = lipgloss.Color("214")

func barColorFor(pct int) lipgloss.Color {
	switch {
	case pct >= 25:
		return lipgloss.Color("88")
	case pct >= 8:
		return lipgloss.Color("94")
	default:
		return lipgloss.Color("28")
	}
}

func barStyleFor(pct int) lipgloss.Style {
	switch {
	case pct >= 25:
		return styleBarHigh
	case pct >= 8:
		return styleBarMid
	default:
		return styleBarLow
	}
}

func percentInt(part, whole int64) int {
	if whole <= 0 {
		return 0
	}
	v := int(float64(part)/float64(whole)*100 + 0.5)
	switch {
	case v > 100:
		return 100
	case v < 0:
		return 0
	default:
		return v
	}
}

func (m *model) columnTitles(cols []column, width int) string {
	var b strings.Builder
	for i, c := range cols {
		style := styleColTitleInactive
		active := c.active
		label := " " + c.title
		switch {
		case active:
			style = styleColTitleActive
		case c.preview:
			style = styleColTitlePreview
			label = " " + c.title + " ›"
		}

		if suffix := m.scrollSuffix(i, c); suffix != "" {
			label = " " + truncate(c.title, max(1, width-ansi.StringWidth(suffix)-1)) + suffix
			if c.preview && !active {
				label = " " + truncate(c.title, max(1, width-ansi.StringWidth(suffix)-3)) + " ›" + suffix
			}
		}
		b.WriteString(style.Render(pad(truncate(label, width), width)))
		b.WriteString(" ")
	}
	return b.String()
}

// scrollSuffix describes how far a column is scrolled.
func (m *model) scrollSuffix(i int, c column) string {
	if i >= len(m.layout.columns) {
		return ""
	}
	rows := m.layout.bodyHeight
	if rows <= 0 || len(c.nodes) <= rows {
		return ""
	}
	cl := m.layout.columns[i]
	above := cl.offset > 0
	below := cl.offset+rows < len(c.nodes)
	arrow := "▲▼"
	switch {
	case below && !above:
		arrow = "▼"
	case above && !below:
		arrow = "▲"
	}
	return fmt.Sprintf(" %d %s", len(c.nodes), arrow)
}

func (m *model) viewHeader() string {
	title := styleTitleName.Render(" diskusage ")
	cur := ""
	if len(m.path) > 0 {
		cur = m.path[len(m.path)-1].Path
	}
	left := title + styleTitleBar.Render(" "+foldPath(cur, max(10, m.w/3))+" ")

	size, files, dirs, errs := int64(0), int64(0), int64(0), int64(0)
	if m.root != nil {
		size, files, dirs, errs = m.root.Size, m.root.Files, m.root.Dirs, m.root.Errors
	}
	right := fmt.Sprintf("%s · %s %s · %s %s ",
		humanBytes(size),
		humanCount(files), plural(files, "file", "files"),
		humanCount(dirs), plural(dirs, "dir", "dirs"))
	if errs > 0 {
		right += styleWarnOnBar.Render(fmt.Sprintf("· %d unreadable ", errs))
	}
	if m.diskTotal > 0 {
		used := float64(m.diskTotal-m.diskFree) / float64(m.diskTotal) * 100
		right += fmt.Sprintf("· disk %.0f%% full (%s free) ", used, humanBytes(int64(m.diskFree)))
	}

	// Trim the path first so the title bar never wraps.
	if m.w-lipgloss.Width(left)-lipgloss.Width(right) < 0 {
		left = title
	}
	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		right = truncate(right, max(0, m.w-lipgloss.Width(left)))
		gap = max(1, m.w-lipgloss.Width(left)-lipgloss.Width(right))
	}
	return clip(left+styleTitleBar.Render(strings.Repeat(" ", gap))+right, m.w)
}

func (m *model) viewFooter() string {
	if m.status != "" {
		style := styleOK
		if m.statusErr {
			style = styleBad
		}
		return clip(style.Render(" "+m.status), m.w)
	}

	if m.mode == modeFiltering {
		matches := len(m.activeNodes())
		return clip(" "+styleKey.Render("/")+m.filter+styleKey.Render("▌")+
			styleHint.Render(fmt.Sprintf("  %d %s · enter keeps it · esc clears",
				matches, plural(int64(matches), "match", "matches"))), m.w)
	}

	sort := "sort:" + m.sortMode.String()
	if m.filter != "" {
		sort += " filter:" + m.filter
	}
	if count, size := m.markedTotals(); count > 0 {
		head := " " + styleMarked.Render("●") + styleOK.Render(fmt.Sprintf(" %d selected · %s", count, humanBytes(size))) + "  "
		hints := styleKey.Render("space") + " toggle  " +
			styleKey.Render("d") + " delete  " +
			styleKey.Render("c") + " clear  " +
			styleKey.Render("?") + " help  " +
			styleKey.Render("q") + " quit"
		return clip(head+hints, m.w)
	}
	hints := " " + styleKey.Render("↑↓") + " move  " +
		styleKey.Render("→/⏎") + " open  " +
		styleKey.Render("←") + " back  " +
		styleKey.Render("space") + " mark  " +
		styleKey.Render("d") + " delete  " +
		styleKey.Render("s") + " " + sort + "  " +
		styleKey.Render("/") + " filter  " +
		styleKey.Render("o") + " finder  " +
		styleKey.Render("r") + " rescan  " +
		styleKey.Render("?") + " help  " +
		styleKey.Render("q") + " quit"
	return clip(hints, m.w)
}

func (m *model) viewScanning() string {
	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	s := spinner[m.spinner%len(spinner)]
	p := m.progress

	lines := []string{
		"",
		"  " + styleTitleName.Render(" diskusage ") + styleTitleBar.Render(" scanning "),
		"",
		"  " + styleKey.Render(s) + "  " + humanCount(p.Files) + " files   " +
			humanCount(p.Dirs) + " dirs   " + humanBytes(p.Bytes),
	}
	if p.Errs > 0 {
		lines = append(lines, "  "+styleWarn.Render(fmt.Sprintf("%d entries could not be read", p.Errs)))
	}
	lines = append(lines, "", "  "+styleHint.Render(foldPath(p.Cur, max(20, m.w-4))), "",
		"  "+styleHint.Render(fmt.Sprintf("scanning %s", foldPath(m.scanRoot(), max(20, m.w-16)))), "",
		"  "+styleHint.Render("q quits"))

	out := make([]string, 0, m.h)
	for i := 0; i < max(0, (m.h-len(lines))/2); i++ {
		out = append(out, "")
	}
	out = append(out, lines...)
	for len(out) < m.h {
		out = append(out, "")
	}
	return strings.Join(out[:m.h], "\n")
}

func (m *model) scanRoot() string {
	if len(m.path) > 0 {
		return m.path[0].Path
	}
	if m.root != nil {
		return m.root.Path
	}
	return ""
}

// shortPath shows a path relative to the scan root.
func (m *model) shortPath(n *scan.Node) string {
	if n == nil {
		return ""
	}
	p := n.Path
	if m.root != nil && m.root.Path != "" && m.root.Path != p && strings.HasPrefix(p, m.root.Path) {
		p = strings.TrimPrefix(strings.TrimPrefix(p, m.root.Path), string(filepath.Separator))
	}
	if p == "" {
		p = n.Name
	}
	return p
}

// fitPath keeps the tail of a long path visible.
func fitPath(p string, width int) string {
	if ansi.StringWidth(p) <= width {
		return p
	}
	return truncate(foldPath(p, width), width)
}

func (m *model) viewConfirm() string {
	if len(m.pending) == 0 {
		return ""
	}
	inner := min(max(40, m.w-8), 68) - 6

	count, size := 0, int64(0)
	files, dirs := int64(0), int64(0)
	for _, n := range m.pending {
		count++
		size += n.Size
		files += n.Files
		dirs += n.Dirs
	}

	verb := "Delete permanently?"
	note := "This cannot be undone."
	if m.opts.Trash {
		verb = "Move to Trash?"
		note = "It can be restored from the Trash until the Trash is emptied."
	}
	if count > 1 {
		verb = fmt.Sprintf("%s (%d entries)", strings.TrimSuffix(verb, "?"), count)
	}

	detail := fmt.Sprintf("%s · %s %s · %s %s",
		humanBytes(size),
		humanCount(files), plural(files, "file", "files"),
		humanCount(dirs), plural(dirs, "dir", "dirs"))

	body := []string{styleBad.Bold(true).Render(verb), ""}
	for i, n := range m.pending {
		if i == 3 {
			body = append(body, styleHint.Render(fmt.Sprintf("… and %d more", count-i)))
			break
		}
		body = append(body, fitPath(m.shortPath(n), inner))
	}
	body = append(body,
		styleHint.Render(truncate(detail, inner)),
		"",
		styleKey.Render("y")+" confirm      "+styleKey.Render("n")+" cancel",
		"",
		styleHint.Render(truncate(note, inner)),
	)
	return styleBox.Render(strings.Join(body, "\n"))
}

func (m *model) viewHelp() string {
	keyW := 22
	inner := min(max(44, m.w-16), 76) - 6

	rows := [][2]string{
		{"↑ ↓ / j k", "move the selection"},
		{"→ / l / enter", "walk into the highlighted directory"},
		{"← / h", "walk back out one level"},
		{"g / G", "jump to the first / last entry"},
		{"space", "select the entry for deletion (and move on)"},
		{"a / c", "select every entry here / clear the selection"},
		{"d / x", "delete the selected entries, after a confirmation"},
		{"o", "open in Finder (files are revealed)"},
		{"R", "reveal the highlighted entry in Finder"},
		{"s", "sort by size → name → items"},
		{"/", "filter entries, enter keeps it, esc clears"},
		{"r", "rescan everything from the root"},
		{"mouse", "wheel scrolls, click selects and enters"},
		{"? / q", "close help / quit"},
	}
	var b strings.Builder
	b.WriteString(styleTitleName.Render(" keys "))
	b.WriteString("\n\n")
	for _, r := range rows {
		b.WriteString(styleKey.Render(pad(r[0], keyW)))
		b.WriteString(styleHint.Render(truncate(r[1], inner-keyW)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if m.opts.Trash {
		b.WriteString(styleHint.Render("deletes go to the Trash"))
	} else {
		b.WriteString(styleWarn.Render("deletes are permanent — run with --trash to use the Trash"))
	}
	return styleHelpBox.Render(b.String())
}

// centreBox centres box in an area of exactly height lines.
func centreBox(box string, height, width int) []string {
	boxLines := strings.Split(box, "\n")
	boxW := 0
	for _, l := range boxLines {
		boxW = max(boxW, ansi.StringWidth(l))
	}
	top := max(0, (height-len(boxLines))/2)
	left := max(0, (width-boxW)/2)

	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		j := i - top
		if j < 0 || j >= len(boxLines) {
			out = append(out, "")
			continue
		}
		out = append(out, clip(strings.Repeat(" ", left)+boxLines[j], width))
	}
	return out
}
