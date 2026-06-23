// Package diff computes changes between two trees and renders unified diffs of
// file contents.
package diff

import (
	"fmt"
	"sort"
	"strings"

	"haven/internal/object"
)

// Kind classifies a path-level change.
type Kind string

const (
	Added    Kind = "added"
	Modified Kind = "modified"
	Deleted  Kind = "deleted"
)

// Change is one path that differs between two trees.
type Change struct {
	Path string
	Kind Kind
	Old  string // old blob hash ("" if added)
	New  string // new blob hash ("" if deleted)
}

// Tree compares two trees (by hash; "" means empty) and returns sorted changes.
func Tree(store *object.Store, treeA, treeB string) ([]Change, error) {
	a, err := object.Flatten(store, treeA)
	if err != nil {
		return nil, err
	}
	b, err := object.Flatten(store, treeB)
	if err != nil {
		return nil, err
	}
	return Maps(a, b), nil
}

// Maps compares two path->hash maps and returns sorted changes.
func Maps(a, b map[string]string) []Change {
	var changes []Change
	for path, oldH := range a {
		newH, ok := b[path]
		if !ok {
			changes = append(changes, Change{Path: path, Kind: Deleted, Old: oldH})
		} else if oldH != newH {
			changes = append(changes, Change{Path: path, Kind: Modified, Old: oldH, New: newH})
		}
	}
	for path, newH := range b {
		if _, ok := a[path]; !ok {
			changes = append(changes, Change{Path: path, Kind: Added, New: newH})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes
}

// Unified renders a unified diff between two byte contents with 3 lines of
// context. Returns "" if identical.
func Unified(oldName, newName string, oldContent, newContent []byte) string {
	if string(oldContent) == string(newContent) {
		return ""
	}
	oldLines := splitKeep(string(oldContent))
	newLines := splitKeep(string(newContent))
	ops := lcsDiff(oldLines, newLines)
	hunks := group(ops, 3)
	if len(hunks) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n", oldName)
	fmt.Fprintf(&b, "+++ %s\n", newName)
	for _, h := range hunks {
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", h.oldStart+1, h.oldCount, h.newStart+1, h.newCount)
		for _, op := range h.ops {
			b.WriteString(op.sign())
			b.WriteString(op.text)
			if !strings.HasSuffix(op.text, "\n") {
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

func splitKeep(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	// SplitAfter leaves a trailing "" if s ends in newline; drop it.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}
