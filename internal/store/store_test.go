package store

import (
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
