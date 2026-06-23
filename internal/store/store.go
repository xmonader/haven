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
const SchemaVersion = 3

// migrations transform the database from version N to N+1. migrations[n] is the
// step that upgrades a v(n) database to v(n+1). The base `schema` above defines
// v1, so the first entry is migrations[1]: v1 -> v2.
// Each migration runs inside the SAME transaction that stamps the new
// PRAGMA user_version (see migrate), so the content/schema change and the
// version bump commit or roll back together. This is critical: if they could
// commit separately, a crash between them could leave a rewritten database
// stamped at the old version, and re-running the step would corrupt it (e.g.
// double-encoding every object). Steps must therefore also be idempotent where
// cheap, as defence in depth.
var migrations = map[int]func(*sql.Tx) error{
	1: migrateV2Compress,
	2: migrateV3CreatedAt,
}

// migrateV3CreatedAt adds an object creation timestamp so gc can apply a grace
// period (never prune freshly-written objects, closing the race where gc sweeps
// an in-flight commit/push before its ref update lands). Existing rows default
// to 0 (epoch) — they predate this binary and are immediately prune-eligible.
// Idempotent: if the column is already present (e.g. a re-run upgrade), it does
// nothing rather than failing on a duplicate column.
func migrateV3CreatedAt(tx *sql.Tx) error {
	has, err := columnExists(tx, "objects", "created_at")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	_, err = tx.Exec(`ALTER TABLE objects ADD COLUMN created_at INTEGER NOT NULL DEFAULT 0`)
	return err
}

// columnExists reports whether table has a column named col.
func columnExists(tx *sql.Tx, table, col string) (bool, error) {
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == col {
			return true, nil
		}
	}
	return false, rows.Err()
}

// migrateV2Compress re-stores every existing object through the v2 codec
// (one-byte tag + optional zlib). v1 rows hold bare payloads; rewriting them
// makes all rows self-describing so Decode works uniformly. It runs inside the
// caller's transaction (which also bumps user_version), so the rewrite and the
// version stamp are atomic — there is no window where rewritten content is
// stamped v1 and re-encoded on a re-run. Rows are buffered and the cursor closed
// before the updates because a single connection can't read and write at once.
func migrateV2Compress(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT hash, content FROM objects`)
	if err != nil {
		return err
	}
	type row struct {
		h string
		c []byte
	}
	var all []row
	for rows.Next() {
		var h string
		var c []byte
		if err := rows.Scan(&h, &c); err != nil {
			rows.Close()
			return err
		}
		all = append(all, row{h, c})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range all {
		if _, err := tx.Exec(`UPDATE objects SET content=? WHERE hash=?`, Encode(r.c), r.h); err != nil {
			return err
		}
	}
	return nil
}

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
		// Run the step and stamp the new version in ONE transaction so they are
		// atomic: a crash or failed stamp can never leave a migrated database
		// recorded at the old version (which would corrupt it on a re-run).
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration v%d->v%d: %w", v, v+1, err)
		}
		if err := step(tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrate v%d->v%d: %w", v, v+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", v+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("stamp schema v%d: %w", v+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration v%d->v%d: %w", v, v+1, err)
		}
		v++
	}
	return nil
}
