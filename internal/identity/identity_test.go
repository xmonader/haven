package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateLoadRoundTrip(t *testing.T) {
	t.Setenv("HAVEN_IDENTITY", filepath.Join(t.TempDir(), "id"))

	if Exists() {
		t.Fatal("identity should not exist yet")
	}
	gen, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(); err == nil {
		t.Fatal("Generate must refuse to overwrite an existing identity")
	}
	if !Exists() {
		t.Fatal("Exists should be true after Generate")
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Recipient() != gen.Recipient() {
		t.Errorf("recipient mismatch: %s vs %s", loaded.Recipient(), gen.Recipient())
	}
	if loaded.SignPub() != gen.SignPub() {
		t.Errorf("sign pub mismatch")
	}
}

// TestKeyFilePrivate verifies the identity file is written 0600 and that Load
// tightens a too-open key file back to 0600 (the private key must never be
// group/world-readable).
func TestKeyFilePrivate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "id")
	t.Setenv("HAVEN_IDENTITY", p)
	if _, err := Generate(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("generated key mode = %o, want 0600", perm)
	}
	// Loosen it, then Load must tighten it back.
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Stat(p)
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("Load left key group/world-accessible: %o", perm)
	}
}
