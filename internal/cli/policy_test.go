package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haven/internal/identity"
)

// mintKeys generates a throwaway identity at its own path and returns its
// public sign key and age recipient (without disturbing the caller's identity).
func mintKeys(t *testing.T) (signPub, recipient string) {
	t.Helper()
	saved := os.Getenv("HAVEN_IDENTITY")
	defer os.Setenv("HAVEN_IDENTITY", saved)
	os.Setenv("HAVEN_IDENTITY", filepath.Join(t.TempDir(), "other"))
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	return id.SignPub(), id.Recipient()
}

func TestPolicyMembersGroupsRestrict(t *testing.T) {
	withIdentity(t) // alice
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "alice")
	if out, code := run(t, dir, "key", "gen"); code != 0 {
		t.Fatalf("key gen:\n%s", out)
	}

	bobSign, bobEnc := mintKeys(t)
	if out, code := run(t, dir, "member", "add", "bob", bobSign, bobEnc); code != 0 {
		t.Fatalf("member add:\n%s", out)
	}
	run(t, dir, "group", "create", "deployers")
	run(t, dir, "group", "add", "deployers", "alice", "bob")
	run(t, dir, "branch", "create", "staging")
	if out, code := run(t, dir, "restrict", "staging", "--read", "deployers"); code != 0 {
		t.Fatalf("restrict:\n%s", out)
	}

	// The whole chain of signed versions must verify.
	if out, code := run(t, dir, "policy", "verify"); code != 0 {
		t.Fatalf("policy verify failed:\n%s", out)
	}

	// main stays public; staging is need-to-know (no "everyone").
	mainOut, _ := run(t, dir, "policy", "access", "main")
	if !strings.Contains(mainOut, "everyone") {
		t.Errorf("main should be public:\n%s", mainOut)
	}
	stagingOut, _ := run(t, dir, "policy", "access", "staging")
	if strings.Contains(stagingOut, "everyone") {
		t.Errorf("staging must not be public:\n%s", stagingOut)
	}
	if !strings.Contains(stagingOut, "alice") || !strings.Contains(stagingOut, "bob") {
		t.Errorf("staging should be readable by deployers:\n%s", stagingOut)
	}
}
