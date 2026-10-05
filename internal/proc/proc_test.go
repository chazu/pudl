package proc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTool writes an executable script and returns its path.
func fakeTool(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tool")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755))
	return path
}

func TestOutputReturnsStdoutAndFoldsStderr(t *testing.T) {
	ok := fakeTool(t, "echo out; echo diag >&2\n")
	out, err := Output(context.Background(), 0, ok)
	require.NoError(t, err)
	assert.Equal(t, "out\n", string(out))

	failing := fakeTool(t, "echo broken >&2; exit 4\n")
	_, err = Output(context.Background(), 0, failing)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken")
	assert.False(t, Cancelled(err))
}

// A cancelled tool is asked to stop with SIGTERM, so it can finish what it is
// writing; the caller sees context.Canceled.
func TestOutputCancelSendsSIGTERM(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "terminated")
	tool := fakeTool(t, "trap 'echo term > "+marker+"; exit 143' TERM\nwhile :; do sleep 0.05; done\n")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := Output(ctx, 0, tool)
	require.Error(t, err)
	assert.True(t, Cancelled(err), "cancellation is reported as context.Canceled: %v", err)
	assert.Less(t, time.Since(start), DefaultGrace, "a tool that honours SIGTERM ends well within the grace period")

	data, readErr := os.ReadFile(marker)
	require.NoError(t, readErr, "the tool received SIGTERM, not an immediate SIGKILL")
	assert.Equal(t, "term", strings.TrimSpace(string(data)))
}

// A tool that ignores SIGTERM is killed once the grace period ends.
func TestCommandKillsAfterGrace(t *testing.T) {
	tool := fakeTool(t, "trap '' TERM\nwhile :; do sleep 0.05; done\n")
	ctx, cancel := context.WithCancel(context.Background())
	command := Command(ctx, 300*time.Millisecond, tool)
	require.NoError(t, command.Start())
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	cancel()
	err := command.Wait()
	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second, "the grace period bounds the wait")
}

func TestOutputTimeoutIsNotACancellation(t *testing.T) {
	tool := fakeTool(t, "while :; do sleep 0.05; done\n")
	_, err := Output(context.Background(), 200*time.Millisecond, tool)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "%v", err)
	assert.False(t, Cancelled(err), "a timeout is a failure, not an operator cancellation")
	assert.Contains(t, err.Error(), "timed out after 200ms")
}
