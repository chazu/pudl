package filelock

import (
	"context"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireContextCancelsContendedLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	held, err := Acquire(path)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = AcquireContext(ctx, path)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), time.Second)
	require.NoError(t, held.Release())
	next, err := AcquireContext(context.Background(), path)
	require.NoError(t, err)
	require.NoError(t, next.Release())
}
