// Package lock provides an advisory working-copy lock so concurrent mutating
// operations (commit, checkout, merge) don't corrupt the working tree.
package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Lock is a held flock on .haven/wclock.
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
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("repository is locked by another haven process")
	}
	return &Lock{f: f}, nil
}

// Release drops the lock.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	return l.f.Close()
}
