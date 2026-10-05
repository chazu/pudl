package mubridge

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
	"github.com/chazu/pudl/internal/ingestprep"
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
	if err := os.MkdirAll(tmp, 0o755); err != nil {
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
	p.spool, err = os.Create(filepath.Join(dir, "entries.ndjson"))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	p.rawDir = filepath.Join(in.DataDir, "raw", now.Format("2006/01/02"))
	p.snapshot = database.ObserveSnapshot{SnapshotID: in.SnapshotID, RunID: in.RunID, Model: in.Model, Workspace: in.Workspace, Origin: in.Origin, Source: in.Source, CreatedAt: now}
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
			entry, recordJSON, err := prepareObserveRecord(record, target, in.Origin, dir, now, p.snapshot.RecordCount, in.SnapshotID, in.Graph, in.Inferrer, in.SchemaMappings, in.RunID)
			if err != nil {
				return err
			}
			descriptor, err := json.Marshal(entry)
			if err != nil {
				return err
			}
			staged += entry.SizeBytes + int64(len(descriptor)+1)
			if staged > limits.StagingBytes {
				return fmt.Errorf("observation exceeds staging byte limit")
			}
			if err := os.WriteFile(entry.StoredPath, recordJSON, 0o600); err != nil {
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
	ingested := 0
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
				var entry database.CatalogEntry
				if err := dec.Decode(&entry); err == io.EOF {
					break
				} else if err != nil {
					return err
				}
				existing, err := tx.GetLatestObserveByContentHash(*entry.Target, *entry.ContentHash)
				if err != nil {
					return err
				}
				if existing != nil {
					if entry.IdentityJSON != nil && (existing.IdentityJSON == nil || *existing.IdentityJSON == "") {
						if err := tx.UpdateEntryIdentity(existing.ID, *entry.ResourceID, *entry.IdentityJSON); err != nil {
							return err
						}
					}
					if err := tx.AddCollectionMembership(p.in.SnapshotID, existing.ID, *entry.ItemIndex); err != nil {
						return err
					}
					continue
				}
				if err := publishObserveEntry(journal, &entry, p.rawDir); err != nil {
					return err
				}
				if err := tx.AddEntry(entry); err != nil {
					return err
				}
				ingested++
			}
			return nil
		})
		if err != nil {
			journal.Rollback()
		}
		return err
	})
	if err != nil {
		return ObserveIngestResult{}, err
	}
	return ObserveIngestResult{Records: ingested, SnapshotID: p.in.SnapshotID}, nil
}

func publishObserveEntry(journal *artifacts.Journal, entry *database.CatalogEntry, dir string) error {
	destination := filepath.Join(dir, filepath.Base(entry.StoredPath))
	if err := journal.Publish(entry.StoredPath, destination); err != nil {
		return err
	}
	entry.StoredPath = destination
	return nil
}
