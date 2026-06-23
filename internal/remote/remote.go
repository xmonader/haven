// Package remote manages configured remotes (team and personal hosts) in the
// remotes table.
package remote

import (
	"database/sql"
	"fmt"
)

// Kinds mirror protocol server kinds.
const (
	Team     = "team"
	Personal = "personal"
)

// Remote is a configured remote host.
type Remote struct {
	Name string
	URL  string
	Kind string
}

// Add stores a new remote. Kind must be team or personal.
func Add(db *sql.DB, name, url, kind string) error {
	if kind != Team && kind != Personal {
		return fmt.Errorf("kind must be team or personal, got %q", kind)
	}
	_, err := db.Exec(
		`INSERT INTO remotes(name, url, kind) VALUES(?,?,?)
		 ON CONFLICT(name) DO UPDATE SET url=excluded.url, kind=excluded.kind`,
		name, url, kind)
	return err
}

// Get returns a remote by name.
func Get(db *sql.DB, name string) (Remote, error) {
	var r Remote
	err := db.QueryRow(`SELECT name, url, kind FROM remotes WHERE name=?`, name).
		Scan(&r.Name, &r.URL, &r.Kind)
	if err == sql.ErrNoRows {
		return r, fmt.Errorf("no such remote %q", name)
	}
	return r, err
}

// List returns all remotes ordered by name.
func List(db *sql.DB) ([]Remote, error) {
	rows, err := db.Query(`SELECT name, url, kind FROM remotes ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Remote
	for rows.Next() {
		var r Remote
		if err := rows.Scan(&r.Name, &r.URL, &r.Kind); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Remove deletes a remote.
func Remove(db *sql.DB, name string) error {
	_, err := db.Exec(`DELETE FROM remotes WHERE name=?`, name)
	return err
}
