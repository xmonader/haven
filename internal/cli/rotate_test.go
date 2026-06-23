package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haven/internal/object"
	"haven/internal/store"
)

func secretCiphertext(t *testing.T, dir string) string {
	t.Helper()
	db, err := store.Open(filepath.Join(dir, ".haven", "haven.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := object.NewStore(db)
	var found string
	s.Each(func(h string, typ object.Type, content []byte) error {
		if typ == object.Secret {
			found = string(content)
		}
		return nil
	})
	return found
}

func keyShow(t *testing.T, dir string) (sign, enc string) {
	t.Helper()
	out, _ := run(t, dir, "key", "show")
	for _, line := range strings.Split(out, "\n") {
		if s, ok := strings.CutPrefix(line, "sign: "); ok {
			sign = strings.TrimSpace(s)
		}
		if e, ok := strings.CutPrefix(line, "enc:  "); ok {
			enc = strings.TrimSpace(e)
		}
	}
	if sign == "" || enc == "" {
		t.Fatalf("could not parse key show:\n%s", out)
	}
	return sign, enc
}

// Rotation must actually rewrite the stored ciphertext (age is non-deterministic,
// so a real re-encrypt always differs from the original bytes).
func TestRotateRewritesStoredCiphertext(t *testing.T) {
	t.Setenv("HAVEN_IDENTITY", filepath.Join(t.TempDir(), "id"))
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")
	run(t, dir, "key", "gen")
	os.WriteFile(filepath.Join(dir, ".env"), []byte("TOKEN=abc\n"), 0o644)
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "secret")

	before := secretCiphertext(t, dir)
	run(t, dir, "secret", "rotate")
	after := secretCiphertext(t, dir)
	if before == "" || after == "" {
		t.Fatal("no secret object found")
	}
	if before == after {
		t.Fatal("rotate did not rewrite the ciphertext (no-op)")
	}
}

// The end-to-end point of rotation: a reader added AFTER a secret was committed
// can decrypt it only once the secret is rotated to the new recipient set.
func TestRotateGrantsNewReaderAccess(t *testing.T) {
	adminID := filepath.Join(t.TempDir(), "admin")
	bobID := filepath.Join(t.TempDir(), "bob")

	dir := t.TempDir()
	t.Setenv("HAVEN_IDENTITY", adminID)
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Admin")
	run(t, dir, "key", "gen")
	os.WriteFile(filepath.Join(dir, ".env"), []byte("TOKEN=abc\n"), 0o644)
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "secret")

	// Create Bob's identity in a separate key file.
	t.Setenv("HAVEN_IDENTITY", bobID)
	run(t, dir, "key", "gen")
	bobSign, bobEnc := keyShow(t, dir)

	// Admin adds Bob to the keyring, then rotates so the secret includes him.
	t.Setenv("HAVEN_IDENTITY", adminID)
	run(t, dir, "member", "add", "bob", bobSign, bobEnc)
	if out, code := run(t, dir, "secret", "rotate"); code != 0 {
		t.Fatalf("rotate failed:\n%s", out)
	}

	// As Bob, re-materialize the working tree; he must now read the plaintext.
	t.Setenv("HAVEN_IDENTITY", bobID)
	run(t, dir, "branch", "create", "x")
	run(t, dir, "branch", "switch", "x")
	run(t, dir, "branch", "switch", "main")
	got, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if string(got) != "TOKEN=abc\n" {
		t.Fatalf("Bob could not read the rotated secret; got %q", got)
	}
}
