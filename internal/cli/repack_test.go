package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haven/internal/lock"
	"haven/internal/object"
	"haven/internal/protocol"
	"haven/internal/repo"
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

// TestRepackedRepoClonesIntact proves the wire still carries whole objects after
// repack: a repo with delta-stored objects must push and clone with byte-exact
// content (a peer that lacks a delta base must never receive a raw delta).
func TestRepackedRepoClonesIntact(t *testing.T) {
	url := startServer(t, protocol.KindTeam)

	work := t.TempDir()
	run(t, work, "init")
	run(t, work, "config", "user.name", "Dev")

	var v strings.Builder
	for i := 0; i < 400; i++ {
		v.WriteString("\tvalue := transform(record, schema, opts)\n")
	}
	file := filepath.Join(work, "big.go")
	os.WriteFile(file, []byte(v.String()), 0o644)
	run(t, work, "add", ".")
	run(t, work, "commit", "-m", "v1")
	v2 := v.String() + "\t// appended in v2\n"
	os.WriteFile(file, []byte(v2), 0o644)
	run(t, work, "add", ".")
	run(t, work, "commit", "-m", "v2")

	if out, code := run(t, work, "repack"); code != 0 || strings.Contains(out, "repacked 0 object") {
		t.Fatalf("repack did not deltify:\n%s", out)
	}

	run(t, work, "remote", "add", "origin", url, "--kind", "team")
	if out, code := run(t, work, "push", "origin", "main"); code != 0 {
		t.Fatalf("push of repacked repo failed:\n%s", out)
	}

	cloneParent := t.TempDir()
	if out, code := run(t, cloneParent, "clone", url, "c2"); code != 0 {
		t.Fatalf("clone failed:\n%s", out)
	}
	got, err := os.ReadFile(filepath.Join(cloneParent, "c2", "big.go"))
	if err != nil || string(got) != v2 {
		t.Fatalf("cloned content mismatch (err=%v, len got=%d want=%d)", err, len(got), len(v2))
	}
	if out, code := run(t, filepath.Join(cloneParent, "c2"), "fsck"); code != 0 {
		t.Fatalf("fsck on clone of repacked repo failed:\n%s", out)
	}
}

// TestFsckCatchesCorruptDeltaBase corrupts the base of a delta after repack and
// verifies fsck reports corruption rather than silently serving wrong content.
func TestFsckCatchesCorruptDeltaBase(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "T")
	var v strings.Builder
	for i := 0; i < 400; i++ {
		v.WriteString("\tx := step(a, b, c, d, e)\n")
	}
	file := filepath.Join(dir, "big.go")
	os.WriteFile(file, []byte(v.String()), 0o644)
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "v1")
	os.WriteFile(file, []byte(v.String()+"\t// v2\n"), 0o644)
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "v2")
	if out, code := run(t, dir, "repack"); code != 0 || strings.Contains(out, "repacked 0 object") {
		t.Fatalf("repack did not deltify:\n%s", out)
	}

	// Find a delta object and corrupt its base directly in the DB.
	dbPath := filepath.Join(dir, ".haven", "haven.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	st := object.NewStore(db)
	metas, err := st.Metas()
	if err != nil {
		t.Fatal(err)
	}
	var baseHash string
	for _, m := range metas {
		if m.IsDelta {
			b, ok, err := st.DeltaBase(m.Hash)
			if err == nil && ok {
				baseHash = b
				break
			}
		}
	}
	if baseHash == "" {
		db.Close()
		t.Fatal("no delta object found after repack")
	}
	if _, err := db.Exec(`UPDATE objects SET content=? WHERE hash=?`, []byte{0, 9, 9, 9, 9, 9}, baseHash); err != nil {
		t.Fatal(err)
	}
	db.Close()

	if out, code := run(t, dir, "fsck"); code == 0 {
		t.Fatalf("fsck passed despite a corrupted delta base:\n%s", out)
	}
}

// TestRepackAndGcRefuseWhenLocked proves the corruption-preventing serialization:
// while the repo lock is held, both repack and gc fail fast rather than racing
// (the gc/repack race could otherwise delete a delta's base).
func TestRepackAndGcRefuseWhenLocked(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "T")
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hi\n"), 0o644)
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "c1")

	r, err := repo.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	held, err := lock.Acquire(r.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()

	if out, code := run(t, dir, "repack"); code == 0 {
		t.Fatalf("repack should fail while repo is locked:\n%s", out)
	}
	if out, code := run(t, dir, "gc"); code == 0 {
		t.Fatalf("gc should fail while repo is locked:\n%s", out)
	}
}
