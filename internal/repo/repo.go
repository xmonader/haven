// Package repo handles repository discovery, initialization, and the on-disk
// layout under .haven/.
package repo

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"haven/internal/store"
)

// Dir names within a repository.
const (
	Dir      = ".haven"   // repository metadata directory
	dbName   = "haven.db" // SQLite database
	headName = "HEAD"     // symbolic ref to the current branch/haven
)

// DefaultBranch is the ref a fresh repository starts on.
const DefaultBranch = "refs/branches/main"

// ErrNotARepo is returned when no .haven directory is found.
var ErrNotARepo = errors.New("not a haven repository (no .haven directory)")

// ErrExists is returned by Init when a repository already exists.
var ErrExists = errors.New("haven repository already exists")

// Repo is an open repository handle.
type Repo struct {
	Root string  // absolute path to the working tree root (parent of .haven)
	DB   *sql.DB // open object/ref store
}

// metaDir returns the .haven path for a given root.
func metaDir(root string) string { return filepath.Join(root, Dir) }
func dbPath(root string) string  { return filepath.Join(metaDir(root), dbName) }
func headPath(root string) string {
	return filepath.Join(metaDir(root), headName)
}

// Init creates a new repository rooted at dir. It is an error if one already
// exists. The repository starts on DefaultBranch with no commits (unborn HEAD).
func Init(dir string) (*Repo, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	meta := metaDir(abs)
	if _, err := os.Stat(meta); err == nil {
		return nil, ErrExists
	}
	if err := os.MkdirAll(meta, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", meta, err)
	}
	db, err := store.Open(dbPath(abs))
	if err != nil {
		return nil, err
	}
	if err := writeHead(abs, DefaultBranch); err != nil {
		db.Close()
		return nil, err
	}
	return &Repo{Root: abs, DB: db}, nil
}

// Open finds the repository containing dir by walking up to the filesystem
// root, then opens its database.
func Open(dir string) (*Repo, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	root, err := discover(abs)
	if err != nil {
		return nil, err
	}
	db, err := store.Open(dbPath(root))
	if err != nil {
		return nil, err
	}
	return &Repo{Root: root, DB: db}, nil
}

// discover walks up from start looking for a .haven directory.
func discover(start string) (string, error) {
	cur := start
	for {
		if fi, err := os.Stat(metaDir(cur)); err == nil && fi.IsDir() {
			return cur, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", ErrNotARepo
		}
		cur = parent
	}
}

// Close releases the database handle.
func (r *Repo) Close() error {
	if r.DB == nil {
		return nil
	}
	return r.DB.Close()
}

// Head returns the symbolic ref name HEAD points at (e.g. refs/branches/main).
func (r *Repo) Head() (string, error) {
	return readHead(r.Root)
}

func writeHead(root, ref string) error {
	return os.WriteFile(headPath(root), []byte("ref: "+ref+"\n"), 0o644)
}

func readHead(root string) (string, error) {
	b, err := os.ReadFile(headPath(root))
	if err != nil {
		return "", err
	}
	s := string(b)
	const prefix = "ref: "
	if len(s) <= len(prefix) || s[:len(prefix)] != prefix {
		return "", fmt.Errorf("malformed HEAD: %q", s)
	}
	// trim prefix and trailing newline
	out := s[len(prefix):]
	for len(out) > 0 && (out[len(out)-1] == '\n' || out[len(out)-1] == '\r') {
		out = out[:len(out)-1]
	}
	return out, nil
}
