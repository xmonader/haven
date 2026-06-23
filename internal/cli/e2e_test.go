package cli

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haven/internal/protocol"
	"haven/internal/store"
)

// End-to-end: a policy-bearing repo served over HTTP enforces access for an
// anonymous clone — public branch + ciphertext visible, restricted branch
// hidden, secret undecryptable.
func TestEndToEndACLOverHTTP(t *testing.T) {
	adminID := filepath.Join(t.TempDir(), "admin-id")
	t.Setenv("HAVEN_IDENTITY", adminID)

	A := t.TempDir()
	run(t, A, "init")
	run(t, A, "config", "user.name", "Admin")
	if out, code := run(t, A, "key", "gen"); code != 0 {
		t.Fatalf("key gen:\n%s", out)
	}
	os.WriteFile(filepath.Join(A, "README"), []byte("public docs\n"), 0o644)
	os.WriteFile(filepath.Join(A, ".env"), []byte("TOKEN=secret123\n"), 0o644)
	run(t, A, "add", ".")
	run(t, A, "commit", "-m", "init")

	// A restricted staging branch only admins can read.
	run(t, A, "branch", "create", "staging")
	run(t, A, "branch", "switch", "staging")
	os.WriteFile(filepath.Join(A, "deploy.sh"), []byte("rm -rf prod\n"), 0o644)
	run(t, A, "add", ".")
	run(t, A, "commit", "-m", "staging work")
	run(t, A, "branch", "switch", "main")
	run(t, A, "group", "create", "admins")
	run(t, A, "group", "add", "admins", "Admin")
	if out, code := run(t, A, "restrict", "staging", "--read", "admins"); code != 0 {
		t.Fatalf("restrict:\n%s", out)
	}

	// Serve A over HTTP from a fresh read connection.
	db, err := store.Open(filepath.Join(A, ".haven", "haven.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	srv := httptest.NewServer(protocol.NewServer(db, protocol.KindTeam).Handler())
	defer srv.Close()

	// Anonymous clone: no identity present.
	t.Setenv("HAVEN_IDENTITY", filepath.Join(t.TempDir(), "nobody"))
	work := t.TempDir()
	if out, code := run(t, work, "clone", srv.URL, "clone"); code != 0 {
		t.Fatalf("anonymous clone failed:\n%s", out)
	}
	B := filepath.Join(work, "clone")

	// Public README is present.
	if got, err := os.ReadFile(filepath.Join(B, "README")); err != nil || string(got) != "public docs\n" {
		t.Errorf("public README not cloned: %q %v", got, err)
	}
	// Restricted staging branch must not be visible.
	if out, _ := run(t, B, "branch", "list"); strings.Contains(out, "staging") {
		t.Errorf("restricted branch leaked to anonymous clone:\n%s", out)
	}
	// The secret .env is present but as a locked notice (anon is not a recipient).
	if got, _ := os.ReadFile(filepath.Join(B, ".env")); strings.Contains(string(got), "secret123") {
		t.Error("anonymous clone decrypted a secret it should not have")
	} else if !strings.Contains(string(got), "haven") {
		t.Errorf(".env should be a locked notice, got %q", got)
	}
	// The policy chain came along and verifies offline.
	if out, code := run(t, B, "policy", "verify"); code != 0 || !strings.Contains(out, "valid") {
		t.Errorf("policy should verify in the clone:\n%s", out)
	}
}

// End-to-end: a full local feature chain (secret branch, history ops) stays
// consistent — clean tree throughout.
func TestEndToEndLocalFeatureChain(t *testing.T) {
	t.Setenv("HAVEN_IDENTITY", filepath.Join(t.TempDir(), "id"))
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")
	run(t, dir, "key", "gen")

	commitFile(t, dir, "app.go", "package main\n", "base")

	// Stash some WIP, do other work, pop it back.
	os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main // wip\n"), 0o644)
	run(t, dir, "stash")
	commitFile(t, dir, "README", "docs\n", "docs")
	if out, code := run(t, dir, "stash", "pop"); code != 0 {
		t.Fatalf("stash pop:\n%s", out)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "app.go")); !strings.Contains(string(got), "wip") {
		t.Errorf("stash pop lost WIP: %q", got)
	}

	// fsck and gc must stay happy on a repo with a policy + secrets.
	os.WriteFile(filepath.Join(dir, ".env"), []byte("K=v\n"), 0o644)
	run(t, dir, "add", ".")
	commitFile(t, dir, ".env", "K=v\n", "secret")
	if out, code := run(t, dir, "fsck"); code != 0 {
		t.Fatalf("fsck failed:\n%s", out)
	}
	if out, code := run(t, dir, "gc"); code != 0 {
		t.Fatalf("gc failed:\n%s", out)
	}
	// Secret survives gc and still decrypts.
	run(t, dir, "branch", "create", "x")
	run(t, dir, "branch", "switch", "x")
	run(t, dir, "branch", "switch", "main")
	if got, _ := os.ReadFile(filepath.Join(dir, ".env")); string(got) != "K=v\n" {
		t.Errorf(".env after gc+checkout = %q", got)
	}
}
