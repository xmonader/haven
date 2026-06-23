package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haven/internal/protocol"
)

// withIdentity points HAVEN_IDENTITY at a fresh per-test key file.
func withIdentity(t *testing.T) {
	t.Helper()
	t.Setenv("HAVEN_IDENTITY", filepath.Join(t.TempDir(), "identity"))
}

func TestSecretEncryptedAtRestAndDecryptsOnCheckout(t *testing.T) {
	withIdentity(t)
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")
	if out, code := run(t, dir, "key", "gen"); code != 0 {
		t.Fatalf("key gen failed:\n%s", out)
	}

	secretLine := "DB_PASSWORD=hunter2\n"
	os.WriteFile(filepath.Join(dir, ".env"), []byte(secretLine), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)
	run(t, dir, "add", ".")
	if out, code := run(t, dir, "commit", "-m", "app"); code != 0 {
		t.Fatalf("commit failed:\n%s", out)
	}

	// The raw repository database must not contain the plaintext secret.
	dbBytes, err := os.ReadFile(filepath.Join(dir, ".haven", "haven.db"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(dbBytes, []byte("hunter2")) {
		t.Fatal("plaintext secret found in repository database")
	}

	// A recipient round-trips the secret cleanly through a branch switch.
	run(t, dir, "branch", "create", "x")
	run(t, dir, "branch", "switch", "x")
	run(t, dir, "branch", "switch", "main")
	got, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if string(got) != secretLine {
		t.Errorf(".env after checkout = %q, want %q", got, secretLine)
	}

	// Working tree is clean (secret identity hash is stable).
	if out, _ := run(t, dir, "status"); !strings.Contains(out, "nothing to commit") {
		t.Errorf("expected clean tree, got:\n%s", out)
	}
}

func TestSecretStaysCiphertextThroughPush(t *testing.T) {
	withIdentity(t)
	url := startServer(t, protocol.KindTeam)

	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")
	run(t, dir, "key", "gen")
	os.WriteFile(filepath.Join(dir, ".env"), []byte("API_KEY=sk-topsecret\n"), 0o644)
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "secret")
	run(t, dir, "remote", "add", "origin", url, "--kind", "team")

	if out, code := run(t, dir, "push", "origin", "main"); code != 0 {
		t.Fatalf("push failed:\n%s", out)
	}
	// The server only ever received ciphertext; GET /objects returns no
	// plaintext. (Verified indirectly: a fresh clone with NO identity yields a
	// locked marker rather than the secret.)
	withIdentity(t) // new, empty identity: not a recipient
	parent := t.TempDir()
	run(t, parent, "clone", url, "c2")
	got, _ := os.ReadFile(filepath.Join(parent, "c2", ".env"))
	if strings.Contains(string(got), "sk-topsecret") {
		t.Fatalf("non-recipient clone exposed the secret: %q", got)
	}
	if !strings.Contains(string(got), "not a recipient") {
		t.Errorf("expected locked marker for non-recipient, got %q", got)
	}
}
