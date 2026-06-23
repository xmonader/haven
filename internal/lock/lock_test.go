package lock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockIsExclusive(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".haven"), 0o755); err != nil {
		t.Fatal(err)
	}
	l1, err := Acquire(root)
	if err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}
	if _, err := Acquire(root); err == nil {
		t.Fatal("second Acquire should fail while the lock is held")
	}
	if err := l1.Release(); err != nil {
		t.Fatal(err)
	}
	l2, err := Acquire(root)
	if err != nil {
		t.Fatalf("Acquire after Release failed: %v", err)
	}
	l2.Release()
}
