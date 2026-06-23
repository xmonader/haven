// Package lock provides an advisory repository lock so concurrent mutating
// operations don't corrupt the working tree or the object store. It serializes
// working-tree changes (checkout, merge, rebase, …) and object-store
// maintenance (gc, repack) against each other. The underlying primitive is
// platform-specific (flock on Unix, LockFileEx on Windows); see lock_unix.go
// and lock_windows.go.
package lock

import (
	"fmt"
	"os"
	"path/filepath"
)

// Lock is a held exclusive lock on .haven/wclock.
type Lock struct {
	f *os.File
}

// Acquire takes an exclusive, non-blocking lock for the repository rooted at
// root. It fails immediately if another process holds it.
func Acquire(root string) (*Lock, error) {
	path := filepath.Join(root, ".haven", "wclock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("repository is locked by another haven process")
	}
	return &Lock{f: f}, nil
}

// Release drops the lock.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	unlockFile(l.f)
	return l.f.Close()
}
