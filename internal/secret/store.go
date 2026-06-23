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

// Member is an actor and their public recipient.
type Member struct {
	Name      string
	Recipient string
}

// AddMember records (or updates) an actor's recipient key.
func AddMember(db *sql.DB, name, recipient string) error {
	_, err := db.Exec(
		`INSERT INTO members(name, recipient) VALUES(?,?)
		 ON CONFLICT(name) DO UPDATE SET recipient=excluded.recipient`, name, recipient)
	return err
}

// Members lists all members.
func Members(db *sql.DB) ([]Member, error) {
	rows, err := db.Query(`SELECT name, recipient FROM members ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.Name, &m.Recipient); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Recipients returns just the recipient strings of all members.
func Recipients(db *sql.DB) ([]string, error) {
	members, err := Members(db)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(members))
	for _, m := range members {
		out = append(out, m.Recipient)
	}
	return out, nil
}
