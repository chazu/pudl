package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The reconcile workspace is a real directory inside the user's mu project:
// `mu build --plan` reads its config from a file, and merging into the project
// is what lets it inherit the project's toolchains and cache. It is removed by a
// deferred Cleanup on every normal exit path, including an error return.
//
// A process that dies before that defer runs leaves the directory behind in the
// project root. This file closes the two halves of that gap which are closable:
// a signal removes the directory before exiting, and a later run sweeps up what
// earlier ones could not. A SIGKILL still leaks — the sweep is what eventually
// collects it.

// workspacePrefix is the temp-dir prefix every reconcile workspace shares. It is
// also what the sweep matches on, so the two must not drift apart.
const workspacePrefix = "pudl_run_"

// staleWorkspaceAge is how old an abandoned workspace must be before a sweep
// will remove it. It exists to make the sweep safe in the presence of a
// concurrently running pudl: another process's workspace is only touched if that
// run has been going for longer than this, which no real run is.
const staleWorkspaceAge = 24 * time.Hour

// removeOnSignal arranges for dir to be removed if the process is interrupted
// before the returned cleanup runs, and returns that cleanup for the caller to
// defer. The cleanup is idempotent and safe to call after a signal.
//
// On SIGINT/SIGTERM the directory is removed, the handler is uninstalled, and
// the signal is re-raised so the process still dies of the signal it was sent
// rather than reporting a plain exit status.
func removeOnSignal(dir string) func() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})

	go func() {
		select {
		case received := <-signals:
			os.RemoveAll(dir)
			signal.Stop(signals)
			// Re-raise against the default disposition so the exit status is the
			// conventional one for a signal death.
			signal.Reset(received)
			if process, err := os.FindProcess(os.Getpid()); err == nil {
				_ = process.Signal(received)
			}
		case <-done:
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			signal.Stop(signals)
			os.RemoveAll(dir)
		})
	}
}

// sweepStaleWorkspaces removes reconcile workspaces under muRoot left behind by
// runs that died without cleaning up — a SIGKILL, a lost terminal, a power cut.
// Only directories older than maxAge are considered, so a workspace belonging to
// a concurrently running pudl is never removed out from under it.
//
// Returns the paths removed. Best-effort by design: it is called for its side
// effect at the start of a run, and a failure to tidy must not fail that run.
func sweepStaleWorkspaces(muRoot string, maxAge time.Duration) []string {
	entries, err := os.ReadDir(muRoot)
	if err != nil {
		return nil
	}
	cutoff := time.Now().Add(-maxAge)

	var removed []string
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), workspacePrefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		path := filepath.Join(muRoot, entry.Name())
		if err := os.RemoveAll(path); err == nil {
			removed = append(removed, path)
		}
	}
	return removed
}

// reportSweptWorkspaces tells the operator what a sweep collected. A leaked
// directory is evidence a previous run died mid-flight, which is worth surfacing
// rather than tidying away in silence.
func reportSweptWorkspaces(removed []string, live bool) {
	if !live || len(removed) == 0 {
		return
	}
	fmt.Printf("note: removed %d abandoned reconcile workspace(s) from earlier run(s) that did not exit cleanly\n",
		len(removed))
	for _, path := range removed {
		fmt.Printf("      %s\n", path)
	}
}
