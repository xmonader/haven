package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGcReclaimsUnreachableObjects(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "T")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644)
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "base")

	// Create a haven with a unique object, then delete the haven: its objects
	// become unreachable.
	run(t, dir, "haven", "create", "junk")
	run(t, dir, "haven", "switch", "junk")
	os.WriteFile(filepath.Join(dir, "garbage.txt"), []byte("throwaway\n"), 0o644)
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "junk")
	run(t, dir, "branch", "switch", "main")
	run(t, dir, "haven", "delete", "junk")

	out, code := run(t, dir, "gc")
	if code != 0 {
		t.Fatalf("gc failed:\n%s", out)
	}
	if strings.Contains(out, "removed 0 ") {
		t.Errorf("gc should have reclaimed the deleted haven's objects:\n%s", out)
	}

	// Repository remains consistent afterwards.
	if out, code := run(t, dir, "fsck"); code != 0 {
		t.Fatalf("fsck after gc failed:\n%s", out)
	}
}

func TestFsckOnHealthyRepo(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "T")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0o644)
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "c")

	out, code := run(t, dir, "fsck")
	if code != 0 || !strings.Contains(out, "no corruption") {
		t.Fatalf("fsck on healthy repo:\n%s", out)
	}
}
