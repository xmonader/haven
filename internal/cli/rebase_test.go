package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRebaseReplaysOntoUpstream(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")
	commitFile(t, dir, "base.txt", "base\n", "base")

	// feature branch diverges with its own commit.
	run(t, dir, "branch", "create", "feat")
	run(t, dir, "branch", "switch", "feat")
	commitFile(t, dir, "feat.txt", "f\n", "feat work")

	// main advances independently.
	run(t, dir, "branch", "switch", "main")
	commitFile(t, dir, "main.txt", "m\n", "main work")

	// Rebase feature onto main.
	run(t, dir, "branch", "switch", "feat")
	if out, code := run(t, dir, "rebase", "main"); code != 0 {
		t.Fatalf("rebase failed:\n%s", out)
	}

	// feature now contains main's file AND its own, on top of main.
	for _, f := range []string{"base.txt", "main.txt", "feat.txt"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("expected %s present after rebase: %v", f, err)
		}
	}
	if out, _ := run(t, dir, "status"); !strings.Contains(out, "nothing to commit") {
		t.Errorf("expected clean tree, got:\n%s", out)
	}
	// main work must be an ancestor now: log shows both messages.
	out, _ := run(t, dir, "log")
	if !strings.Contains(out, "main work") || !strings.Contains(out, "feat work") {
		t.Errorf("rebased log missing commits:\n%s", out)
	}
}

func TestRebaseConflictRollsBack(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")
	commitFile(t, dir, "x.txt", "base\n", "base")

	run(t, dir, "branch", "create", "feat")
	run(t, dir, "branch", "switch", "feat")
	commitFile(t, dir, "x.txt", "feat-version\n", "feat edit")

	run(t, dir, "branch", "switch", "main")
	commitFile(t, dir, "x.txt", "main-version\n", "main edit")

	run(t, dir, "branch", "switch", "feat")
	featTipBefore, _ := run(t, dir, "log")
	if _, code := run(t, dir, "rebase", "main"); code == 0 {
		t.Fatal("expected rebase to report a conflict")
	}
	// Branch must be left unchanged (rolled back) and tree clean.
	if out, _ := run(t, dir, "status"); !strings.Contains(out, "nothing to commit") {
		t.Errorf("expected clean tree after rollback, got:\n%s", out)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "x.txt")); string(got) != "feat-version\n" {
		t.Errorf("x.txt after rollback = %q, want feat-version", got)
	}
	if featTipAfter, _ := run(t, dir, "log"); featTipBefore != featTipAfter {
		t.Errorf("history changed despite conflict rollback")
	}
}
