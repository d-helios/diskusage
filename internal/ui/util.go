package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// humanBytes formats a byte count into at most 8 characters.
func humanBytes(n int64) string {
	if n < 0 {
		return "-"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit && exp < 5; x /= unit {
		div *= unit
		exp++
	}
	v := float64(n) / float64(div)
	unitChar := "KMGTPE"[exp]
	switch {
	case v >= 100:
		return fmt.Sprintf("%.0f %ciB", v, unitChar)
	case v >= 10:
		return fmt.Sprintf("%.1f %ciB", v, unitChar)
	default:
		return fmt.Sprintf("%.2f %ciB", v, unitChar)
	}
}

func humanCount(n int64) string {
	s := fmt.Sprintf("%d", n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "…")
}

func pad(s string, width int) string {
	w := ansi.StringWidth(s)
	if w > width {
		return truncate(s, width)
	}
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

func clip(s string, width int) string {
	if ansi.StringWidth(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "")
}

// splitAtCells splits s after cells terminal cells, without splitting a rune.
func splitAtCells(s string, cells int) (string, string) {
	if cells <= 0 {
		return "", s
	}
	width := 0
	for i, r := range s {
		rw := ansi.StringWidth(string(r))
		if width+rw > cells {
			return s[:i], s[i:]
		}
		width += rw
	}
	return s, ""
}

func plural(n int64, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// foldPath shortens a long path from the front, keeping the tail.
func foldPath(p string, width int) string {
	if len(p) <= width {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		p = "~" + strings.TrimPrefix(p, home)
		if len(p) <= width {
			return p
		}
	}
	parts := strings.Split(p, string(filepath.Separator))
	var tail []string
	size := 0
	for i := len(parts) - 1; i >= 0; i-- {
		size += len(parts[i]) + 1
		if size > width-2 {
			break
		}
		tail = append(tail, parts[i])
	}
	for i, j := 0, len(tail)-1; i < j; i, j = i+1, j-1 {
		tail[i], tail[j] = tail[j], tail[i]
	}
	return "…" + string(filepath.Separator) + strings.Join(tail, string(filepath.Separator))
}
