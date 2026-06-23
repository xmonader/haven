package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSymlinkTrackedAndRestored(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")

	// main: a regular file only.
	os.WriteFile(filepath.Join(dir, "real.txt"), []byte("hi\n"), 0o644)
	run(t, dir, "add", ".")
	if out, code := run(t, dir, "commit", "-m", "base"); code != 0 {
		t.Fatalf("commit failed:\n%s", out)
	}

	// branch x: add a symlink and commit it there.
	run(t, dir, "branch", "create", "x")
	run(t, dir, "branch", "switch", "x")
	if err := os.Symlink("real.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Skipf("symlinks unsupported here: %v", err)
	}
	run(t, dir, "add", ".")
	if out, code := run(t, dir, "commit", "-m", "add link"); code != 0 {
		t.Fatalf("commit on x failed:\n%s", out)
	}
	if out, _ := run(t, dir, "status"); !strings.Contains(out, "nothing to commit") {
		t.Errorf("expected clean tree on x (symlink hashed by target), got:\n%s", out)
	}

	// Switching to main must remove the link; switching back must recreate it
	// AS a symlink via checkout.
	run(t, dir, "branch", "switch", "main")
	if _, err := os.Lstat(filepath.Join(dir, "link.txt")); !os.IsNotExist(err) {
		t.Fatalf("link should be absent on main, got err=%v", err)
	}
	run(t, dir, "branch", "switch", "x")

	fi, err := os.Lstat(filepath.Join(dir, "link.txt"))
	if err != nil {
		t.Fatalf("link not restored: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("restored entry is not a symlink")
	}
	if target, _ := os.Readlink(filepath.Join(dir, "link.txt")); target != "real.txt" {
		t.Errorf("symlink target = %q, want real.txt", target)
	}
}
