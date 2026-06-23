// Package object defines Haven's content-addressable object model
// (blob, tree, commit) and the SQLite-backed store that holds them.
package object

import (
	"database/sql"
	"fmt"

	"haven/internal/hash"
	"haven/internal/store"
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
	// Policy is a signed access-policy version (JSON).
	Policy Type = "policy"
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
		h, string(t), len(payload), store.Encode(payload),
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
		hash, string(t), len(content), store.Encode(content),
	)
	if err != nil {
		return fmt.Errorf("put raw %s %s: %w", t, hash, err)
	}
	return nil
}

// ReplaceContent overwrites the stored bytes of an existing object, keeping its
// hash. Only meaningful for Secret objects, whose hash is over the plaintext, so
// rotating to a new ciphertext (or recipient set) preserves identity. No-op when
// the hash is absent. (PutRaw is INSERT OR IGNORE and cannot rewrite, by design,
// for idempotent wire receipt.)
func (s *Store) ReplaceContent(h string, content []byte) error {
	_, err := s.db.Exec(`UPDATE objects SET content=?, size=? WHERE hash=?`, store.Encode(content), len(content), h)
	if err != nil {
		return fmt.Errorf("replace content %s: %w", h, err)
	}
	return nil
}

// PutSecret stores a Secret object, overwriting any existing ciphertext for the
// same (plaintext-derived) hash. Unlike PutRaw, this propagates a rotated
// ciphertext: the hash is stable across re-encryption, so receiving a rotated
// secret must replace the stored bytes rather than ignore them.
func (s *Store) PutSecret(hash string, content []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO objects(hash, type, size, content) VALUES(?,?,?,?)
		 ON CONFLICT(hash) DO UPDATE SET content=excluded.content, size=excluded.size`,
		hash, string(Secret), len(content), store.Encode(content),
	)
	if err != nil {
		return fmt.Errorf("put secret %s: %w", hash, err)
	}
	return nil
}

// maxDeltaDepth bounds delta-base chains so a corrupt or maliciously circular
// store cannot cause unbounded recursion on read. repack only ever creates
// depth-1 deltas (bases are full objects), so this is a generous safety margin.
const maxDeltaDepth = 50

// Get returns the type and payload of an object, transparently reconstructing
// objects stored as deltas against a base.
func (s *Store) Get(h string) (Type, []byte, error) {
	return s.get(h, 0)
}

func (s *Store) get(h string, depth int) (Type, []byte, error) {
	if depth > maxDeltaDepth {
		return "", nil, fmt.Errorf("object %s: delta chain exceeds depth %d", h, maxDeltaDepth)
	}
	var t string
	var stored []byte
	err := s.db.QueryRow(`SELECT type, content FROM objects WHERE hash=?`, h).Scan(&t, &stored)
	if err == sql.ErrNoRows {
		return "", nil, fmt.Errorf("object %s: not found", h)
	}
	if err != nil {
		return "", nil, err
	}
	payload, err := s.decodeStored(h, stored, depth)
	if err != nil {
		return "", nil, err
	}
	return Type(t), payload, nil
}

// decodeStored turns raw stored bytes into the object payload, resolving a delta
// envelope by fetching and applying against its base.
func (s *Store) decodeStored(h string, stored []byte, depth int) ([]byte, error) {
	baseHash, delta, isDelta, err := store.DecodeDelta(stored)
	if err != nil {
		return nil, fmt.Errorf("object %s: %w", h, err)
	}
	if !isDelta {
		payload, err := store.Decode(stored)
		if err != nil {
			return nil, fmt.Errorf("object %s: %w", h, err)
		}
		return payload, nil
	}
	_, base, err := s.get(baseHash, depth+1)
	if err != nil {
		return nil, fmt.Errorf("object %s: delta base: %w", h, err)
	}
	payload, err := store.ApplyDelta(base, delta)
	if err != nil {
		return nil, fmt.Errorf("object %s: %w", h, err)
	}
	return payload, nil
}

// Each calls fn for every stored object with its reconstructed payload. fn must
// not retain content. Rows are buffered before reconstruction so resolving a
// delta's base does not run a query against the still-open cursor.
func (s *Store) Each(fn func(hash string, t Type, content []byte) error) error {
	type row struct {
		h, t   string
		stored []byte
	}
	rows, err := s.db.Query(`SELECT hash, type, content FROM objects`)
	if err != nil {
		return err
	}
	var buf []row
	for rows.Next() {
		var rw row
		if err := rows.Scan(&rw.h, &rw.t, &rw.stored); err != nil {
			rows.Close()
			return err
		}
		buf = append(buf, rw)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, rw := range buf {
		content, err := s.decodeStored(rw.h, rw.stored, 0)
		if err != nil {
			return err
		}
		if err := fn(rw.h, Type(rw.t), content); err != nil {
			return err
		}
	}
	return nil
}

// AllHashes returns every stored object hash.
func (s *Store) AllHashes() ([]string, error) {
	rows, err := s.db.Query(`SELECT hash FROM objects`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// Meta is lightweight per-object metadata for maintenance tasks (repack),
// gathered without loading full content.
type Meta struct {
	Hash    string
	Type    Type
	Size    int // logical payload size
	IsDelta bool
}

// Metas returns metadata for every object, reading only the first content byte
// to classify delta vs whole — never the whole payload.
func (s *Store) Metas() ([]Meta, error) {
	rows, err := s.db.Query(`SELECT hash, type, size, substr(content,1,1) FROM objects`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Meta
	for rows.Next() {
		var m Meta
		var t string
		var tag []byte
		if err := rows.Scan(&m.Hash, &t, &m.Size, &tag); err != nil {
			return nil, err
		}
		m.Type = Type(t)
		m.IsDelta = store.IsDelta(tag)
		out = append(out, m)
	}
	return out, rows.Err()
}

// Delete removes an object by hash.
func (s *Store) Delete(hash string) error {
	_, err := s.db.Exec(`DELETE FROM objects WHERE hash=?`, hash)
	return err
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

// StoreAsDelta rewrites object h to be stored as a delta against baseHash,
// preserving its hash and logical size. Guarantees:
//   - It refuses (error) if the base is itself a delta or is h, so read chains
//     stay at depth 1.
//   - It reconstructs the envelope byte-for-byte and refuses (error) on any
//     mismatch, so a delta-encoder bug can never corrupt the store.
//   - It is a no-op (applied=false) when the delta would not be smaller than the
//     object's current stored form, so it never bloats the store.
func (s *Store) StoreAsDelta(h, baseHash string) (applied bool, err error) {
	if h == baseHash {
		return false, fmt.Errorf("store-as-delta: object cannot be its own base")
	}
	_, payload, err := s.Get(h)
	if err != nil {
		return false, err
	}
	if isDelta, err := s.isDeltaRow(baseHash); err != nil {
		return false, err
	} else if isDelta {
		return false, fmt.Errorf("store-as-delta: base %s is itself a delta", baseHash)
	}
	curSize, err := s.StoredSize(h)
	if err != nil {
		return false, err
	}
	_, base, err := s.Get(baseHash)
	if err != nil {
		return false, fmt.Errorf("store-as-delta: base: %w", err)
	}
	envelope := store.EncodeDelta(baseHash, store.MakeDelta(base, payload))
	if len(envelope) >= curSize {
		return false, nil // delta buys nothing; leave the object whole
	}

	// Self-verify before committing: reconstruct from the envelope and demand it
	// equals the original payload. Never trust the encoder blindly with the store.
	reconstructed, err := s.decodeStored(h, envelope, 0)
	if err != nil {
		return false, fmt.Errorf("store-as-delta: self-check decode: %w", err)
	}
	if string(reconstructed) != string(payload) {
		return false, fmt.Errorf("store-as-delta: self-check mismatch for %s; refusing to rewrite", h)
	}

	if _, err := s.db.Exec(`UPDATE objects SET content=? WHERE hash=?`, envelope, h); err != nil {
		return false, fmt.Errorf("store-as-delta %s: %w", h, err)
	}
	return true, nil
}

// DeltaBase returns the base hash if h is stored as a delta, else ok=false.
func (s *Store) DeltaBase(h string) (string, bool, error) {
	var stored []byte
	err := s.db.QueryRow(`SELECT content FROM objects WHERE hash=?`, h).Scan(&stored)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	base, _, ok, err := store.DecodeDelta(stored)
	return base, ok, err
}

func (s *Store) isDeltaRow(h string) (bool, error) {
	var stored []byte
	err := s.db.QueryRow(`SELECT content FROM objects WHERE hash=?`, h).Scan(&stored)
	if err == sql.ErrNoRows {
		return false, fmt.Errorf("object %s: not found", h)
	}
	if err != nil {
		return false, err
	}
	return store.IsDelta(stored), nil
}

// StoredSize returns the number of bytes object h occupies on disk (the encoded
// content column), as opposed to its logical payload size.
func (s *Store) StoredSize(h string) (int, error) {
	var stored []byte
	err := s.db.QueryRow(`SELECT content FROM objects WHERE hash=?`, h).Scan(&stored)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("object %s: not found", h)
	}
	if err != nil {
		return 0, err
	}
	return len(stored), nil
}
