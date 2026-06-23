package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStashSaveAndPop(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")
	commitFile(t, dir, "a.txt", "committed\n", "base")

	// Make an uncommitted change, then stash it.
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("work in progress\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0o644)
	run(t, dir, "add", "a.txt")
	if out, code := run(t, dir, "stash"); code != 0 {
		t.Fatalf("stash save failed:\n%s", out)
	}

	// Working tree is back to the committed state.
	if got, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(got) != "committed\n" {
		t.Errorf("after stash, a.txt = %q, want committed", got)
	}
	if out, _ := run(t, dir, "status"); !strings.Contains(out, "nothing to commit") {
		t.Errorf("expected clean tree after stash, got:\n%s", out)
	}
	if out, _ := run(t, dir, "stash", "list"); !strings.Contains(out, "stash@{0}") {
		t.Errorf("stash list missing entry:\n%s", out)
	}

	// Pop restores the changes.
	if out, code := run(t, dir, "stash", "pop"); code != 0 {
		t.Fatalf("stash pop failed:\n%s", out)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(got) != "work in progress\n" {
		t.Errorf("after pop, a.txt = %q, want work in progress", got)
	}
	// Stash is now empty.
	if out, _ := run(t, dir, "stash", "list"); !strings.Contains(out, "no stashes") {
		t.Errorf("stash should be empty after pop:\n%s", out)
	}
}

func TestStashNoChanges(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")
	commitFile(t, dir, "a.txt", "x\n", "base")
	if out, _ := run(t, dir, "stash"); !strings.Contains(out, "no local changes") {
		t.Errorf("expected no-changes message, got:\n%s", out)
	}
}
