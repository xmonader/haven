package protocol

import (
	"net/http/httptest"
	"testing"

	"haven/internal/object"
	"haven/internal/ref"
	"haven/internal/store"
)

func newServer(t *testing.T, kind string) (*Client, *object.Store) {
	t.Helper()
	db, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	srv := httptest.NewServer(NewServer(db, kind).Handler())
	t.Cleanup(srv.Close)
	return NewClient(srv.URL), object.NewStore(db)
}

func TestObjectRoundtripOverHTTP(t *testing.T) {
	c, _ := newServer(t, KindTeam)
	content := []byte("hello over http")
	realHash := objectHash(content)
	if err := c.PutObject(realHash, object.Blob, content); err != nil {
		t.Fatalf("put: %v", err)
	}

	typ, got, err := c.GetObject(realHash)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if typ != object.Blob || string(got) != string(content) {
		t.Errorf("roundtrip got (%s,%q)", typ, got)
	}
}

func TestPutObjectRejectsHashMismatch(t *testing.T) {
	c, _ := newServer(t, KindTeam)
	if err := c.PutObject("0000000000000000000000000000000000000000000000000000000000000000",
		object.Blob, []byte("x")); err == nil {
		t.Fatal("expected hash mismatch error")
	}
}

func TestTeamServerRefusesPrivateRef(t *testing.T) {
	c, _ := newServer(t, KindTeam)
	err := c.UpdateRef(RefUpdate{Name: "refs/havens/secret", Visibility: ref.Private, Target: "abc"})
	if err == nil {
		t.Fatal("team server must refuse a private ref")
	}
}

func TestPersonalServerAcceptsPrivateRef(t *testing.T) {
	c, _ := newServer(t, KindPersonal)
	err := c.UpdateRef(RefUpdate{Name: "refs/havens/secret", Visibility: ref.Private, Target: ""})
	if err != nil {
		t.Fatalf("personal server should accept private refs: %v", err)
	}
}

func TestConditionalRefUpdateConflict(t *testing.T) {
	c, _ := newServer(t, KindTeam)
	// First create succeeds (old_target "").
	if err := c.UpdateRef(RefUpdate{Name: "refs/branches/main", Visibility: ref.Public, Target: "v1"}); err != nil {
		t.Fatal(err)
	}
	// Stale old_target should conflict.
	if err := c.UpdateRef(RefUpdate{Name: "refs/branches/main", Visibility: ref.Public, Target: "v2", OldTarget: "WRONG"}); err == nil {
		t.Fatal("expected conditional update conflict")
	}
}

// objectHash mirrors hash.Of for blobs without importing the hash package into
// the test's expectations; it stores then reads back the canonical hash.
func objectHash(content []byte) string {
	db, _ := store.Open(":memory:")
	defer db.Close()
	s := object.NewStore(db)
	h, _ := s.Put(object.Blob, content)
	return h
}
