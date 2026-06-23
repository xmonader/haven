package cli

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"haven/internal/object"
	"haven/internal/protocol"
	"haven/internal/repo"
	"haven/internal/store"
)

// A rotated secret must propagate to the server: the server upserts secret
// ciphertext on receive instead of ignoring an already-known hash.
func TestRotatePropagatesOverPush(t *testing.T) {
	withIdentity(t)

	srvDir := t.TempDir()
	sr, err := repo.Init(srvDir)
	if err != nil {
		t.Fatal(err)
	}
	defer sr.Close()
	ts := httptest.NewServer(protocol.NewServer(sr.DB, protocol.KindTeam).Handler())
	defer ts.Close()

	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")
	run(t, dir, "key", "gen")
	os.WriteFile(filepath.Join(dir, ".env"), []byte("API_KEY=abc\n"), 0o644)
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "secret")
	run(t, dir, "remote", "add", "origin", ts.URL, "--kind", "team")
	if out, code := run(t, dir, "push", "origin", "main"); code != 0 {
		t.Fatalf("push failed:\n%s", out)
	}

	before := serverSecret(t, srvDir)

	// Rotate locally, then push again.
	if out, code := run(t, dir, "secret", "rotate"); code != 0 {
		t.Fatalf("rotate:\n%s", out)
	}
	if out, code := run(t, dir, "push", "origin", "main"); code != 0 {
		t.Fatalf("second push failed:\n%s", out)
	}
	after := serverSecret(t, srvDir)

	if before == "" || after == "" {
		t.Fatal("no secret object on server")
	}
	if before == after {
		t.Fatal("rotated ciphertext did not propagate to the server")
	}
}

func serverSecret(t *testing.T, srvDir string) string {
	t.Helper()
	db, err := store.Open(filepath.Join(srvDir, ".haven", "haven.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var found string
	object.NewStore(db).Each(func(h string, typ object.Type, content []byte) error {
		if typ == object.Secret {
			found = string(content)
		}
		return nil
	})
	return found
}
