package object

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// File modes stored in tree entries.
const (
	ModeFile    = "100644"
	ModeExec    = "100755"
	ModeTree    = "040000"
	ModeSymlink = "120000" // blob content is the link target path
)

// TreeEntry is one row in a tree: a named child object.
type TreeEntry struct {
	Mode string // ModeFile, ModeExec, or ModeTree
	Type Type   // Blob or Tree
	Hash string
	Name string // single path component, no slashes
}

// SerializeTree renders entries to the canonical on-disk form, sorted by name
// so identical content always hashes identically.
// Each line: "<mode> <type> <hash>\t<name>\n".
func SerializeTree(entries []TreeEntry) []byte {
	sorted := make([]TreeEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	var b bytes.Buffer
	for _, e := range sorted {
		fmt.Fprintf(&b, "%s %s %s\t%s\n", e.Mode, e.Type, e.Hash, e.Name)
	}
	return b.Bytes()
}

// ParseTree decodes a tree payload.
func ParseTree(payload []byte) ([]TreeEntry, error) {
	var out []TreeEntry
	for _, line := range strings.Split(strings.TrimRight(string(payload), "\n"), "\n") {
		if line == "" {
			continue
		}
		meta, name, ok := strings.Cut(line, "\t")
		if !ok {
			return nil, fmt.Errorf("tree line missing name: %q", line)
		}
		fields := strings.Fields(meta)
		if len(fields) != 3 {
			return nil, fmt.Errorf("tree line malformed: %q", line)
		}
		out = append(out, TreeEntry{
			Mode: fields[0],
			Type: Type(fields[1]),
			Hash: fields[2],
			Name: name,
		})
	}
	return out, nil
}

// PutTree serializes and stores a tree, returning its hash.
func (s *Store) PutTree(entries []TreeEntry) (string, error) {
	return s.Put(Tree, SerializeTree(entries))
}

// GetTree fetches and parses a tree object.
func (s *Store) GetTree(h string) ([]TreeEntry, error) {
	t, payload, err := s.Get(h)
	if err != nil {
		return nil, err
	}
	if t != Tree {
		return nil, fmt.Errorf("object %s is %s, want tree", h, t)
	}
	return ParseTree(payload)
}
