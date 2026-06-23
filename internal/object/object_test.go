package object

import (
	"reflect"
	"testing"

	"haven/internal/store"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db)
}

func TestBlobRoundtrip(t *testing.T) {
	s := newTestStore(t)
	h, err := s.Put(Blob, []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	typ, payload, err := s.Get(h)
	if err != nil {
		t.Fatal(err)
	}
	if typ != Blob || string(payload) != "hello" {
		t.Errorf("got (%s,%q), want (blob,hello)", typ, payload)
	}
}

func TestContentAddressingIsDeterministic(t *testing.T) {
	s := newTestStore(t)
	h1, _ := s.Put(Blob, []byte("same"))
	h2, _ := s.Put(Blob, []byte("same"))
	if h1 != h2 {
		t.Errorf("identical content hashed differently: %s vs %s", h1, h2)
	}
}

func TestTreeRoundtrip(t *testing.T) {
	s := newTestStore(t)
	entries := []TreeEntry{
		{Mode: ModeFile, Type: Blob, Hash: "aaa", Name: "z.txt"},
		{Mode: ModeFile, Type: Blob, Hash: "bbb", Name: "a.txt"},
		{Mode: ModeTree, Type: Tree, Hash: "ccc", Name: "sub"},
	}
	h, err := s.PutTree(entries)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetTree(h)
	if err != nil {
		t.Fatal(err)
	}
	// Stored sorted by name: a.txt, sub, z.txt
	want := []TreeEntry{
		{Mode: ModeFile, Type: Blob, Hash: "bbb", Name: "a.txt"},
		{Mode: ModeTree, Type: Tree, Hash: "ccc", Name: "sub"},
		{Mode: ModeFile, Type: Blob, Hash: "aaa", Name: "z.txt"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tree roundtrip:\n got %+v\nwant %+v", got, want)
	}
}

func TestCommitRoundtrip(t *testing.T) {
	s := newTestStore(t)
	c := CommitObj{
		Tree:    "treehash",
		Parents: []string{"p1", "p2"},
		Author:  "Ada Lovelace",
		Email:   "ada@example.com",
		When:    1700000000,
		Message: "first commit\n\nwith body",
	}
	h, err := s.PutCommit(c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetCommit(h)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, c) {
		t.Errorf("commit roundtrip:\n got %+v\nwant %+v", got, c)
	}
}
