package cmd

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRegistry() *workspaceRegistry {
	return &workspaceRegistry{dirs: map[string]struct{}{}}
}

func TestWorkspaceRegistry_ReleaseRemovesDirAndUntracks(t *testing.T) {
	reg := newTestRegistry()
	dir := filepath.Join(t.TempDir(), workspacePrefix+"abc")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	release := reg.track(dir)
	assert.Equal(t, []string{dir}, reg.tracked())
	release()

	_, err := os.Stat(dir)
	assert.True(t, os.IsNotExist(err), "release removes the workspace")
	assert.Empty(t, reg.tracked(), "a released workspace is no longer tracked")
}

// Release runs from a defer and from failure paths in setup, so a second call
// must be a no-op.
func TestWorkspaceRegistry_ReleaseIsIdempotent(t *testing.T) {
	reg := newTestRegistry()
	dir := filepath.Join(t.TempDir(), workspacePrefix+"abc")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	release := reg.track(dir)
	release()
	assert.NotPanics(t, release)
}

func TestWorkspaceRegistry_RemoveAll(t *testing.T) {
	reg := newTestRegistry()
	root := t.TempDir()
	a := filepath.Join(root, workspacePrefix+"a")
	b := filepath.Join(root, "pudl_ewe_b")
	require.NoError(t, os.MkdirAll(a, 0o755))
	require.NoError(t, os.MkdirAll(b, 0o755))
	reg.track(a)
	reg.track(b)

	assert.ElementsMatch(t, []string{a, b}, reg.removeAll())
	for _, dir := range []string{a, b} {
		_, err := os.Stat(dir)
		assert.True(t, os.IsNotExist(err), "%s removed", dir)
	}
	assert.Empty(t, reg.tracked())
}

// Without a cancellation the watcher must return when the command finishes and
// remove nothing.
func TestExitOnSecondSignal_ReturnsWhenCommandFinishes(t *testing.T) {
	reg := newTestRegistry()
	dir := filepath.Join(t.TempDir(), workspacePrefix+"keep")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	reg.track(dir)

	done := make(chan struct{})
	returned := make(chan struct{})
	go func() {
		exitOnSecondSignal(context.Background(), done, reg, func(int) { t.Error("must not exit") })
		close(returned)
	}()
	close(done)
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not return after the command finished")
	}
	_, err := os.Stat(dir)
	assert.NoError(t, err, "nothing is removed when no interrupt arrived")
}

// The point of the registry: an operator who interrupts twice gets an
// immediate exit, and no workspace is left in their mu project.
//
// This runs in a subprocess because the second signal exits the process. The
// first signal only cancels the context (as main's signal.NotifyContext does),
// which is the graceful path; the child deliberately does not unwind, so the
// test proves the second signal alone is what removes the directory.
func TestExitOnSecondSignal_RemovesWorkspacesAndExits(t *testing.T) {
	if dir := os.Getenv("PUDL_TEST_SIGNAL_WORKSPACE"); dir != "" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		workspaces.track(dir)
		go exitOnSecondSignal(ctx, make(chan struct{}), workspaces, os.Exit)
		marker := filepath.Dir(dir)
		if err := os.WriteFile(filepath.Join(marker, "ready"), nil, 0o644); err != nil {
			os.Exit(1)
		}
		<-ctx.Done()
		if err := os.WriteFile(filepath.Join(marker, "cancelled"), nil, 0o644); err != nil {
			os.Exit(1)
		}
		time.Sleep(30 * time.Second) // a run that is slow to unwind
		os.Exit(3)
	}

	root := t.TempDir()
	dir := filepath.Join(root, workspacePrefix+"abc")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	child := exec.Command(os.Args[0], "-test.run=^TestExitOnSecondSignal_RemovesWorkspacesAndExits$")
	child.Env = append(os.Environ(), "PUDL_TEST_SIGNAL_WORKSPACE="+dir)
	require.NoError(t, child.Start())
	t.Cleanup(func() { _ = child.Process.Kill() })

	waitFor := func(name string) {
		require.Eventually(t, func() bool {
			_, err := os.Stat(filepath.Join(root, name))
			return err == nil
		}, 10*time.Second, 10*time.Millisecond, "child never reached %q", name)
	}
	waitFor("ready")
	require.NoError(t, child.Process.Signal(syscall.SIGTERM))
	waitFor("cancelled")

	_, err := os.Stat(dir)
	require.NoError(t, err, "the first interrupt alone leaves unwinding to the run")

	// The watcher installs its handler after observing the cancellation; give it
	// a moment so the second signal is not delivered before it listens.
	require.Eventually(t, func() bool {
		_ = child.Process.Signal(syscall.SIGTERM)
		_, statErr := os.Stat(dir)
		return os.IsNotExist(statErr)
	}, 10*time.Second, 50*time.Millisecond, "a second interrupt removes the workspace")

	err = child.Wait()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, forcedExitCode, exitErr.ExitCode(), "a forced exit uses the conventional SIGINT status")
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
