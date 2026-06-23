package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func commitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "add", ".")
	if _, code := run(t, dir, "commit", "-m", msg); code != 0 {
		t.Fatalf("commit %q failed", msg)
	}
}

func TestHavenIsPrivateAndPublishes(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "T")
	commitFile(t, dir, "f.txt", "base\n", "base")

	run(t, dir, "haven", "create", "scratch")
	run(t, dir, "haven", "switch", "scratch")
	commitFile(t, dir, "exp.txt", "wip\n", "experiment")

	// Haven appears in haven list...
	out, _ := run(t, dir, "haven", "list")
	if !strings.Contains(out, "scratch") {
		t.Errorf("haven list missing scratch:\n%s", out)
	}
	// ...but never in branch list.
	out, _ = run(t, dir, "branch", "list")
	if strings.Contains(out, "scratch") {
		t.Errorf("haven leaked into branch list:\n%s", out)
	}

	// Publish graduates it to a public branch.
	if _, code := run(t, dir, "publish", "scratch", "--as", "feature-x"); code != 0 {
		t.Fatal("publish failed")
	}
	out, _ = run(t, dir, "branch", "list")
	if !strings.Contains(out, "feature-x") {
		t.Errorf("published branch missing:\n%s", out)
	}
}

func TestPublishRefusesDivergence(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "T")
	commitFile(t, dir, "f.txt", "base\n", "base")

	// Public branch advances independently.
	run(t, dir, "branch", "create", "feature")
	run(t, dir, "branch", "switch", "feature")
	commitFile(t, dir, "f.txt", "public change\n", "public")

	// A haven diverges from base.
	run(t, dir, "branch", "switch", "main")
	run(t, dir, "haven", "create", "work")
	run(t, dir, "haven", "switch", "work")
	commitFile(t, dir, "g.txt", "private change\n", "private")

	// Publishing onto the diverged public branch must refuse.
	out, code := run(t, dir, "publish", "work", "--as", "feature")
	if code == 0 {
		t.Fatalf("publish onto diverged branch should refuse; got:\n%s", out)
	}
	if !strings.Contains(out, "diverged") {
		t.Errorf("expected divergence error, got:\n%s", out)
	}
}
