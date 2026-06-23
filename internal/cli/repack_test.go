package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haven/internal/object"
	"haven/internal/store"
)

// TestRepackPreservesIntegrity proves the full delta path end-to-end: two
// versions of a large file are committed, repack deltifies one against the
// other, and fsck (which reconstructs and re-hashes every object) plus a content
// read plus gc all still hold.
func TestRepackPreservesIntegrity(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Tester")

	var v1 strings.Builder
	for i := 0; i < 400; i++ {
		v1.WriteString("\tx := lookup(table, key) + adjust(base, offset)\n")
	}
	file := filepath.Join(dir, "big.go")
	if err := os.WriteFile(file, []byte(v1.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "add", ".")
	if _, code := run(t, dir, "commit", "-m", "v1"); code != 0 {
		t.Fatal("commit v1")
	}

	v2 := v1.String() + "\t// a small change in v2 — most bytes are shared\n"
	if err := os.WriteFile(file, []byte(v2), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "add", ".")
	if _, code := run(t, dir, "commit", "-m", "v2"); code != 0 {
		t.Fatal("commit v2")
	}

	out, code := run(t, dir, "repack")
	if code != 0 {
		t.Fatalf("repack exit %d:\n%s", code, out)
	}
	if !strings.Contains(out, "repacked") {
		t.Fatalf("repack output:\n%s", out)
	}
	// At least one object should have been deltified (the two big.go versions).
	if strings.Contains(out, "repacked 0 object") {
		t.Fatalf("expected at least one delta; got:\n%s", out)
	}

	// fsck reconstructs and re-hashes every object — the strongest integrity check.
	if out, code := run(t, dir, "fsck"); code != 0 {
		t.Fatalf("fsck after repack exit %d:\n%s", code, out)
	}

	// The working file content must still read back exactly (via restore).
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if out, code := run(t, dir, "restore", "big.go"); code != 0 {
		t.Fatalf("restore exit %d:\n%s", code, out)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != v2 {
		t.Fatalf("restored content mismatch after repack")
	}

	// gc must keep the delta base; fsck must still pass afterward.
	if out, code := run(t, dir, "gc"); code != 0 {
		t.Fatalf("gc exit %d:\n%s", code, out)
	}
	if out, code := run(t, dir, "fsck"); code != 0 {
		t.Fatalf("fsck after gc exit %d:\n%s", code, out)
	}
}

// TestGcKeepsDeltaBase verifies the dangerous case directly: a reachable object
// stored as a delta against a base that nothing else references must keep that
// base in the live set.
func TestGcKeepsDeltaBase(t *testing.T) {
	s := newGcTestStore(t)
	var b []byte
	for i := 0; i < 300; i++ {
		b = append(b, []byte("\tshared line of content for the base object\n")...)
	}
	baseHash, err := s.Put(object.Blob, b)
	if err != nil {
		t.Fatal(err)
	}
	targetHash, err := s.Put(object.Blob, append(append([]byte{}, b...), []byte("\t// delta tail\n")...))
	if err != nil {
		t.Fatal(err)
	}
	applied, err := s.StoreAsDelta(targetHash, baseHash)
	if err != nil || !applied {
		t.Fatalf("StoreAsDelta applied=%v err=%v", applied, err)
	}

	// Only the delta object is reachable; its base is not directly referenced.
	reachable := map[string]bool{targetHash: true}
	if err := addDeltaBases(s, reachable); err != nil {
		t.Fatal(err)
	}
	if !reachable[baseHash] {
		t.Fatal("addDeltaBases failed to retain the base of a reachable delta")
	}
}

func newGcTestStore(t *testing.T) *object.Store {
	t.Helper()
	db, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return object.NewStore(db)
}
