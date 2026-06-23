// Package object defines Haven's content-addressable object model
// (blob, tree, commit) and the SQLite-backed store that holds them.
package object

import (
	"database/sql"
	"fmt"

	"haven/internal/hash"
)

// Type is an object kind.
type Type string

const (
	Blob   Type = "blob"
	Tree   Type = "tree"
	Commit Type = "commit"
	// Secret is an age-encrypted file. Its store hash is computed over the
	// PLAINTEXT (so its identity is stable across re-encryption), while the
	// stored content is ciphertext the server can never read.
	Secret Type = "secret"
)

// Store reads and writes objects to the database.
type Store struct {
	db *sql.DB
}

// NewStore wraps a database handle.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Put stores a payload under its content hash and returns the hash. Storing an
// object that already exists is a no-op (content addressing makes it safe).
func (s *Store) Put(t Type, payload []byte) (string, error) {
	h := hash.Of(string(t), payload)
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO objects(hash, type, size, content) VALUES(?,?,?,?)`,
		h, string(t), len(payload), payload,
	)
	if err != nil {
		return "", fmt.Errorf("put %s %s: %w", t, h, err)
	}
	return h, nil
}

// PutRaw stores content under a caller-supplied hash without recomputing it.
// Used for Secret objects (whose hash is over plaintext, not the stored
// ciphertext) and for receiving such objects over the wire, where the verifier
// cannot recompute the hash. Idempotent.
func (s *Store) PutRaw(hash string, t Type, content []byte) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO objects(hash, type, size, content) VALUES(?,?,?,?)`,
		hash, string(t), len(content), content,
	)
	if err != nil {
		return fmt.Errorf("put raw %s %s: %w", t, hash, err)
	}
	return nil
}

// Get returns the type and payload of an object.
func (s *Store) Get(h string) (Type, []byte, error) {
	var t string
	var payload []byte
	err := s.db.QueryRow(`SELECT type, content FROM objects WHERE hash=?`, h).Scan(&t, &payload)
	if err == sql.ErrNoRows {
		return "", nil, fmt.Errorf("object %s: not found", h)
	}
	if err != nil {
		return "", nil, err
	}
	return Type(t), payload, nil
}

// Has reports whether an object exists.
func (s *Store) Has(h string) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM objects WHERE hash=?`, h).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}
