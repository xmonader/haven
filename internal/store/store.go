// Package store owns the SQLite database: connection, schema, and migrations.
// Every repository is a single .haven/haven.db file.
package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// schema is the full v1 layout. Applied idempotently on open.
const schema = `
CREATE TABLE IF NOT EXISTS objects (
	hash    TEXT PRIMARY KEY,
	type    TEXT NOT NULL,           -- blob | tree | commit
	size    INTEGER NOT NULL,
	content BLOB NOT NULL
);

CREATE TABLE IF NOT EXISTS refs (
	name       TEXT PRIMARY KEY,     -- e.g. refs/branches/main, refs/havens/scratch
	visibility TEXT NOT NULL,        -- public | restricted | private | policy
	target     TEXT NOT NULL,        -- commit hash (or "" for unborn)
	mtime      INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS remotes (
	name TEXT PRIMARY KEY,
	url  TEXT NOT NULL,
	kind TEXT NOT NULL               -- team | personal
);

CREATE TABLE IF NOT EXISTS config (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS staging (
	path TEXT PRIMARY KEY,
	hash TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS secret_manifest (
	ref            TEXT NOT NULL,
	name           TEXT NOT NULL,
	object         TEXT NOT NULL,
	policy_version INTEGER NOT NULL,
	PRIMARY KEY (ref, name)
);
`

// Open opens (creating if needed) the SQLite database at path and ensures the
// schema is present. WAL mode and foreign keys are enabled for safe concurrent
// reads and atomic writes.
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// modernc/sqlite is not goroutine-safe across a single connection for writes;
	// the busy_timeout pragma plus a single shared *sql.DB pool is sufficient here.
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return db, nil
}
