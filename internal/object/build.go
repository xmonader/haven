package object

import (
	"fmt"
	"sort"
	"strings"
)

// FileEntry describes a file to place in a tree.
type FileEntry struct {
	Hash string // blob hash
	Mode string // ModeFile or ModeExec
}

// BuildTree constructs nested tree objects from a flat map of forward-slash
// paths to file entries, stores them, and returns the root tree hash.
// An empty map yields an empty tree.
func BuildTree(s *Store, files map[string]FileEntry) (string, error) {
	return buildSubtree(s, files, "")
}

// buildSubtree builds the tree rooted at prefix (e.g. "" or "a/b/").
func buildSubtree(s *Store, files map[string]FileEntry, prefix string) (string, error) {
	// Group immediate children: direct files vs subdirectories.
	type sub struct{ paths map[string]FileEntry }
	subdirs := map[string]*sub{}
	var entries []TreeEntry

	for path, fe := range files {
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		rest := path[len(prefix):]
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			name := rest[:i]
			sd := subdirs[name]
			if sd == nil {
				sd = &sub{paths: map[string]FileEntry{}}
				subdirs[name] = sd
			}
			sd.paths[path] = fe
		} else {
			entries = append(entries, TreeEntry{
				Mode: fe.Mode, Type: Blob, Hash: fe.Hash, Name: rest,
			})
		}
	}

	// Build subdirectory trees in deterministic order.
	names := make([]string, 0, len(subdirs))
	for n := range subdirs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		h, err := buildSubtree(s, subdirs[name].paths, prefix+name+"/")
		if err != nil {
			return "", err
		}
		entries = append(entries, TreeEntry{Mode: ModeTree, Type: Tree, Hash: h, Name: name})
	}

	return s.PutTree(entries)
}

// Flatten walks a tree and returns a map of forward-slash paths to blob hashes.
func Flatten(s *Store, treeHash string) (map[string]string, error) {
	out := map[string]string{}
	if treeHash == "" {
		return out, nil
	}
	if err := flatten(s, treeHash, "", out); err != nil {
		return nil, err
	}
	return out, nil
}

func flatten(s *Store, treeHash, prefix string, out map[string]string) error {
	entries, err := s.GetTree(treeHash)
	if err != nil {
		return err
	}
	for _, e := range entries {
		switch e.Type {
		case Blob:
			out[prefix+e.Name] = e.Hash
		case Tree:
			if err := flatten(s, e.Hash, prefix+e.Name+"/", out); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected tree entry type %q", e.Type)
		}
	}
	return nil
}

// TreeOfCommit returns the root tree hash of a commit, or "" for no commit.
func (s *Store) TreeOfCommit(commitHash string) (string, error) {
	if commitHash == "" {
		return "", nil
	}
	c, err := s.GetCommit(commitHash)
	if err != nil {
		return "", err
	}
	return c.Tree, nil
}
