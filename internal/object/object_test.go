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

// bigBlob builds a realistically-sized source-like payload (delta storage only
// pays off above the 64-byte base-hash envelope overhead).
func bigBlob(extra string) []byte {
	var b []byte
	for i := 0; i < 300; i++ {
		b = append(b, []byte("\tresult := compute(input, options, context)\n")...)
	}
	return append(b, []byte(extra)...)
}

func TestStoreAsDeltaRoundtrip(t *testing.T) {
	s := newTestStore(t)
	base := bigBlob("")
	target := bigBlob("\t// one new line in v2\n")

	baseHash, err := s.Put(Blob, base)
	if err != nil {
		t.Fatal(err)
	}
	targetHash, err := s.Put(Blob, target)
	if err != nil {
		t.Fatal(err)
	}
	fullSize, _ := s.StoredSize(targetHash)

	applied, err := s.StoreAsDelta(targetHash, baseHash)
	if err != nil {
		t.Fatalf("StoreAsDelta: %v", err)
	}
	if !applied {
		t.Fatal("expected delta to apply for a large object with a small edit")
	}

	// Get must reconstruct the exact original payload and type.
	typ, got, err := s.Get(targetHash)
	if err != nil {
		t.Fatalf("Get after delta: %v", err)
	}
	if typ != Blob || string(got) != string(target) {
		t.Fatalf("reconstructed (%s,%q...), want blob/original", typ, got[:20])
	}

	// The delta must actually be smaller than the whole object.
	deltaSize, _ := s.StoredSize(targetHash)
	if deltaSize >= fullSize {
		t.Fatalf("delta storage %d not smaller than full %d", deltaSize, fullSize)
	}

	// DeltaBase must report the base; the base itself is not a delta.
	b, ok, err := s.DeltaBase(targetHash)
	if err != nil || !ok || b != baseHash {
		t.Fatalf("DeltaBase = (%q,%v,%v), want (%s,true,nil)", b, ok, err, baseHash)
	}
}

func TestStoreAsDeltaNoBloat(t *testing.T) {
	s := newTestStore(t)
	// Tiny, unrelated blobs: a delta cannot beat the envelope overhead, so the
	// store must leave them whole rather than grow them.
	bh, _ := s.Put(Blob, []byte("short base"))
	th, _ := s.Put(Blob, []byte("totally different short content"))
	applied, err := s.StoreAsDelta(th, bh)
	if err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Fatal("delta should not have been applied: it would bloat the object")
	}
	if _, ok, _ := s.DeltaBase(th); ok {
		t.Fatal("object was rewritten as a delta despite no savings")
	}
}

func TestStoreAsDeltaRefusesDeltaBase(t *testing.T) {
	s := newTestStore(t)
	base := bigBlob("")
	mid := bigBlob("\t// mid\n")
	leaf := bigBlob("\t// mid\n\t// leaf\n")
	bh, _ := s.Put(Blob, base)
	mh, _ := s.Put(Blob, mid)
	lh, _ := s.Put(Blob, leaf)

	applied, err := s.StoreAsDelta(mh, bh)
	if err != nil || !applied {
		t.Fatalf("first delta: applied=%v err=%v", applied, err)
	}
	// mid is now a delta; using it as a base must be refused (keeps chains depth-1).
	if _, err := s.StoreAsDelta(lh, mh); err == nil {
		t.Fatal("expected refusal: base is itself a delta")
	}
}

func TestEachReconstructsDeltas(t *testing.T) {
	s := newTestStore(t)
	base := bigBlob("")
	target := bigBlob("\t// extra\n")
	bh, _ := s.Put(Blob, base)
	th, _ := s.Put(Blob, target)
	applied, err := s.StoreAsDelta(th, bh)
	if err != nil || !applied {
		t.Fatalf("StoreAsDelta: applied=%v err=%v", applied, err)
	}
	seen := map[string]string{}
	err = s.Each(func(h string, typ Type, content []byte) error {
		seen[h] = string(content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen[th] != string(target) {
		t.Fatalf("Each gave wrong delta object content")
	}
	if seen[bh] != string(base) {
		t.Fatalf("Each gave wrong base content")
	}
}
