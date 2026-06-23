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

CREATE TABLE IF NOT EXISTS members (
	name      TEXT PRIMARY KEY,   -- actor id
	recipient TEXT NOT NULL       -- age public recipient ("age1...")
);

CREATE TABLE IF NOT EXISTS secret_paths (
	glob TEXT PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS seen_nonces (
	nonce   TEXT PRIMARY KEY,   -- accepted signed-request nonce
	seen_at INTEGER NOT NULL    -- client unix time, for skew-window eviction
);
`

// SchemaVersion is the layout this binary speaks. It is stamped into the
// database's PRAGMA user_version on open. A database stamped higher than this
// was written by a newer hv and is refused rather than silently corrupted.
const SchemaVersion = 1

// migrations transform the database from version N to N+1. migrations[n] is the
// step that upgrades a v(n) database to v(n+1). The base `schema` above defines
// v1, so the first entry (if any) is migrations[1]: v1 -> v2.
var migrations = map[int]func(*sql.DB) error{}

// Open opens (creating if needed) the SQLite database at path and brings its
// schema up to SchemaVersion. WAL mode and foreign keys are enabled for safe
// concurrent reads and atomic writes.
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// modernc/sqlite is not goroutine-safe across a single connection for writes;
	// the busy_timeout pragma plus a single shared *sql.DB pool is sufficient here.
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// migrate applies the base schema idempotently, then runs any pending migrations
// to reach SchemaVersion, recording progress in PRAGMA user_version so each step
// runs at most once and an interrupted upgrade resumes where it left off.
func migrate(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if v > SchemaVersion {
		return fmt.Errorf("repository schema is v%d but this hv understands only up to v%d; upgrade hv", v, SchemaVersion)
	}
	if v < 1 {
		// Fresh database (tables just created) or a legacy DB written before
		// versioning existed — either way the layout above is v1.
		if _, err := db.Exec("PRAGMA user_version = 1"); err != nil {
			return fmt.Errorf("stamp schema v1: %w", err)
		}
		v = 1
	}
	for v < SchemaVersion {
		step, ok := migrations[v]
		if !ok {
			return fmt.Errorf("no migration registered for v%d->v%d", v, v+1)
		}
		if err := step(db); err != nil {
			return fmt.Errorf("migrate v%d->v%d: %w", v, v+1, err)
		}
		v++
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", v)); err != nil {
			return fmt.Errorf("stamp schema v%d: %w", v, err)
		}
	}
	return nil
}
