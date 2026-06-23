package object

import (
	"fmt"
	"sort"
	"strings"
)

// maxTreeDepth bounds how deeply nested a tree may be when walked. Real source
// trees are nowhere near this; the limit exists so a maliciously or corruptly
// deep tree cannot exhaust the stack and crash the process (a remote can push
// such a tree, and the server walks it during reachability checks).
const maxTreeDepth = 256

// errTreeTooDeep is returned when a tree walk exceeds maxTreeDepth.
var errTreeTooDeep = fmt.Errorf("tree nesting exceeds %d levels: refusing (possible malicious or corrupt tree)", maxTreeDepth)

// FileEntry describes a file to place in a tree.
type FileEntry struct {
	Hash string // object hash
	Mode string // ModeFile or ModeExec
	Type Type   // Blob (default) or Secret
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
			t := fe.Type
			if t == "" {
				t = Blob
			}
			entries = append(entries, TreeEntry{
				Mode: fe.Mode, Type: t, Hash: fe.Hash, Name: rest,
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
	if err := flatten(s, treeHash, "", out, 0); err != nil {
		return nil, err
	}
	return out, nil
}

// FlattenFull walks a tree returning paths mapped to full file entries
// (blob hash and mode).
func FlattenFull(s *Store, treeHash string) (map[string]FileEntry, error) {
	out := map[string]FileEntry{}
	if treeHash == "" {
		return out, nil
	}
	if err := flattenFull(s, treeHash, "", out, 0); err != nil {
		return nil, err
	}
	return out, nil
}

func flattenFull(s *Store, treeHash, prefix string, out map[string]FileEntry, depth int) error {
	if depth > maxTreeDepth {
		return errTreeTooDeep
	}
	entries, err := s.GetTree(treeHash)
	if err != nil {
		return err
	}
	for _, e := range entries {
		switch e.Type {
		case Tree:
			if err := flattenFull(s, e.Hash, prefix+e.Name+"/", out, depth+1); err != nil {
				return err
			}
		default: // Blob or Secret: a leaf
			out[prefix+e.Name] = FileEntry{Hash: e.Hash, Mode: e.Mode, Type: e.Type}
		}
	}
	return nil
}

func flatten(s *Store, treeHash, prefix string, out map[string]string, depth int) error {
	if depth > maxTreeDepth {
		return errTreeTooDeep
	}
	entries, err := s.GetTree(treeHash)
	if err != nil {
		return err
	}
	for _, e := range entries {
		switch e.Type {
		case Tree:
			if err := flatten(s, e.Hash, prefix+e.Name+"/", out, depth+1); err != nil {
				return err
			}
		default: // Blob or Secret: a leaf
			out[prefix+e.Name] = e.Hash
		}
	}
	return nil
}

// IsAncestor reports whether ancestor is reachable from descendant by walking
// parent links. A commit is its own ancestor. An empty ancestor is treated as
// reachable from anything (the empty/unborn history).
func (s *Store) IsAncestor(ancestor, descendant string) (bool, error) {
	if ancestor == "" || ancestor == descendant {
		return true, nil
	}
	if descendant == "" {
		return false, nil
	}
	seen := map[string]bool{}
	stack := []string{descendant}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == ancestor {
			return true, nil
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		c, err := s.GetCommit(cur)
		if err != nil {
			return false, err
		}
		stack = append(stack, c.Parents...)
	}
	return false, nil
}

// ancestors returns the set of commits reachable from start (including start).
func (s *Store) ancestors(start string) (map[string]bool, error) {
	set := map[string]bool{}
	if start == "" {
		return set, nil
	}
	stack := []string{start}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if set[cur] {
			continue
		}
		set[cur] = true
		c, err := s.GetCommit(cur)
		if err != nil {
			return nil, err
		}
		stack = append(stack, c.Parents...)
	}
	return set, nil
}

// MergeBase returns the best common ancestor of a and b: a common ancestor that
// is not itself an ancestor of any other common ancestor. Returns "" if there
// is none (disjoint histories).
func (s *Store) MergeBase(a, b string) (string, error) {
	if a == "" || b == "" {
		return "", nil
	}
	ancA, err := s.ancestors(a)
	if err != nil {
		return "", err
	}
	// Walk b's history; the frontier of nodes already in ancA are candidates.
	var candidates []string
	visited := map[string]bool{}
	queue := []string{b}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if visited[cur] {
			continue
		}
		visited[cur] = true
		if ancA[cur] {
			candidates = append(candidates, cur)
			continue // do not descend past a common ancestor
		}
		c, err := s.GetCommit(cur)
		if err != nil {
			return "", err
		}
		queue = append(queue, c.Parents...)
	}
	// Pick a candidate not dominated by another candidate.
	for _, c := range candidates {
		dominated := false
		for _, d := range candidates {
			if c == d {
				continue
			}
			anc, err := s.IsAncestor(c, d)
			if err != nil {
				return "", err
			}
			if anc {
				dominated = true
				break
			}
		}
		if !dominated {
			return c, nil
		}
	}
	return "", nil
}

// Reachable returns the set of all object hashes reachable from a commit:
// the commit chain, every tree and subtree, and every blob.
func (s *Store) Reachable(commitHash string) (map[string]bool, error) {
	objs := map[string]bool{}
	if commitHash == "" {
		return objs, nil
	}
	commits := []string{commitHash}
	for len(commits) > 0 {
		ch := commits[len(commits)-1]
		commits = commits[:len(commits)-1]
		if objs[ch] {
			continue
		}
		objs[ch] = true
		c, err := s.GetCommit(ch)
		if err != nil {
			return nil, err
		}
		if err := s.collectTree(c.Tree, objs, 0); err != nil {
			return nil, err
		}
		commits = append(commits, c.Parents...)
	}
	return objs, nil
}

func (s *Store) collectTree(treeHash string, objs map[string]bool, depth int) error {
	if treeHash == "" || objs[treeHash] {
		return nil
	}
	if depth > maxTreeDepth {
		return errTreeTooDeep
	}
	objs[treeHash] = true
	entries, err := s.GetTree(treeHash)
	if err != nil {
		return err
	}
	for _, e := range entries {
		switch e.Type {
		case Tree:
			if err := s.collectTree(e.Hash, objs, depth+1); err != nil {
				return err
			}
		default:
			objs[e.Hash] = true
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
