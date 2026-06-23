// Package ref manages named pointers (branches, havens, policy) stored in the
// refs table, each carrying a visibility.
package ref

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Visibility values.
const (
	Public     = "public"
	Restricted = "restricted"
	Private    = "private"
	Policy     = "policy"
)

// Ref name prefixes.
const (
	BranchPrefix = "refs/branches/"
	HavenPrefix  = "refs/havens/"
	TagPrefix    = "refs/tags/"
)

// Ref is a named pointer to a commit.
type Ref struct {
	Name       string
	Visibility string
	Target     string // commit hash, or "" if unborn
	Mtime      int64
}

// VisibilityFor infers a default visibility from a ref name.
func VisibilityFor(name string) string {
	switch {
	case strings.HasPrefix(name, HavenPrefix):
		return Private
	case name == "refs/policy":
		return Policy
	default:
		return Public
	}
}

// Get returns a ref, or sql.ErrNoRows if it does not exist.
func Get(db *sql.DB, name string) (Ref, error) {
	var r Ref
	err := db.QueryRow(
		`SELECT name, visibility, target, mtime FROM refs WHERE name=?`, name,
	).Scan(&r.Name, &r.Visibility, &r.Target, &r.Mtime)
	return r, err
}

// Resolve returns the target commit of name, or "" if the ref is absent or
// unborn (no commits yet).
func Resolve(db *sql.DB, name string) (string, error) {
	r, err := Get(db, name)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return r.Target, nil
}

// Set upserts a ref to point at target. If the ref already exists its
// visibility is preserved; otherwise visibility is inferred from the name.
func Set(db *sql.DB, name, target string) error {
	vis := VisibilityFor(name)
	if existing, err := Get(db, name); err == nil {
		vis = existing.Visibility
	} else if err != sql.ErrNoRows {
		return err
	}
	return SetVisible(db, name, target, vis)
}

// SetVisible upserts a ref with an explicit visibility.
func SetVisible(db *sql.DB, name, target, visibility string) error {
	_, err := db.Exec(
		`INSERT INTO refs(name, visibility, target, mtime) VALUES(?,?,?,?)
		 ON CONFLICT(name) DO UPDATE SET target=excluded.target, visibility=excluded.visibility, mtime=excluded.mtime`,
		name, visibility, target, time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("set ref %s: %w", name, err)
	}
	return nil
}

// CompareAndSwap atomically repoints name to newTarget only if its current
// target equals oldTarget. An oldTarget of "" means the ref must be absent or
// unborn (empty target). Returns false (without error) when the precondition no
// longer holds — i.e. a concurrent writer won the race. The whole check-and-set
// is one SQLite statement, so it is safe against concurrent ref updates.
func CompareAndSwap(db *sql.DB, name, oldTarget, newTarget, visibility string) (bool, error) {
	now := time.Now().Unix()
	if oldTarget == "" {
		// Create when absent, or advance a row that is still unborn (target "").
		res, err := db.Exec(
			`INSERT INTO refs(name, visibility, target, mtime) VALUES(?,?,?,?)
			 ON CONFLICT(name) DO UPDATE SET target=excluded.target, visibility=excluded.visibility, mtime=excluded.mtime
			 WHERE refs.target=''`,
			name, visibility, newTarget, now,
		)
		if err != nil {
			return false, fmt.Errorf("cas ref %s: %w", name, err)
		}
		n, _ := res.RowsAffected()
		return n == 1, nil
	}
	res, err := db.Exec(
		`UPDATE refs SET target=?, visibility=?, mtime=? WHERE name=? AND target=?`,
		newTarget, visibility, now, name, oldTarget,
	)
	if err != nil {
		return false, fmt.Errorf("cas ref %s: %w", name, err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// List returns all refs ordered by name.
func List(db *sql.DB) ([]Ref, error) {
	rows, err := db.Query(`SELECT name, visibility, target, mtime FROM refs ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ref
	for rows.Next() {
		var r Ref
		if err := rows.Scan(&r.Name, &r.Visibility, &r.Target, &r.Mtime); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Delete removes a ref.
func Delete(db *sql.DB, name string) error {
	_, err := db.Exec(`DELETE FROM refs WHERE name=?`, name)
	return err
}

// ShortName strips the known prefixes for display (refs/branches/main -> main).
func ShortName(name string) string {
	for _, p := range []string{BranchPrefix, HavenPrefix} {
		if strings.HasPrefix(name, p) {
			return name[len(p):]
		}
	}
	return name
}
