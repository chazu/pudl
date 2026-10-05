// Package filelock provides a small cross-process advisory lock for short
// repository initialization and generated-workspace critical sections.
package filelock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// Lock is an exclusive advisory lock held through an open file descriptor.
// The operating system releases it if the process exits unexpectedly.
type Lock struct {
	file *os.File
}

// Acquire opens path and waits for an exclusive lock.
func Acquire(path string) (*Lock, error) {
	return AcquireContext(context.Background(), path)
}

// AcquireContext waits for a lock until cancellation, closing the descriptor on failure.
func AcquireContext(ctx context.Context, path string) (*Lock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock %s: %w", path, err)
	}
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return nil, err
		}
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) && !errors.Is(err, unix.EINTR) {
			_ = file.Close()
			return nil, fmt.Errorf("acquire lock %s: %w", path, err)
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
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
