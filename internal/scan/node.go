package scan

import (
	"sort"
	"strings"
)

// Node is a single entry in the scanned tree.
type Node struct {
	Name     string
	Path     string
	Size     int64
	IsDir    bool
	Children []*Node
	Files    int64
	Dirs     int64
	Errors   int64
	Skipped  bool

	Parent *Node
}

func (n *Node) Items() int64 {
	if !n.IsDir {
		return 1
	}
	return n.Files + n.Dirs
}

type SortMode int

const (
	SortBySize SortMode = iota
	SortByName
	SortByCount
)

var sortModes = []SortMode{SortBySize, SortByName, SortByCount}

func (s SortMode) String() string {
	switch s {
	case SortByName:
		return "name"
	case SortByCount:
		return "items"
	default:
		return "size"
	}
}

func (s SortMode) Next() SortMode {
	for i, m := range sortModes {
		if m == s {
			return sortModes[(i+1)%len(sortModes)]
		}
	}
	return SortBySize
}

func SortTree(n *Node, mode SortMode) {
	if n == nil || !n.IsDir || len(n.Children) == 0 {
		return
	}
	sort.SliceStable(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		switch mode {
		case SortByName:
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case SortByCount:
			if a.Items() != b.Items() {
				return a.Items() > b.Items()
			}
		}
		if a.Size != b.Size {
			return a.Size > b.Size
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	for _, c := range n.Children {
		SortTree(c, mode)
	}
}

// Detach removes n from its parent and adjusts the sizes and counts above it.
func Detach(n *Node) int64 {
	p := n.Parent
	if p == nil {
		return 0
	}
	i := IndexOf(p.Children, n)
	if i < 0 {
		return 0
	}
	p.Children = append(p.Children[:i], p.Children[i+1:]...)
	for a := p; a != nil; a = a.Parent {
		a.Size -= n.Size
		a.Files -= n.Files
		a.Dirs -= n.Dirs
		if a.Size < 0 {
			a.Size = 0
		}
	}
	n.Parent = nil
	return n.Size
}

// ReplaceSubtree swaps old for fresh and updates the totals on the path to the root.
func ReplaceSubtree(old, fresh *Node) {
	p := old.Parent
	if p == nil {
		return
	}
	fresh.Parent = p
	delta := fresh.Size - old.Size
	fileDelta := fresh.Files - old.Files
	dirDelta := fresh.Dirs - old.Dirs
	for i, c := range p.Children {
		if c == old {
			p.Children[i] = fresh
			break
		}
	}
	for a := p; a != nil; a = a.Parent {
		a.Size += delta
		a.Files += fileDelta
		a.Dirs += dirDelta
	}
}

func IndexOf(nodes []*Node, n *Node) int {
	for i, c := range nodes {
		if c == n {
			return i
		}
	}
	return -1
}
