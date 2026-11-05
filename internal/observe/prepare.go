package observe

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/artifacts"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/ingestprep"
	"github.com/chazu/pudl/internal/policyfacts"
	"github.com/chazu/pudl/internal/projection"
)

// PreparedObservation owns private record files and a disk-backed descriptor
// spool. No catalog write lock is held during decoding, inference or hashing.
type PreparedObservation struct {
	in       ObserveIngest
	dir      string
	spool    *os.File
	snapshot database.ObserveSnapshot
	entry    database.CatalogEntry
	rawDir   string
	empty    bool
	route    observeRoute
	// projector computes schema-declared facts; tally reports them.
	projector *projection.Registry
	tally     *projection.Tally
	// policies evaluates compliance policies per record.
	policies *policyfacts.Evaluator
}

// observeDescriptor is one spooled record: its catalog entry and its facts.
type observeDescriptor struct {
	database.CatalogEntry
	Projection *projection.Prepared  `json:"projection,omitempty"`
	Policies   *policyfacts.Prepared `json:"policies,omitempty"`
}

func (p *PreparedObservation) Close() {
	if p.spool != nil {
		_ = p.spool.Close()
	}
	_ = os.RemoveAll(p.dir)
}

func PrepareObservation(in ObserveIngest) (_ *PreparedObservation, resultErr error) {
	if in.Context == nil {
		in.Context = context.Background()
	}
	limits, err := in.Limits.Resolve()
	if err != nil {
		return nil, err
	}
	if in.Origin == "" {
		in.Origin = database.SnapshotSourceMuObserve
	}
	if in.Source == "" {
		in.Source = database.SnapshotSourceIngestObserve
	}
	if in.SnapshotID == "" {
		in.SnapshotID = NewSnapshotID()
	}
	if filepath.Base(in.SnapshotID) != in.SnapshotID || in.SnapshotID == "." || in.SnapshotID == ".." {
		return nil, fmt.Errorf("snapshot id must be a filename component")
	}
	tmp := filepath.Join(in.DataDir, "tmp")
	if err := artifacts.MkdirPrivate(tmp); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(tmp, "observe-prepare-*")
	if err != nil {
		return nil, err
	}
	p := &PreparedObservation{in: in, dir: dir}
	defer func() {
		if resultErr != nil {
			p.Close()
		}
	}()
	p.spool, err = artifacts.CreatePrivate(filepath.Join(dir, "entries.ndjson"))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	p.rawDir = filepath.Join(in.DataDir, "raw", now.Format("2006/01/02"))
	p.snapshot = database.ObserveSnapshot{Scope: in.Scope, Complete: in.Complete, Schemas: append([]string(nil), in.Schemas...), SnapshotID: in.SnapshotID, RunID: in.RunID, Model: in.Model, Workspace: in.Workspace, Origin: in.Origin, Source: in.Source, CreatedAt: now}
	if in.ManualSchema != "" {
		p.snapshot.Schemas = append(p.snapshot.Schemas, in.ManualSchema)
	}
	if in.Complete && in.Scope == "" {
		return nil, fmt.Errorf("complete observation requires an explicit scope")
	}
	reader := bufio.NewReader(&ingestprep.Reader{Context: in.Context, Source: in.Reader, Remaining: limits.DecodedBytes})
	// Historical empty-input behavior is retained, without buffering input.
	for {
		b, err := reader.Peek(1)
		if err == io.EOF {
			p.empty = true
			return p, nil
		}
		if err != nil {
			return nil, err
		}
		if !strings.ContainsRune(" \t\r\n", rune(b[0])) {
			break
		}
		_, _ = reader.ReadByte()
	}
	route, err := newObserveRoute(in)
	if err != nil {
		return nil, err
	}
	p.route = route
	p.projector = in.Projection
	if p.projector == nil && in.Inferrer != nil {
		p.projector = projection.NewRegistry(in.Inferrer, route.redactor)
	}
	p.tally = projection.NewTally()
	if in.Inferrer != nil {
		p.policies = policyfacts.New(in.Inferrer)
	}
	counts := map[string]int{}
	var failures []map[string]string
	staged := int64(0)
	_, err = ingestprep.Array(reader, limits.RecordBytes, func(_ int, raw json.RawMessage) error {
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.UseNumber()
		var result ObserveResult
		if err := dec.Decode(&result); err != nil {
			return err
		}
		if result.Target == "" {
			return fmt.Errorf("observe result has empty target")
		}
		target := strings.TrimPrefix(result.Target, "//")
		p.snapshot.Targets = append(p.snapshot.Targets, target)
		if result.Error != "" {
			failures = append(failures, map[string]string{"target": target, "error": result.Error})
			return nil
		}
		if result.Current == nil {
			return nil
		}
		records, hasRecords := result.Current["records"]
		emit := func(record map[string]any) error {
			if err := in.Context.Err(); err != nil {
				return err
			}
			entry, recordJSON, err := prepareObserveRecord(record, target, in.Origin, dir, now, p.snapshot.RecordCount, in.SnapshotID, in.Graph, in.Inferrer, in.RunID, route)
			if err != nil {
				return err
			}
			// record is redacted in place by now; facts come from that.
			projected, projectedRes := projection.Prepare(p.projector, entry.Schema, entry.ID, *entry.ResourceID, entry.IdentityJSON != nil, record)
			p.tally.Note(p.projector, entry.Schema, projectedRes)
			results := p.policies.Prepare(entry.Schema, entry.ID, *entry.ResourceID, entry.IdentityJSON != nil, record)
			descriptor, err := json.Marshal(observeDescriptor{CatalogEntry: entry, Projection: projected, Policies: results})
			if err != nil {
				return err
			}
			staged += entry.SizeBytes + int64(len(descriptor)+1)
			if staged > limits.StagingBytes {
				return fmt.Errorf("observation exceeds staging byte limit")
			}
			if err := artifacts.WriteFilePrivate(entry.StoredPath, recordJSON); err != nil {
				return err
			}
			counts[entry.Schema]++
			p.snapshot.RecordCount++
			_, err = p.spool.Write(append(descriptor, '\n'))
			return err
		}
		if !hasRecords {
			return emit(result.Current)
		}
		array, ok := records.([]any)
		if !ok {
			return fmt.Errorf("target %s records is not an array", target)
		}
		for i, item := range array {
			record, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("target %s record %d is not an object", target, i)
			}
			if err := emit(record); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("prepare observe results: %w", err)
	}
	if len(failures) > 0 || (p.route.mismatches != nil && *p.route.mismatches > 0) {
		p.snapshot.Complete = false
	}
	p.entry, err = prepareObserveSnapshot(observeSnapshotEntry{snapshotID: in.SnapshotID, now: now, origin: in.Origin, targets: p.snapshot.Targets, recordCount: p.snapshot.RecordCount, schemaCounts: counts, errors: failures, rawDir: dir, maxBytes: limits.StagingBytes - staged, runID: in.RunID})
	if err != nil {
		return nil, err
	}
	if staged+p.entry.SizeBytes > limits.StagingBytes {
		return nil, fmt.Errorf("observation exceeds staging byte limit")
	}
	return p, nil
}

func (p *PreparedObservation) Commit(db *database.CatalogDB) (ObserveIngestResult, error) {
	// observed holds the distinct entries linked into the snapshot (what
	// `pudl snapshot show` counts); ingested counts those stored as new.
	observed := map[string]struct{}{}
	ingested := 0
	var policyChange policyfacts.Change
	err := artifacts.WithLock(p.in.Context, db.Root(), func() error {
		journal, err := artifacts.NewJournalContext(p.in.Context, p.dir)
		if err != nil {
			return err
		}
		defer journal.Close()
		if _, err := p.spool.Seek(0, io.SeekStart); err != nil {
			return err
		}
		err = db.WithCatalogTxContext(p.in.Context, func(tx *database.CatalogTx) error {
			if err := publishObserveEntry(journal, &p.entry, p.rawDir); err != nil {
				return err
			}
			if err := tx.AddEntry(p.entry); err != nil {
				return err
			}
			if err := tx.RecordObserveSnapshot(p.snapshot); err != nil {
				return err
			}
			dec := json.NewDecoder(p.spool)
			for {
				var descriptor observeDescriptor
				if err := dec.Decode(&descriptor); err == io.EOF {
					break
				} else if err != nil {
					return err
				}
				entry := descriptor.CatalogEntry
				observed[entry.ID] = struct{}{}
				// Dedup on the entry ID itself — the content hash — since that is
				// what the catalog keys on. A byte-identical record from another
				// target or model is the same entry: reuse it and record its
				// membership in this snapshot.
				existing, err := tx.GetEntry(entry.ID)
				if errors.GetErrorCode(err) == errors.ErrCodeNotFound {
					existing, err = nil, nil
				}
				if err != nil {
					return err
				}
				if existing != nil {
					if entry.IdentityJSON != nil && (existing.IdentityJSON == nil || *existing.IdentityJSON == "") {
						// Moving to the identified resource makes it that
						// resource's next version (see UpdateEntryIdentity).
						if err := tx.UpdateEntryIdentity(existing.ID, *entry.ResourceID, *entry.IdentityJSON); err != nil {
							return err
						}
					}
					if err := tx.AddCollectionMembership(p.in.SnapshotID, existing.ID, *entry.ItemIndex); err != nil {
						return err
					}
					// Re-observed unchanged: it is the resource's latest state again.
					if existing.Schema == entry.Schema {
						if err := observeFacts(tx, descriptor, existing.ID, &policyChange); err != nil {
							return err
						}
					}
					continue
				}
				// A new content hash for a known resource is its next version.
				// Read inside the transaction, so two records of one resource in
				// this observation take consecutive versions.
				latest, err := tx.GetLatestVersion(*entry.ResourceID)
				if err != nil {
					return err
				}
				version := latest + 1
				entry.Version = &version
				if err := publishObserveEntry(journal, &entry, p.rawDir); err != nil {
					return err
				}
				if err := tx.AddEntry(entry); err != nil {
					return err
				}
				if err := observeFacts(tx, descriptor, entry.ID, &policyChange); err != nil {
					return err
				}
				ingested++
			}
			if !p.snapshot.Complete {
				return nil
			}
			closed, err := policyfacts.CloseScopeLeavers(tx, p.snapshot.Scope, p.snapshot.SnapshotID)
			policyChange.Closed += closed
			return err
		})
		if err != nil {
			journal.Rollback()
		}
		return err
	})
	if err != nil {
		return ObserveIngestResult{}, err
	}
	result := ObserveIngestResult{Records: len(observed), NewRecords: ingested, SnapshotID: p.in.SnapshotID}
	if p.route.mismatches != nil {
		result.SchemaMismatches = *p.route.mismatches
	}
	if p.route.redacted != nil {
		result.Redacted = *p.route.redacted
	}
	if p.tally != nil {
		result.Facts = p.tally.Facts
		result.FactWarnings = p.tally.Warnings()
	}
	if p.route.secrets != nil {
		result.SecretWarnings, result.SecretWarningsTotal = p.route.secrets.Warnings, p.route.secrets.Total
	}
	result.PolicyChange = policyChange
	return result, nil
}

// observeFacts makes a stored record (entryID: the new entry, or the existing
// one it deduplicated to) its resource's latest state: its projected facts and
// policy results become current.
func observeFacts(tx *database.CatalogTx, d observeDescriptor, entryID string, change *policyfacts.Change) error {
	if d.Projection != nil {
		projected := *d.Projection
		projected.EntryID = entryID
		if err := projection.Observe(tx, &projected); err != nil {
			return err
		}
	}
	if d.Policies == nil {
		return nil
	}
	results := *d.Policies
	results.EntryID = entryID
	c, err := policyfacts.Observe(tx, &results, database.ProjectionObservation)
	change.Closed += c.Closed
	change.Updated += c.Updated
	return err
}

func publishObserveEntry(journal *artifacts.Journal, entry *database.CatalogEntry, dir string) error {
	destination := filepath.Join(dir, filepath.Base(entry.StoredPath))
	if err := journal.Publish(entry.StoredPath, destination); err != nil {
		return err
	}
	entry.StoredPath = destination
	return nil
}
