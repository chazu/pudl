// Package acute contains the PUDL-owned run policy: run identity, run-set
// ordering, drift comparison and the verdict summary. It has no subprocess or
// catalog dependencies; those belong to adapters at the run seam.
package acute

import (
	"github.com/chazu/pudl/internal/idgen"
)

// RunSession gives one run a durable audit identity.
type RunSession struct {
	RunID string
	// SnapshotID is allocated with the run, not by the ingest that writes it.
	// PUDL decides when to observe, so it owns the identifier for the
	// observation it initiates — which is what lets a run name the snapshot a
	// failed ingest would have produced, instead of discarding the ID with the
	// error and leaving the partial state unnamed.
	SnapshotID string
}

// NewRunSession allocates the identities of one run.
func NewRunSession() *RunSession {
	return &RunSession{
		RunID:      "run_" + idgen.GenerateRandomProquint(),
		SnapshotID: "snap_" + idgen.GenerateRandomProquint(),
	}
}

// NewCheckID allocates the identity of one check (its report ID).
func NewCheckID() string { return "check_" + idgen.GenerateRandomProquint() }

func NewMemberRunID() string { return "run_" + idgen.GenerateRandomProquint() }
