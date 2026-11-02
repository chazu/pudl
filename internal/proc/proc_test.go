package proc

import (
	"context"
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

// A cancelled tool is asked to stop with SIGTERM, so it can finish what it is
// writing; Stopped reports the caller's cancellation.
func TestCommandCancelSendsSIGTERM(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "terminated")
	tool := fakeTool(t, "trap 'echo term > "+marker+"; exit 143' TERM\nwhile :; do sleep 0.05; done\n")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	err := Command(ctx, DefaultGrace, tool).Run()
	require.Error(t, err)
	stopped := Stopped(ctx, ctx)
	assert.True(t, Cancelled(stopped), "cancellation is reported as context.Canceled: %v", stopped)
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

// A per-run timeout is a failure, not an operator cancellation.
func TestStoppedTimeoutIsNotACancellation(t *testing.T) {
	parent := context.Background()
	run, cancel := context.WithTimeout(parent, time.Millisecond)
	defer cancel()
	<-run.Done()
	stopped := Stopped(parent, run)
	require.Error(t, stopped)
	assert.False(t, Cancelled(stopped))
}
