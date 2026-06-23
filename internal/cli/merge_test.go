package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeCleanThreeWay(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "T")
	commitFile(t, dir, "f.txt", "a\nb\nc\n", "base")

	run(t, dir, "branch", "create", "topic")
	run(t, dir, "branch", "switch", "topic")
	commitFile(t, dir, "t.txt", "topic\n", "topic")

	run(t, dir, "branch", "switch", "main")
	commitFile(t, dir, "m.txt", "main\n", "main")

	out, code := run(t, dir, "merge", "topic")
	if code != 0 {
		t.Fatalf("clean merge failed:\n%s", out)
	}
	// Both side files present after merge.
	for _, f := range []string{"t.txt", "m.txt", "f.txt"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s after merge", f)
		}
	}
}

func TestMergeConflict(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "T")
	commitFile(t, dir, "f.txt", "a\nb\nc\n", "base")

	run(t, dir, "branch", "create", "topic")
	run(t, dir, "branch", "switch", "topic")
	commitFile(t, dir, "f.txt", "a\nTHEIRS\nc\n", "theirs")

	run(t, dir, "branch", "switch", "main")
	commitFile(t, dir, "f.txt", "a\nOURS\nc\n", "ours")

	out, code := run(t, dir, "merge", "topic")
	if code == 0 {
		t.Fatalf("conflicting merge should fail; got:\n%s", out)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "f.txt"))
	if !strings.Contains(string(got), "<<<<<<<") || !strings.Contains(string(got), ">>>>>>>") {
		t.Errorf("conflict markers not written:\n%s", got)
	}
}

func TestMergeFastForward(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "T")
	commitFile(t, dir, "f.txt", "base\n", "base")

	run(t, dir, "branch", "create", "ahead")
	run(t, dir, "branch", "switch", "ahead")
	commitFile(t, dir, "f.txt", "base\nmore\n", "ahead")

	run(t, dir, "branch", "switch", "main")
	out, code := run(t, dir, "merge", "ahead")
	if code != 0 || !strings.Contains(out, "fast-forward") {
		t.Fatalf("expected fast-forward:\n%s", out)
	}
}
