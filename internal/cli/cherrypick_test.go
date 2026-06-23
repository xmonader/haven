package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cherry-pick a commit from another branch onto main.
func TestCherryPickAppliesCommit(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")
	commitFile(t, dir, "a.txt", "base\n", "base")

	// On a feature branch, add a new file.
	run(t, dir, "branch", "create", "feat")
	run(t, dir, "branch", "switch", "feat")
	commitFile(t, dir, "feature.txt", "hello\n", "add feature")

	// Back on main, cherry-pick that commit.
	run(t, dir, "branch", "switch", "main")
	if _, err := os.Stat(filepath.Join(dir, "feature.txt")); err == nil {
		t.Fatal("feature.txt should not exist on main yet")
	}
	if out, code := run(t, dir, "cherry-pick", "feat"); code != 0 {
		t.Fatalf("cherry-pick failed:\n%s", out)
	}
	got, err := os.ReadFile(filepath.Join(dir, "feature.txt"))
	if err != nil || string(got) != "hello\n" {
		t.Fatalf("feature.txt = %q err=%v", got, err)
	}
	if out, _ := run(t, dir, "status"); !strings.Contains(out, "nothing to commit") {
		t.Errorf("expected clean tree, got:\n%s", out)
	}
	if out, _ := run(t, dir, "log"); !strings.Contains(out, "add feature") {
		t.Errorf("cherry-pick should preserve the message:\n%s", out)
	}
}

// revert undoes a commit's change.
func TestRevertUndoesCommit(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")
	commitFile(t, dir, "a.txt", "v1\n", "first")
	commitFile(t, dir, "a.txt", "v2\n", "second")

	if out, code := run(t, dir, "revert", "HEAD"); code != 0 {
		t.Fatalf("revert failed:\n%s", out)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(got) != "v1\n" {
		t.Errorf("after revert, a.txt = %q, want v1", got)
	}
	if out, _ := run(t, dir, "log"); !strings.Contains(out, "revert") {
		t.Errorf("expected a revert commit in log:\n%s", out)
	}
}
