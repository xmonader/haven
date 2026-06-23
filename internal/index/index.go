// Package index is the staging area: the set of paths and blob hashes that the
// next commit will capture. Backed by the staging table.
package index

import "database/sql"

// Add stages a path at a blob hash (upsert).
func Add(db *sql.DB, path, hash string) error {
	_, err := db.Exec(
		`INSERT INTO staging(path, hash) VALUES(?,?)
		 ON CONFLICT(path) DO UPDATE SET hash=excluded.hash`, path, hash)
	return err
}

// Remove unstages a path.
func Remove(db *sql.DB, path string) error {
	_, err := db.Exec(`DELETE FROM staging WHERE path=?`, path)
	return err
}

// All returns the staged set as path -> blob hash.
func All(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query(`SELECT path, hash FROM staging`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var p, h string
		if err := rows.Scan(&p, &h); err != nil {
			return nil, err
		}
		out[p] = h
	}
	return out, rows.Err()
}

// Clear empties the staging area.
func Clear(db *sql.DB) error {
	_, err := db.Exec(`DELETE FROM staging`)
	return err
}
