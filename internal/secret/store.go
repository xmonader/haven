package secret

import "database/sql"

// DefaultMarks are the secret path globs seeded into a new repository so that
// common credential files are encrypted without any user action.
var DefaultMarks = []string{
	".env", ".env.*", "*.pem", "*.key", "id_rsa", "id_ed25519",
	"secrets/**", "**/credentials.json",
}

// SeedDefaultMarks inserts the default marks (idempotent).
func SeedDefaultMarks(db *sql.DB) error {
	for _, g := range DefaultMarks {
		if err := AddMark(db, g); err != nil {
			return err
		}
	}
	return nil
}

// AddMark records a secret path glob.
func AddMark(db *sql.DB, glob string) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO secret_paths(glob) VALUES(?)`, glob)
	return err
}

// Marks returns all secret path globs.
func Marks(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT glob FROM secret_paths ORDER BY glob`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
