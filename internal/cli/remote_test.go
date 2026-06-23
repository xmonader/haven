package cli

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haven/internal/protocol"
	"haven/internal/repo"
)

// startServer creates a fresh server-side repository and serves it over HTTP.
func startServer(t *testing.T, kind string) string {
	t.Helper()
	srvDir := t.TempDir()
	sr, err := repo.Init(srvDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sr.Close() })
	ts := httptest.NewServer(protocol.NewServer(sr.DB, kind).Handler())
	t.Cleanup(ts.Close)
	return ts.URL
}

func TestPushAndCloneRoundtrip(t *testing.T) {
	url := startServer(t, protocol.KindTeam)

	// Author commits and pushes.
	work := t.TempDir()
	run(t, work, "init")
	run(t, work, "config", "user.name", "Dev")
	os.WriteFile(filepath.Join(work, "a.txt"), []byte("hello v1\n"), 0o644)
	run(t, work, "add", ".")
	run(t, work, "commit", "-m", "first")
	run(t, work, "remote", "add", "origin", url, "--kind", "team")

	if out, code := run(t, work, "push", "origin", "main"); code != 0 {
		t.Fatalf("push failed:\n%s", out)
	}

	// A fresh clone reproduces the working tree and history.
	cloneParent := t.TempDir()
	if out, code := run(t, cloneParent, "clone", url, "c2"); code != 0 {
		t.Fatalf("clone failed:\n%s", out)
	}
	got, err := os.ReadFile(filepath.Join(cloneParent, "c2", "a.txt"))
	if err != nil || string(got) != "hello v1\n" {
		t.Fatalf("cloned content = %q, err=%v", got, err)
	}
	if out, _ := run(t, filepath.Join(cloneParent, "c2"), "log"); !strings.Contains(out, "first") {
		t.Errorf("cloned log missing commit:\n%s", out)
	}
}

func TestPushRefusesPrivateHavenToTeam(t *testing.T) {
	url := startServer(t, protocol.KindTeam)
	work := t.TempDir()
	run(t, work, "init")
	run(t, work, "config", "user.name", "Dev")
	os.WriteFile(filepath.Join(work, "a.txt"), []byte("x\n"), 0o644)
	run(t, work, "add", ".")
	run(t, work, "commit", "-m", "base")
	run(t, work, "remote", "add", "origin", url, "--kind", "team")

	run(t, work, "haven", "create", "secret")
	run(t, work, "haven", "switch", "secret")
	os.WriteFile(filepath.Join(work, "s.txt"), []byte("top secret\n"), 0o644)
	run(t, work, "add", ".")
	run(t, work, "commit", "-m", "wip")

	out, code := run(t, work, "push", "origin", "secret")
	if code == 0 {
		t.Fatalf("pushing a haven must be refused; got:\n%s", out)
	}
	if !strings.Contains(out, "refusing") {
		t.Errorf("expected refusal message, got:\n%s", out)
	}
}
