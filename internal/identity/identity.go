// Package identity manages a user's encryption keypair. The private key lives
// outside any repository (in ~/.config/haven/identity by default) and never
// touches the object store or a server.
package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
)

// Identity is a user's age (X25519) encryption keypair.
type Identity struct {
	X25519 *age.X25519Identity
}

// Path returns the on-disk location of the private key. It honors the
// HAVEN_IDENTITY environment variable, then falls back to
// $XDG_CONFIG_HOME/haven/identity or ~/.config/haven/identity.
func Path() string {
	if p := os.Getenv("HAVEN_IDENTITY"); p != "" {
		return p
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "haven", "identity")
}

// Exists reports whether an identity file is present.
func Exists() bool {
	_, err := os.Stat(Path())
	return err == nil
}

// Generate creates a new identity and writes it to Path with 0600 permissions.
// It refuses to overwrite an existing identity.
func Generate() (*Identity, error) {
	if Exists() {
		return nil, fmt.Errorf("identity already exists at %s", Path())
	}
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, err
	}
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, []byte(id.String()+"\n"), 0o600); err != nil {
		return nil, err
	}
	return &Identity{X25519: id}, nil
}

// Load reads the identity from Path. It errors if none exists.
func Load() (*Identity, error) {
	p := Path()
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no identity at %s (run 'hv key gen')", p)
		}
		return nil, err
	}
	id, err := age.ParseX25519Identity(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, fmt.Errorf("parse identity: %w", err)
	}
	return &Identity{X25519: id}, nil
}

// LoadOrNil returns the identity, or nil if none exists (without erroring).
// Used on checkout, where a non-recipient simply gets locked markers.
func LoadOrNil() *Identity {
	if !Exists() {
		return nil
	}
	id, err := Load()
	if err != nil {
		return nil
	}
	return id
}

// Recipient returns the public age recipient string ("age1...").
func (i *Identity) Recipient() string {
	return i.X25519.Recipient().String()
}
