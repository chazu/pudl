// Package filelock provides a small cross-process advisory lock for short
// repository initialization and generated-workspace critical sections.
package filelock

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// Lock is an exclusive advisory lock held through an open file descriptor.
// The operating system releases it if the process exits unexpectedly.
type Lock struct {
	file *os.File
}

// Acquire opens path and waits for an exclusive lock.
func Acquire(path string) (*Lock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock %s: %w", path, err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("acquire lock %s: %w", path, err)
	}
	return &Lock{file: file}, nil
}

// Release unlocks and closes the lock file. It is not idempotent.
func (l *Lock) Release() error {
	path := l.file.Name()
	unlockErr := unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return fmt.Errorf("release lock %s: %w", path, unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close lock %s: %w", path, closeErr)
	}
	return nil
}
