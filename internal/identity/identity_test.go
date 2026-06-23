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

func TestLoadRejectsMissing(t *testing.T) {
	t.Setenv("HAVEN_IDENTITY", filepath.Join(t.TempDir(), "absent"))
	_, err := Load()
	if err == nil {
		t.Fatal("Load must error when no identity exists")
	}
}

func TestLoadRejectsCorruptAndBadKeys(t *testing.T) {
	cases := map[string]string{
		"not json":         "}{ not json",
		"bad enc key":      `{"enc":"not-an-age-key","sign":"00"}`,
		"bad sign hex":     `{"enc":"AGE-SECRET-KEY-1","sign":"zz"}`,
		"short sign key":   `{"enc":"AGE-SECRET-KEY-1","sign":"00"}`,
		"empty everything": `{}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "id")
			t.Setenv("HAVEN_IDENTITY", p)
			if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(); err == nil {
				t.Fatalf("Load must reject %s", name)
			}
		})
	}
}

func TestLoadOrNil(t *testing.T) {
	// Absent → nil, no panic.
	t.Setenv("HAVEN_IDENTITY", filepath.Join(t.TempDir(), "absent"))
	if LoadOrNil() != nil {
		t.Fatal("LoadOrNil should be nil when no identity exists")
	}
	// Present → the identity.
	t.Setenv("HAVEN_IDENTITY", filepath.Join(t.TempDir(), "id"))
	gen, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	got := LoadOrNil()
	if got == nil || got.SignPub() != gen.SignPub() {
		t.Fatal("LoadOrNil should return the generated identity")
	}
	// Corrupt → nil (never panics).
	t.Setenv("HAVEN_IDENTITY", filepath.Join(t.TempDir(), "bad"))
	os.WriteFile(Path(), []byte("garbage"), 0o600)
	if LoadOrNil() != nil {
		t.Fatal("LoadOrNil should be nil on a corrupt identity")
	}
}

func TestPathHonorsEnv(t *testing.T) {
	t.Setenv("HAVEN_IDENTITY", "/custom/identity")
	if Path() != "/custom/identity" {
		t.Fatalf("HAVEN_IDENTITY ignored: %s", Path())
	}
	t.Setenv("HAVEN_IDENTITY", "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got, want := Path(), "/xdg/haven/identity"; got != want {
		t.Fatalf("XDG path = %s, want %s", got, want)
	}
}
