package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func headHash(t *testing.T, dir string) string {
	t.Helper()
	out, _ := run(t, dir, "log")
	first := strings.SplitN(out, "\n", 2)[0]
	return strings.TrimPrefix(first, "commit ")
}

func TestBisectFindsFirstBadCommit(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init")
	run(t, dir, "config", "user.name", "Dev")

	// Linear history; the "BUG" marker first appears at commit index 3.
	var firstHash, culpritHash, badHash string
	for i := 0; i < 7; i++ {
		content := fmt.Sprintf("v%d\n", i)
		if i >= 3 {
			content = fmt.Sprintf("BUG v%d\n", i)
		}
		commitFile(t, dir, "code.txt", content, fmt.Sprintf("commit %d", i))
		h := headHash(t, dir)
		switch i {
		case 0:
			firstHash = h
		case 3:
			culpritHash = h
		case 6:
			badHash = h
		}
	}

	run(t, dir, "bisect", "start")
	run(t, dir, "bisect", "good", firstHash)
	out, _ := run(t, dir, "bisect", "bad", badHash)

	for step := 0; step < 12; step++ {
		if strings.Contains(out, "first bad commit") {
			break
		}
		content, _ := os.ReadFile(filepath.Join(dir, "code.txt"))
		if strings.Contains(string(content), "BUG") {
			out, _ = run(t, dir, "bisect", "bad")
		} else {
			out, _ = run(t, dir, "bisect", "good")
		}
	}

	if !strings.Contains(out, "first bad commit") {
		t.Fatalf("bisect did not converge:\n%s", out)
	}
	if !strings.Contains(out, culpritHash[:10]) {
		t.Errorf("bisect identified the wrong commit; want %s\n%s", culpritHash[:10], out)
	}

	// Reset returns to the original branch and removes the work ref.
	if out, code := run(t, dir, "bisect", "reset"); code != 0 || !strings.Contains(out, "back on main") {
		t.Fatalf("bisect reset failed (code %d):\n%s", code, out)
	}
	if out, _ := run(t, dir, "branch", "list"); strings.Contains(out, "bisect") {
		t.Errorf("bisect work ref leaked into branch list:\n%s", out)
	}
}
