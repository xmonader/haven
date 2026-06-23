package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitCreatesRepo(t *testing.T) {
	dir := t.TempDir()
	r, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer r.Close()

	if r.Root != dir {
		// Init resolves to an absolute path; TempDir is already absolute.
		if abs, _ := filepath.Abs(dir); r.Root != abs {
			t.Errorf("Root = %q, want %q", r.Root, abs)
		}
	}

	head, err := r.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if head != DefaultBranch {
		t.Errorf("Head = %q, want %q", head, DefaultBranch)
	}
}

func TestInitTwiceFails(t *testing.T) {
	dir := t.TempDir()
	r, err := Init(dir)
	if err != nil {
		t.Fatalf("first Init: %v", err)
	}
	r.Close()

	if _, err := Init(dir); err != ErrExists {
		t.Errorf("second Init err = %v, want ErrExists", err)
	}
}

func TestOpenDiscoversFromSubdir(t *testing.T) {
	root := t.TempDir()
	r, err := Init(root)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	r.Close()

	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	r2, err := Open(sub)
	if err != nil {
		t.Fatalf("Open from subdir: %v", err)
	}
	defer r2.Close()
	if r2.Root != root {
		t.Errorf("discovered Root = %q, want %q", r2.Root, root)
	}
}

func TestOpenOutsideRepoFails(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(dir); err != ErrNotARepo {
		t.Errorf("Open err = %v, want ErrNotARepo", err)
	}
}
