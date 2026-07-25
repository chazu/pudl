package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveOnSignal_CleanupRemovesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), workspacePrefix+"abc")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	cleanup := removeOnSignal(dir)
	cleanup()

	_, err := os.Stat(dir)
	assert.True(t, os.IsNotExist(err), "cleanup removes the workspace")
}

// Cleanup runs from a defer and from each failure path in setup, so calling it
// twice must not panic on the closed channel.
func TestRemoveOnSignal_CleanupIsIdempotent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), workspacePrefix+"abc")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	cleanup := removeOnSignal(dir)
	cleanup()
	assert.NotPanics(t, cleanup, "a second cleanup is a no-op, not a double close")
}

// The point of the handler: a killed run must not leave its workspace in the
// user's mu project.
//
// This runs in a subprocess because the handler deliberately re-raises the
// signal against the default disposition, which would take the test binary down
// with it. The subprocess also makes the assertion the honest one — the
// directory is checked from the outside, after the signalled process is dead.
func TestRemoveOnSignal_RemovesDirOnSignal(t *testing.T) {
	if dir := os.Getenv("PUDL_TEST_SIGNAL_WORKSPACE"); dir != "" {
		// Child half: install the handler, announce readiness, wait to be killed.
		defer removeOnSignal(dir)()
		if err := os.WriteFile(filepath.Join(filepath.Dir(dir), "ready"), nil, 0o644); err != nil {
			os.Exit(1)
		}
		time.Sleep(30 * time.Second)
		return
	}

	root := t.TempDir()
	dir := filepath.Join(root, workspacePrefix+"abc")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	child := exec.Command(os.Args[0], "-test.run=^TestRemoveOnSignal_RemovesDirOnSignal$")
	child.Env = append(os.Environ(), "PUDL_TEST_SIGNAL_WORKSPACE="+dir)
	require.NoError(t, child.Start())
	t.Cleanup(func() { _ = child.Process.Kill() })

	// Wait for the handler to be installed; signalling before that proves nothing.
	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(root, "ready"))
		return err == nil
	}, 10*time.Second, 10*time.Millisecond, "child never became ready")

	require.NoError(t, child.Process.Signal(syscall.SIGTERM))
	_ = child.Wait() // dies of the signal; a non-nil error is the expected outcome

	_, err := os.Stat(dir)
	assert.True(t, os.IsNotExist(err), "a signalled run removes its workspace before exiting")
}

func TestSweepStaleWorkspaces(t *testing.T) {
	muRoot := t.TempDir()

	mkdir := func(name string, age time.Duration) string {
		path := filepath.Join(muRoot, name)
		require.NoError(t, os.MkdirAll(path, 0o755))
		when := time.Now().Add(-age)
		require.NoError(t, os.Chtimes(path, when, when))
		return path
	}

	stale := mkdir(workspacePrefix+"old", 48*time.Hour)
	fresh := mkdir(workspacePrefix+"live", time.Minute)
	unrelated := mkdir("src", 48*time.Hour)

	// A file, not a directory, that happens to share the prefix.
	strayFile := filepath.Join(muRoot, workspacePrefix+"notadir")
	require.NoError(t, os.WriteFile(strayFile, []byte("x"), 0o644))
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(strayFile, old, old))

	removed := sweepStaleWorkspaces(muRoot, staleWorkspaceAge)

	require.Len(t, removed, 1, "only the stale workspace is collected")
	assert.Equal(t, stale, removed[0])

	assertExists := func(path, why string) {
		_, err := os.Stat(path)
		assert.NoError(t, err, why)
	}
	assertExists(fresh, "a concurrent run's workspace must survive the sweep")
	assertExists(unrelated, "a directory that is not a workspace is not touched")
	assertExists(strayFile, "a non-directory sharing the prefix is not touched")

	_, err := os.Stat(stale)
	assert.True(t, os.IsNotExist(err), "the abandoned workspace is gone")
}

func TestSweepStaleWorkspaces_MissingRootIsNotAnError(t *testing.T) {
	assert.Nil(t, sweepStaleWorkspaces(filepath.Join(t.TempDir(), "absent"), staleWorkspaceAge))
}
