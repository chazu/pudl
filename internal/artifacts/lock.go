// Package artifacts coordinates publication and reclamation of catalog files.
package artifacts

import (
	"context"
	"os"
	"path/filepath"

	"github.com/chazu/pudl/internal/filelock"
)

// WithLock holds the workspace's artifact lock while fn publishes or reclaims
// files and commits their catalog references. Acquire this lock before opening
// a catalog transaction. Preparation of private temporary files needs no lock.
func WithLock(ctx context.Context, pudlRoot string, fn func() error) error {
	path := filepath.Join(pudlRoot, "data", "sqlite", "artifacts.lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	lock, err := filelock.AcquireContext(ctx, path)
	if err != nil {
		return err
	}
	defer lock.Release()
	return fn()
}
