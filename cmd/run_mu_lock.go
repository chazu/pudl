package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/chazu/pudl/internal/filelock"
)

// acquireMuProjectLock serializes PUDL-generated subprojects beneath one mu
// root. Mu merges every non-hidden subdirectory into the root configuration, so
// two simultaneous generated projects would otherwise expose duplicate targets
// to each other. The lock covers only the generated workspace and its mu calls;
// catalog work remains concurrent.
func acquireMuProjectLock(muRoot string) (*filelock.Lock, error) {
	lock, err := filelock.Acquire(filepath.Join(muRoot, ".pudl-run.lock"))
	if err != nil {
		return nil, fmt.Errorf("lock mu project %s: %w", muRoot, err)
	}
	return lock, nil
}
