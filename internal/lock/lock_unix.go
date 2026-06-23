//go:build !windows

package lock

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive, non-blocking advisory lock via flock(2).
func lockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// unlockFile releases the flock.
func unlockFile(f *os.File) {
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
