// Package config reads and writes repository configuration in the config
// table (key/value), plus convenience accessors like the commit author.
package config

import (
	"database/sql"
	"os"
)

// Get returns a config value and whether it was present.
func Get(db *sql.DB, key string) (string, bool, error) {
	var v string
	err := db.QueryRow(`SELECT value FROM config WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// Set stores a config value (upsert).
func Set(db *sql.DB, key, value string) error {
	_, err := db.Exec(
		`INSERT INTO config(key, value) VALUES(?,?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// Author resolves the commit author name and email, falling back to the
// HAVEN_AUTHOR / USER environment and finally a generic default.
func Author(db *sql.DB) (name, email string) {
	name = first(getOr(db, "user.name"), os.Getenv("HAVEN_AUTHOR"), os.Getenv("USER"), "haven")
	email = first(getOr(db, "user.email"), os.Getenv("HAVEN_EMAIL"), "haven@localhost")
	return name, email
}

func getOr(db *sql.DB, key string) string {
	v, ok, err := Get(db, key)
	if err != nil || !ok {
		return ""
	}
	return v
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
