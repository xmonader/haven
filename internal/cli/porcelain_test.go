package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResetHardMovesBranchAndWorkingTree(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")
	commitFile(t, dir, "a.txt", "v1\n", "first")
	commitFile(t, dir, "a.txt", "v2\n", "second")

	// 'keep' marks the v2 commit; a third commit then advances main.
	run(t, dir, "branch", "create", "keep")
	commitFile(t, dir, "a.txt", "v3\n", "third")

	if out, code := run(t, dir, "reset", "--hard", "keep"); code != 0 {
		t.Fatalf("reset failed:\n%s", out)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(got) != "v2\n" {
		t.Errorf("after reset --hard, a.txt = %q, want v2", got)
	}
	if out, _ := run(t, dir, "status"); !strings.Contains(out, "nothing to commit") {
		t.Errorf("expected clean tree after reset --hard, got:\n%s", out)
	}
}

func TestRestoreFileFromHead(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")
	commitFile(t, dir, "a.txt", "committed\n", "c")

	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("local edit\n"), 0o644)
	if out, code := run(t, dir, "restore", "a.txt"); code != 0 {
		t.Fatalf("restore failed:\n%s", out)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(got) != "committed\n" {
		t.Errorf("restore gave %q, want committed", got)
	}
}

func TestTagCreateListDelete(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")
	commitFile(t, dir, "a.txt", "x\n", "c")

	if out, code := run(t, dir, "tag", "v1.0"); code != 0 {
		t.Fatalf("tag create failed:\n%s", out)
	}
	if _, code := run(t, dir, "tag", "v1.0"); code == 0 {
		t.Fatal("duplicate tag should fail")
	}
	if out, _ := run(t, dir, "tag", "list"); !strings.Contains(out, "v1.0") {
		t.Errorf("tag list missing v1.0:\n%s", out)
	}
	if out, _ := run(t, dir, "branch", "list"); strings.Contains(out, "v1.0") {
		t.Errorf("tag leaked into branch list:\n%s", out)
	}
	run(t, dir, "tag", "delete", "v1.0")
	if out, _ := run(t, dir, "tag", "list"); strings.Contains(out, "v1.0") {
		t.Errorf("tag not deleted:\n%s", out)
	}
}
