package scrub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chazu/pudl/internal/artifacts"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/projection"
	"github.com/chazu/pudl/internal/redact"
)

// Outcome is what Apply found and did.
type Outcome struct {
	Plan              *Plan
	ScrubID           string
	Changed           int
	Errors            []string
	Compacted         bool
	CompactionPending bool
}

// Test hooks simulating a crash: after the new files are published and
// before the rewrite commits, and after the commit before old files go.
var (
	beforeCommit func(published []string) error
	afterCommit  func() error
)

// Apply scrubs the catalog under the artifact lock: it finishes an earlier
// interrupted scrub, publishes the rewritten files at new paths, rewrites the
// catalog in one transaction, unlinks the superseded files and compacts
// SQLite. With dryRun it plans under the lock and writes nothing.
func (s *Scrubber) Apply(ctx context.Context, dryRun bool) (*Outcome, error) {
	var out *Outcome
	err := artifacts.WithLock(ctx, s.DB.Root(), func() error {
		plan, err := s.Plan(ctx)
		if err != nil {
			return err
		}
		out = &Outcome{Plan: plan}
		if dryRun {
			return nil
		}
		for _, path := range plan.Leftovers {
			if out.unlink(s.DB, path) {
				out.Changed++
			}
		}
		compact := plan.Uncompacted
		if len(plan.rewrites)+len(plan.reports) > 0 {
			if err := s.commit(ctx, plan, out); err != nil {
				return err
			}
			compact = append(compact, out.ScrubID)
		}
		if len(compact) > 0 {
			err := s.DB.CompactScrubbed(ctx, compact)
			out.Compacted, out.CompactionPending = err == nil, errors.Is(err, database.ErrCompactionBusy)
			if err != nil {
				out.Errors = append(out.Errors, "compaction: "+err.Error())
			} else if len(plan.Uncompacted) > 0 {
				out.Changed++ // an earlier scrub's compaction finished
			}
		}
		return nil
	})
	return out, err
}

// commit publishes the new files and rewrites the catalog. A failure before
// the commit rolls the catalog back and removes the files it published; the
// old files are untouched until after the commit.
func (s *Scrubber) commit(ctx context.Context, plan *Plan, out *Outcome) error {
	tmp := filepath.Join(s.DB.Root(), "data", "tmp")
	if err := artifacts.MkdirPrivate(tmp); err != nil {
		return err
	}
	journal, err := artifacts.NewJournalContext(ctx, tmp)
	if err != nil {
		return err
	}
	defer journal.Close()
	var published []string
	publish := func(content []byte, destination string) error {
		published = append(published, destination)
		return publishBytes(journal, tmp, content, destination)
	}
	for _, rw := range plan.rewrites {
		if rw.mergeInto {
			continue // the surviving entry keeps its own files
		}
		if rw.payload != nil {
			if err := publish(rw.payload, rw.stored); err != nil {
				return err
			}
		}
		if rw.meta != nil {
			if err := publish(rw.meta, rw.metaPath); err != nil {
				return err
			}
		}
	}
	out.ScrubID = idgen.GenerateRandomProquint()
	err = s.DB.WithSecureCatalogTxContext(ctx, func(tx *database.CatalogTx) error {
		redactions, err := s.rewriteCatalog(tx, plan)
		if err != nil {
			return err
		}
		if err := tx.RebuildFactsFTS(); err != nil {
			return err
		}
		if err := tx.RecordSensitiveScrub(out.ScrubID, plan.summary(), redactions); err != nil {
			return err
		}
		if beforeCommit != nil {
			return beforeCommit(published)
		}
		return nil
	})
	if err != nil {
		journal.Rollback()
		out.ScrubID = ""
		return err
	}
	out.Changed += len(plan.rewrites) + len(plan.reports)
	if afterCommit != nil {
		if err := afterCommit(); err != nil {
			return err
		}
	}
	for _, rw := range plan.rewrites {
		for _, path := range rw.superseded() {
			out.unlink(s.DB, path)
		}
	}
	return nil
}

// rewriteCatalog re-keys every rewritten entry, re-projects it, and rewrites
// the matched reports, returning the audit rows.
func (s *Scrubber) rewriteCatalog(tx *database.CatalogTx, plan *Plan) ([]database.SensitiveRedaction, error) {
	var redactions []database.SensitiveRedaction
	for _, rw := range plan.rewrites {
		update := database.ScrubbedEntry{OldID: rw.entry.ID, NewID: rw.newID, ContentHash: rw.newHash,
			StoredPath: rw.stored, MetadataPath: rw.metaPath, SizeBytes: rw.entry.SizeBytes, ResourceID: rw.resourceID}
		if rw.entry.ContentHash != nil {
			update.OldContentHash = *rw.entry.ContentHash
		}
		if rw.payload != nil {
			update.SizeBytes = int64(len(rw.payload))
		}
		merged, err := tx.RekeyScrubbedEntry(update)
		if err != nil {
			return nil, err
		}
		if merged != rw.mergeInto {
			return nil, fmt.Errorf("entry %s: merge target changed since the scrub was planned; rerun it", rw.entry.ID)
		}
		if rw.payload != nil {
			if err := s.reproject(tx, rw); err != nil {
				return nil, fmt.Errorf("re-project %s: %w", rw.entry.ID, err)
			}
		}
		kind := "entry"
		if rw.payload == nil {
			kind = "metadata"
		}
		redactions = append(redactions, database.SensitiveRedaction{SubjectKind: kind, SubjectID: rw.entry.ID, NewID: rw.newID,
			Merged: merged, Paths: rw.paths, OldFiles: rw.superseded()})
	}
	for _, report := range plan.reports {
		if _, err := tx.RewriteReportBlob(report.Kind, report.Key, plan.values.rewriteJSON); err != nil {
			return nil, err
		}
		redactions = append(redactions, database.SensitiveRedaction{SubjectKind: report.Kind, SubjectID: report.Key, Paths: []string{valueMatch}})
	}
	return redactions, nil
}

// reproject corrects the current facts of a rewritten entry that is its
// resource's projected entry: they are recomputed from the redacted record
// under the new ID. A schema that no longer projects only has its state
// repointed. Fact history is not touched.
func (s *Scrubber) reproject(tx *database.CatalogTx, rw *rewrite) error {
	entry := rw.entry
	if entry.ResourceID == nil {
		return nil
	}
	state, err := tx.GetProjectionState(*entry.ResourceID)
	if err != nil || state == nil || state.EntryID != entry.ID {
		return err
	}
	resourceID := *entry.ResourceID
	if rw.resourceID != nil && *rw.resourceID != resourceID {
		resourceID = *rw.resourceID
		if _, _, err := tx.ReconcileProjection(database.ProjectionSource(*entry.ResourceID), nil, database.ProjectionCorrection); err != nil {
			return err
		}
		if err := tx.DeleteProjectionState(*entry.ResourceID); err != nil {
			return err
		}
	}
	var prepared *projection.Prepared
	if s.Projections != nil && rw.record != nil {
		if spec := s.Projections.For(entry.Schema); spec != nil && spec.Err == nil {
			redact.Apply(rw.record, spec.Sensitive)
		}
		prepared, _ = projection.Prepare(s.Projections, entry.Schema, rw.newID, resourceID, entry.IdentityJSON != nil, rw.record)
	}
	if prepared == nil {
		state.EntryID, state.ResourceID = rw.newID, resourceID
		return tx.PutProjectionState(*state)
	}
	return projection.Correct(tx, prepared)
}

// superseded lists the files rw replaces: unlinked after commit when no entry
// references them. A merged entry's row is gone, so all its files go.
func (rw *rewrite) superseded() []string {
	var out []string
	if rw.entry.StoredPath != "" && (rw.mergeInto || rw.stored != rw.entry.StoredPath) {
		out = append(out, rw.entry.StoredPath)
	}
	if rw.entry.MetadataPath != "" && (rw.mergeInto || rw.metaPath != rw.entry.MetadataPath) {
		out = append(out, rw.entry.MetadataPath)
	}
	return out
}

func (o *Outcome) unlink(db *database.CatalogDB, path string) bool {
	removed, err := db.RemoveCommittedOrphan(path)
	if err != nil && !os.IsNotExist(err) {
		o.Errors = append(o.Errors, fmt.Sprintf("unlink superseded %s: %v", path, err))
	}
	return removed
}

// publishBytes installs content at destination through the journal, which
// never overwrites: identical existing bytes are reused.
func publishBytes(journal *artifacts.Journal, tmp string, content []byte, destination string) error {
	f, err := os.CreateTemp(tmp, "scrub-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name) // a no-op once published
	if _, err := f.Write(content); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return journal.Publish(name, destination)
}

// summary is the audit row's JSON: counts per subject kind, never values.
func (p *Plan) summary() string {
	counts := map[string]int{"short_values_skipped": p.ShortValuesSkipped()}
	for _, subject := range p.Subjects() {
		counts[subject.Kind]++
	}
	encoded, _ := json.Marshal(counts)
	return string(encoded)
}
