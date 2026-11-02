package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// A run's temporary directories are removed by a deferred release on every
// normal exit path, including an error return. An interrupt releases every
// registered directory (see workspaceRegistry). Only a SIGKILL leaks.

// workspacePrefix is the temp-dir prefix every generated mu workspace shares.
const workspacePrefix = "pudl_run_"

// forcedExitCode is the conventional status of a process ended by SIGINT.
const forcedExitCode = 130

// workspaceRegistry tracks every temporary directory a run is holding — the
// populate workspaces inside the user's mu project, the staged
// ewe project, the ad-hoc mu root — so an interrupt can take them all back out.
//
// An interrupt is handled in two steps. The first SIGINT/SIGTERM cancels the
// invocation's context (main's signal.NotifyContext): mu is asked to stop, the
// run unwinds through its ordinary defers, each workspace is released, and the
// run row records a cancelled conclusion. A second signal means the operator
// will not wait for that; exitOnSecondSignal then removes whatever is still
// registered and exits at once. Only a SIGKILL leaks.
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
