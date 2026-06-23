package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// run dispatches argv in dir, returning combined stdout and the exit code.
func run(t *testing.T, dir string, argv ...string) (string, int) {
	t.Helper()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	var out, errOut bytes.Buffer
	code := Dispatch(argv, &out, &errOut)
	return out.String() + errOut.String(), code
}

func TestInitAddCommitLog(t *testing.T) {
	dir := t.TempDir()

	if _, code := run(t, dir, "init"); code != 0 {
		t.Fatalf("init exit %d", code)
	}
	if _, code := run(t, dir, "config", "user.name", "Tester"); code != 0 {
		t.Fatal("config name failed")
	}

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := run(t, dir, "status")
	if code != 0 || !strings.Contains(out, "Untracked files:") || !strings.Contains(out, "a.txt") {
		t.Fatalf("status before add:\n%s", out)
	}

	if _, code := run(t, dir, "add", "."); code != 0 {
		t.Fatal("add failed")
	}

	out, code = run(t, dir, "commit", "-m", "first")
	if code != 0 || !strings.Contains(out, "first") {
		t.Fatalf("commit:\n%s", out)
	}

	out, code = run(t, dir, "log")
	if code != 0 || !strings.Contains(out, "first") || !strings.Contains(out, "Tester") {
		t.Fatalf("log:\n%s", out)
	}

	// Clean working tree after commit.
	out, _ = run(t, dir, "status")
	if !strings.Contains(out, "nothing to commit") {
		t.Fatalf("expected clean status, got:\n%s", out)
	}
}

func TestCommitRequiresMessage(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644)
	run(t, dir, "add", ".")
	if _, code := run(t, dir, "commit"); code == 0 {
		t.Fatal("commit without -m should fail")
	}
}

func TestCommandsOutsideRepoFail(t *testing.T) {
	dir := t.TempDir()
	if _, code := run(t, dir, "status"); code == 0 {
		t.Fatal("status outside repo should fail")
	}
}
