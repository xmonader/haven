package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBranchCreateSwitchCheckout(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "T")

	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("f.txt", "v1\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "base")

	if _, code := run(t, dir, "branch", "create", "feature"); code != 0 {
		t.Fatal("branch create failed")
	}
	if _, code := run(t, dir, "branch", "switch", "feature"); code != 0 {
		t.Fatal("branch switch failed")
	}

	write("f.txt", "v2\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "edit")

	// Switch back to main: working tree must revert to v1.
	if _, code := run(t, dir, "branch", "switch", "main"); code != 0 {
		t.Fatal("switch back failed")
	}
	got, _ := os.ReadFile(filepath.Join(dir, "f.txt"))
	if string(got) != "v1\n" {
		t.Errorf("after switch to main, f.txt = %q, want v1", got)
	}

	out, _ := run(t, dir, "branch", "list")
	if !strings.Contains(out, "* main") || !strings.Contains(out, "feature") {
		t.Errorf("branch list:\n%s", out)
	}
}

func TestSwitchRefusesDirtyTree(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "T")
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("v1\n"), 0o644)
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "base")
	run(t, dir, "branch", "create", "other")

	// Dirty the tree.
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("dirty\n"), 0o644)
	if _, code := run(t, dir, "branch", "switch", "other"); code == 0 {
		t.Fatal("switch should refuse a dirty working tree")
	}
}

func TestCannotDeleteCurrentBranch(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	if _, code := run(t, dir, "branch", "delete", "main"); code == 0 {
		t.Fatal("deleting current branch should fail")
	}
}
