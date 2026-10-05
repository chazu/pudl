package cmd

import (
	"context"
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
// an interrupt releases every registered directory (see workspaceRegistry), and
// a later run sweeps up what earlier ones could not. A SIGKILL still leaks — the
// sweep is what eventually collects it.

// workspacePrefix is the temp-dir prefix every reconcile workspace shares. It is
// also what the sweep matches on, so the two must not drift apart.
const workspacePrefix = "pudl_run_"

// staleWorkspaceAge is how old an abandoned workspace must be before a sweep
// will remove it. It exists to make the sweep safe in the presence of a
// concurrently running pudl: another process's workspace is only touched if that
// run has been going for longer than this, which no real run is.
const staleWorkspaceAge = 24 * time.Hour

// forcedExitCode is the conventional status of a process ended by SIGINT.
const forcedExitCode = 130

// workspaceRegistry tracks every temporary directory a run is holding — the
// reconcile and populate workspaces inside the user's mu project, the staged
// ewe project, the ad-hoc mu root — so an interrupt can take them all back out.
//
// An interrupt is handled in two steps. The first SIGINT/SIGTERM cancels the
// invocation's context (main's signal.NotifyContext): mu is asked to stop, the
// run unwinds through its ordinary defers, each workspace is released, and the
// run row records a cancelled conclusion. A second signal means the operator
// will not wait for that; exitOnSecondSignal then removes whatever is still
// registered and exits at once. Only a SIGKILL leaks, and the stale-workspace
// sweep collects that later.
type workspaceRegistry struct {
	mu   sync.Mutex
	dirs map[string]struct{}
}

// workspaces is process-wide because signals are.
var workspaces = &workspaceRegistry{dirs: map[string]struct{}{}}

// track registers dir and returns its release: remove the directory and stop
// tracking it. The release is idempotent, so it can be deferred and also called
// from a failure path.
func (r *workspaceRegistry) track(dir string) func() {
	r.mu.Lock()
	r.dirs[dir] = struct{}{}
	r.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			delete(r.dirs, dir)
			r.mu.Unlock()
			_ = os.RemoveAll(dir)
		})
	}
}

// removeAll removes every tracked directory and returns their paths.
func (r *workspaceRegistry) removeAll() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	removed := make([]string, 0, len(r.dirs))
	for dir := range r.dirs {
		_ = os.RemoveAll(dir)
		removed = append(removed, dir)
		delete(r.dirs, dir)
	}
	return removed
}

// tracked reports the directories currently registered.
func (r *workspaceRegistry) tracked() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	dirs := make([]string, 0, len(r.dirs))
	for dir := range r.dirs {
		dirs = append(dirs, dir)
	}
	return dirs
}

// exitOnSecondSignal waits for ctx to be cancelled by the first interrupt, then
// for a second one, at which point it removes every registered workspace and
// exits immediately. It returns when done closes without a cancellation.
func exitOnSecondSignal(ctx context.Context, done <-chan struct{}, reg *workspaceRegistry, exit func(int)) {
	select {
	case <-ctx.Done():
	case <-done:
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	fmt.Fprintln(os.Stderr, "interrupted: stopping mu and recording the run; interrupt again to exit immediately")
	select {
	case <-signals:
		reg.removeAll()
		exit(forcedExitCode)
	case <-done:
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
