package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haven/internal/protocol"
)

func TestSyncCarriesHavenBetweenMachines(t *testing.T) {
	url := startServer(t, protocol.KindPersonal)

	// Laptop A: a public branch and a private haven.
	a := t.TempDir()
	run(t, a, "init")
	run(t, a, "config", "user.name", "A")
	os.WriteFile(filepath.Join(a, "pub.txt"), []byte("public\n"), 0o644)
	run(t, a, "add", ".")
	run(t, a, "commit", "-m", "public")
	run(t, a, "haven", "create", "experiment")
	run(t, a, "haven", "switch", "experiment")
	os.WriteFile(filepath.Join(a, "idea.txt"), []byte("secret idea\n"), 0o644)
	run(t, a, "add", ".")
	run(t, a, "commit", "-m", "wip")
	run(t, a, "branch", "switch", "main")
	run(t, a, "remote", "add", "personal", url, "--kind", "personal")

	if out, code := run(t, a, "sync", "personal"); code != 0 {
		t.Fatalf("sync failed:\n%s", out)
	}

	// Laptop B: clone from the personal remote and receive the haven.
	parent := t.TempDir()
	if out, code := run(t, parent, "clone", url, "repoB", "--kind", "personal"); code != 0 {
		t.Fatalf("clone failed:\n%s", out)
	}
	b := filepath.Join(parent, "repoB")
	out, _ := run(t, b, "haven", "list")
	if !strings.Contains(out, "experiment") {
		t.Fatalf("haven did not sync to laptop B:\n%s", out)
	}
	if _, code := run(t, b, "haven", "switch", "experiment"); code != 0 {
		t.Fatal("switch to synced haven failed")
	}
	got, _ := os.ReadFile(filepath.Join(b, "idea.txt"))
	if string(got) != "secret idea\n" {
		t.Errorf("synced haven content = %q", got)
	}
}

func TestSyncRefusesTeamRemote(t *testing.T) {
	url := startServer(t, protocol.KindTeam)
	a := t.TempDir()
	run(t, a, "init")
	run(t, a, "remote", "add", "origin", url, "--kind", "team")
	if _, code := run(t, a, "sync", "origin"); code == 0 {
		t.Fatal("sync to a team remote should be refused")
	}
}

func TestPullMergesRemoteChanges(t *testing.T) {
	url := startServer(t, protocol.KindTeam)

	// Author A pushes a base commit.
	a := t.TempDir()
	run(t, a, "init")
	run(t, a, "config", "user.name", "A")
	os.WriteFile(filepath.Join(a, "f.txt"), []byte("base\n"), 0o644)
	run(t, a, "add", ".")
	run(t, a, "commit", "-m", "base")
	run(t, a, "remote", "add", "origin", url, "--kind", "team")
	run(t, a, "push", "origin", "main")

	// B clones, A advances and pushes again.
	parent := t.TempDir()
	run(t, parent, "clone", url, "repoB")
	b := filepath.Join(parent, "repoB")

	os.WriteFile(filepath.Join(a, "f.txt"), []byte("base\nmore\n"), 0o644)
	run(t, a, "add", ".")
	run(t, a, "commit", "-m", "more")
	run(t, a, "push", "origin", "main")

	// B pulls and sees the new content.
	if out, code := run(t, b, "pull", "origin"); code != 0 {
		t.Fatalf("pull failed:\n%s", out)
	}
	got, _ := os.ReadFile(filepath.Join(b, "f.txt"))
	if string(got) != "base\nmore\n" {
		t.Errorf("after pull f.txt = %q, want base+more", got)
	}
}
