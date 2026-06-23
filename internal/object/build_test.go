package object

import (
	"errors"
	"strings"
	"testing"

	"haven/internal/store"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db)
}

func TestBuildTreeFlattenRoundTrip(t *testing.T) {
	s := newStore(t)
	b1, _ := s.Put(Blob, []byte("one"))
	b2, _ := s.Put(Blob, []byte("two"))
	files := map[string]FileEntry{
		"a.txt":        {Hash: b1, Mode: ModeFile, Type: Blob},
		"dir/b.txt":    {Hash: b2, Mode: ModeFile, Type: Blob},
		"dir/sub/c.go": {Hash: b1, Mode: ModeExec, Type: Blob},
	}
	tree, err := BuildTree(s, files)
	if err != nil {
		t.Fatal(err)
	}
	flat, err := Flatten(s, tree)
	if err != nil {
		t.Fatal(err)
	}
	if len(flat) != 3 || flat["a.txt"] != b1 || flat["dir/b.txt"] != b2 {
		t.Fatalf("flatten round-trip wrong: %v", flat)
	}
	full, _ := FlattenFull(s, tree)
	if full["dir/sub/c.go"].Mode != ModeExec {
		t.Errorf("exec mode lost through tree: %+v", full["dir/sub/c.go"])
	}
	// Determinism: same input builds the same tree hash.
	tree2, _ := BuildTree(s, files)
	if tree != tree2 {
		t.Error("BuildTree must be deterministic")
	}
}

func TestEmptyTree(t *testing.T) {
	s := newStore(t)
	tree, err := BuildTree(s, map[string]FileEntry{})
	if err != nil {
		t.Fatal(err)
	}
	flat, _ := Flatten(s, tree)
	if len(flat) != 0 {
		t.Errorf("empty tree should flatten to nothing, got %v", flat)
	}
}

func TestCommitSerializeRoundTrip(t *testing.T) {
	c := CommitObj{Tree: "t1", Parents: []string{"p1", "p2"}, Author: "Dev", Email: "d@e", When: 1234, Message: "msg\nbody"}
	got, err := ParseCommit(SerializeCommit(c))
	if err != nil {
		t.Fatal(err)
	}
	if got.Tree != c.Tree || len(got.Parents) != 2 || got.Author != "Dev" || got.When != 1234 || got.Message != "msg\nbody" {
		t.Fatalf("commit round-trip mismatch: %+v", got)
	}
}

func TestAncestryAndMergeBase(t *testing.T) {
	s := newStore(t)
	empty, _ := BuildTree(s, map[string]FileEntry{})
	c1, _ := s.PutCommit(CommitObj{Tree: empty, Message: "1"})
	c2, _ := s.PutCommit(CommitObj{Tree: empty, Parents: []string{c1}, Message: "2"})
	c3, _ := s.PutCommit(CommitObj{Tree: empty, Parents: []string{c2}, Message: "3"})
	// A divergent branch off c1.
	d2, _ := s.PutCommit(CommitObj{Tree: empty, Parents: []string{c1}, Message: "d2"})

	if anc, _ := s.IsAncestor(c1, c3); !anc {
		t.Error("c1 should be an ancestor of c3")
	}
	if anc, _ := s.IsAncestor(c3, c1); anc {
		t.Error("c3 is not an ancestor of c1")
	}
	if anc, _ := s.IsAncestor(d2, c3); anc {
		t.Error("divergent d2 is not an ancestor of c3")
	}
	base, err := s.MergeBase(c3, d2)
	if err != nil {
		t.Fatal(err)
	}
	if base != c1 {
		t.Errorf("MergeBase(c3,d2) = %s, want c1", base)
	}
}

func TestReachableCoversTreeAndParents(t *testing.T) {
	s := newStore(t)
	blob, _ := s.Put(Blob, []byte("data"))
	tree, _ := BuildTree(s, map[string]FileEntry{"f": {Hash: blob, Mode: ModeFile, Type: Blob}})
	c1, _ := s.PutCommit(CommitObj{Tree: tree, Message: "1"})
	c2, _ := s.PutCommit(CommitObj{Tree: tree, Parents: []string{c1}, Message: "2"})

	objs, err := s.Reachable(c2)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{c1, c2, tree, blob} {
		if !objs[h] {
			t.Errorf("reachable set missing %s", h)
		}
	}
}

// TestDeepTreeRefused proves the tree walkers reject a tree nested past
// maxTreeDepth instead of overflowing the stack (a DoS vector: a remote can
// push such a tree and the server walks it during reachability checks).
func TestDeepTreeRefused(t *testing.T) {
	s := newStore(t)
	parts := make([]string, maxTreeDepth+5)
	for i := range parts {
		parts[i] = "a"
	}
	deepPath := strings.Join(parts, "/") + "/f"
	blob, _ := s.Put(Blob, []byte("x"))
	tree, err := BuildTree(s, map[string]FileEntry{deepPath: {Hash: blob, Mode: ModeFile, Type: Blob}})
	if err != nil {
		t.Fatalf("build deep tree: %v", err)
	}
	if _, err := FlattenFull(s, tree); !errors.Is(err, errTreeTooDeep) {
		t.Fatalf("FlattenFull on a too-deep tree = %v, want errTreeTooDeep", err)
	}
	if _, err := Flatten(s, tree); !errors.Is(err, errTreeTooDeep) {
		t.Fatalf("Flatten on a too-deep tree = %v, want errTreeTooDeep", err)
	}
	// Reachable (server-side path) must also refuse, not crash.
	commit, err := s.PutCommit(CommitObj{Tree: tree, Author: "x", Email: "x@e", Message: "deep"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reachable(commit); !errors.Is(err, errTreeTooDeep) {
		t.Fatalf("Reachable on a too-deep tree = %v, want errTreeTooDeep", err)
	}
}
