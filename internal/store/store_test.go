package store

import (
	"bytes"
	"fmt"
	"testing"
)

// TestOpenStampsVersion verifies a freshly created database is stamped at the
// current schema version, and that reopening it is idempotent.
func TestOpenStampsVersion(t *testing.T) {
	path := t.TempDir() + "/t.db"
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != SchemaVersion {
		t.Fatalf("fresh db stamped v%d, want v%d", v, SchemaVersion)
	}
	db.Close()

	// Reopening an already-current database must succeed and keep the version.
	db2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	db2.QueryRow("PRAGMA user_version").Scan(&v)
	if v != SchemaVersion {
		t.Fatalf("reopened db v%d, want v%d", v, SchemaVersion)
	}
}

// TestOpenRefusesNewerSchema proves a database written by a newer hv (a higher
// user_version) is refused rather than silently mis-read.
func TestOpenRefusesNewerSchema(t *testing.T) {
	path := t.TempDir() + "/t.db"
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Pretend a future hv bumped the schema past what we understand.
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion+1)); err != nil {
		t.Fatal(err)
	}
	db.Close()

	if _, err := Open(path); err == nil {
		t.Fatal("expected refusal of a newer schema version, got nil error")
	}
}

// TestCodecRoundTrip verifies Encode/Decode is lossless for empty, raw-preferred
// (tiny/high-entropy), and compressible payloads, and that compressible data is
// actually stored smaller while incompressible data is never enlarged.
func TestCodecRoundTrip(t *testing.T) {
	compressible := bytes.Repeat([]byte("haven "), 4096) // ~24 KiB, highly redundant
	highEntropy := make([]byte, 4096)
	for i := range highEntropy {
		highEntropy[i] = byte(i * 31) // deterministic, low redundancy
	}
	cases := map[string][]byte{
		"empty":        {},
		"tiny":         []byte("x"),
		"compressible": compressible,
		"highEntropy":  highEntropy,
	}
	for name, payload := range cases {
		enc := Encode(payload)
		got, err := Decode(enc)
		if err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("%s: round trip mismatch (%d != %d bytes)", name, len(got), len(payload))
		}
		if len(enc) > len(payload)+1 {
			t.Fatalf("%s: encoding enlarged payload: %d > %d+1", name, len(enc), len(payload))
		}
	}
	if enc := Encode(compressible); len(enc) >= len(compressible) {
		t.Fatalf("compressible payload was not compressed: %d >= %d", len(enc), len(compressible))
	}
}

// TestMigrateV2RewritesLegacyRows simulates a v1 database (bare, untagged
// content) and proves reopening upgrades it: rows still decode to their original
// bytes through the v2 codec.
func TestMigrateV2RewritesLegacyRows(t *testing.T) {
	path := t.TempDir() + "/t.db"
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Force the DB back to a v1 state and insert a row the v1 way: bare payload.
	payload := bytes.Repeat([]byte("legacy content "), 1000)
	if _, err := db.Exec(`INSERT INTO objects(hash,type,size,content) VALUES(?,?,?,?)`,
		"deadbeef", "blob", len(payload), payload); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA user_version = 1"); err != nil {
		t.Fatal(err)
	}
	db.Close()

	// Reopen: migrate v1->v2 must rewrite the legacy row through Encode.
	db2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen (migrate): %v", err)
	}
	defer db2.Close()
	var v int
	db2.QueryRow("PRAGMA user_version").Scan(&v)
	if v != SchemaVersion {
		t.Fatalf("after migrate v%d, want v%d", v, SchemaVersion)
	}
	var stored []byte
	if err := db2.QueryRow(`SELECT content FROM objects WHERE hash=?`, "deadbeef").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	got, err := Decode(stored)
	if err != nil {
		t.Fatalf("decode migrated row: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("migrated row does not decode to original payload")
	}
}

// TestUserVersionIsTransactional proves the assumption the migration-atomicity
// fix relies on: PRAGMA user_version set inside a transaction commits/rolls back
// WITH the transaction. If this driver didn't honour that, the migration step
// and its version stamp could diverge and corrupt the DB on a re-run.
func TestUserVersionIsTransactional(t *testing.T) {
	db, err := Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	read := func() int {
		var v int
		if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	start := read()

	// Rollback must revert the version bump.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("PRAGMA user_version = 999"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if got := read(); got != start {
		t.Fatalf("user_version after rollback = %d, want %d (PRAGMA not transactional!)", got, start)
	}

	// Commit must persist it.
	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("PRAGMA user_version = 7"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := read(); got != 7 {
		t.Fatalf("user_version after commit = %d, want 7", got)
	}
}
