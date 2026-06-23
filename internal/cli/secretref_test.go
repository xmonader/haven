package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSecretBranchEncryptsWholeTree verifies that marking a ref secret causes
// every file (not just mark-matched ones) to be encrypted at rest, while a
// recipient still round-trips them through checkout.
func TestSecretBranchEncryptsWholeTree(t *testing.T) {
	withIdentity(t)
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")
	if out, code := run(t, dir, "key", "gen"); code != 0 {
		t.Fatalf("key gen failed:\n%s", out)
	}

	// Mark the default branch secret, then commit an ordinary (unmarked) file.
	if out, code := run(t, dir, "secret", "ref", "main"); code != 0 {
		t.Fatalf("secret ref failed:\n%s", out)
	}
	plain := "package main // not normally a secret\n"
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(plain), 0o644)
	run(t, dir, "add", ".")
	if out, code := run(t, dir, "commit", "-m", "app"); code != 0 {
		t.Fatalf("commit failed:\n%s", out)
	}

	// The unmarked file's plaintext must NOT be at rest in the DB.
	dbBytes, _ := os.ReadFile(filepath.Join(dir, ".haven", "haven.db"))
	if bytes.Contains(dbBytes, []byte("not normally a secret")) {
		t.Fatal("plaintext of an ordinary file leaked: secret-ref encryption did not apply")
	}

	// Recipient round-trips it cleanly through a checkout cycle.
	run(t, dir, "haven", "create", "tmp")
	run(t, dir, "haven", "switch", "tmp")
	run(t, dir, "branch", "switch", "main")
	got, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if string(got) != plain {
		t.Errorf("main.go after checkout = %q, want %q", got, plain)
	}
	if out, _ := run(t, dir, "status"); !strings.Contains(out, "nothing to commit") {
		t.Errorf("expected clean tree, got:\n%s", out)
	}
}

// TestSecretRotateRewritesCiphertext verifies that rotation re-encrypts secret
// objects without changing the commit, and arms drift detection.
func TestSecretRotateRewritesCiphertext(t *testing.T) {
	withIdentity(t)
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")
	run(t, dir, "key", "gen")

	os.WriteFile(filepath.Join(dir, ".env"), []byte("TOKEN=abc\n"), 0o644)
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "secret")
	headBefore, _ := run(t, dir, "log")

	if out, code := run(t, dir, "secret", "rotate"); code != 0 {
		t.Fatalf("rotate failed:\n%s", out)
	} else if !strings.Contains(out, "rotated 1 secret object") {
		t.Errorf("unexpected rotate output:\n%s", out)
	}

	// Rotation must not create a new commit (plaintext-addressed hash is stable).
	headAfter, _ := run(t, dir, "log")
	if headBefore != headAfter {
		t.Errorf("rotation changed history:\nbefore %s\nafter %s", headBefore, headAfter)
	}

	// Secret still decrypts to the same plaintext after rotation.
	run(t, dir, "branch", "create", "x")
	run(t, dir, "branch", "switch", "x")
	run(t, dir, "branch", "switch", "main")
	got, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if string(got) != "TOKEN=abc\n" {
		t.Errorf(".env after rotate+checkout = %q", got)
	}

	// With the baseline armed and recipients unchanged, there is no drift.
	if out, _ := run(t, dir, "secret", "status"); !strings.Contains(out, "no secret drift") {
		t.Errorf("expected no drift, got:\n%s", out)
	}
}

// TestSecretDriftDetectedAfterMembershipChange verifies the baseline is armed at
// the first commit of secrets and that adding a reader is reported as drift.
func TestSecretDriftDetectedAfterMembershipChange(t *testing.T) {
	withIdentity(t)
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")
	run(t, dir, "key", "gen")

	os.WriteFile(filepath.Join(dir, ".env"), []byte("TOKEN=abc\n"), 0o644)
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "secret")

	// Baseline armed at commit: no drift yet (no rotate needed first).
	if out, _ := run(t, dir, "secret", "status"); !strings.Contains(out, "no secret drift") {
		t.Errorf("expected no drift right after commit, got:\n%s", out)
	}

	// Adding a reader changes the recipient set on the public branch.
	run(t, dir, "member", "add", "bob",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqsxq8gj")
	out, _ := run(t, dir, "secret", "status")
	if !strings.Contains(out, "DRIFT") {
		t.Errorf("expected drift after adding a reader, got:\n%s", out)
	}
}
