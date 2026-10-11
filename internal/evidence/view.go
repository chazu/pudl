// Package evidence constructs isolated query views from explicitly selected
// snapshots. It never changes the source catalog or its latest-known facts.
package evidence

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/projection"
	"github.com/chazu/pudl/internal/redact"
	"github.com/chazu/pudl/internal/schemaname"
)

type Reference struct {
	SnapshotID string    `json:"snapshot_id"`
	Scope      string    `json:"scope"`
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observed_at"`
	Age        string    `json:"age"`
	Complete   bool      `json:"complete"`
}

type Request struct {
	Selectors []string
	Current   string
	MaxAge    time.Duration
	Now       time.Time
}

type View struct {
	DB          *database.CatalogDB
	References  []Reference
	Schemas     map[string]bool
	Diagnostics []projection.Diagnostic
	dir         string
}

func (v *View) Close() {
	if v.DB != nil {
		_ = v.DB.Close()
		v.DB = nil
	}
	if v.dir != "" {
		_ = os.RemoveAll(v.dir)
		v.dir = ""
	}
}

// Open materializes only selected payloads and their current schema projections
// into a private disk-backed query catalog. Generic latest-known facts are not
// silently mixed into a snapshot view. Multiple differing versions of the same
// resource are ambiguous; callers must choose their evidence explicitly.
func Open(ctx context.Context, source *database.CatalogDB, reg *projection.Registry, request Request) (_ *View, resultErr error) {
	v := &View{Schemas: map[string]bool{}}
	defer func() {
		if resultErr != nil {
			v.Close()
		}
	}()
	if request.Now.IsZero() {
		request.Now = time.Now()
	}
	if request.MaxAge < 0 {
		return nil, fmt.Errorf("evidence max age must not be negative")
	}
	seen := map[string]bool{}
	var snapshots []*database.ObserveSnapshot
	for _, selector := range request.Selectors {
		var snapshot *database.ObserveSnapshot
		var err error
		switch {
		case selector == "current":
			snapshot, err = source.GetObserveSnapshot(request.Current)
		case strings.HasPrefix(selector, "model:"):
			snapshot, err = source.CurrentObserveSnapshot(strings.TrimPrefix(selector, "model:"))
		case strings.HasPrefix(selector, "scope:"):
			snapshot, err = source.CurrentScopeSnapshot(strings.TrimPrefix(selector, "scope:"))
		default:
			snapshot, err = source.GetObserveSnapshot(selector)
		}
		if err != nil {
			return nil, err
		}
		if snapshot == nil {
			v.issue("missing_snapshot", fmt.Sprintf("no observation for %q", selector), "")
			continue
		}
		if seen[snapshot.SnapshotID] {
			continue
		}
		seen[snapshot.SnapshotID] = true
		age := request.Now.Sub(snapshot.CreatedAt)
		v.References = append(v.References, Reference{SnapshotID: snapshot.SnapshotID, Scope: snapshot.Scope, Source: snapshot.Source, ObservedAt: snapshot.CreatedAt, Age: age.String(), Complete: snapshot.Complete})
		if !snapshot.Complete || snapshot.Scope == "" {
			v.issue("incomplete_observation", "snapshot "+snapshot.SnapshotID+" does not establish a complete population", "")
		}
		if age < 0 || (request.MaxAge > 0 && age > request.MaxAge) {
			v.issue("stale_observation", "snapshot "+snapshot.SnapshotID+" is outside the requested freshness policy", "")
		}
		for _, schema := range snapshot.Schemas {
			v.Schemas[schemaname.Normalize(schema)] = true
		}
		snapshots = append(snapshots, snapshot)
	}
	if len(v.Diagnostics) > 0 {
		return v, nil
	}
	var err error
	v.dir, err = os.MkdirTemp("", "pudl-evidence-")
	if err != nil {
		return nil, err
	}
	v.DB, err = database.NewCatalogDB(v.dir)
	if err != nil {
		return nil, err
	}
	resources := map[string]string{}
	entries := map[string]bool{}
	budget := int64(256 << 20)
	err = v.DB.WithCatalogTxContext(ctx, func(tx *database.CatalogTx) error {
		for _, snapshot := range snapshots {
			rows, err := source.SnapshotRecordEntries(snapshot.SnapshotID)
			if err != nil {
				return err
			}
			if len(rows) != snapshot.RecordCount {
				v.issue("incomplete_payloads", "snapshot membership differs from its recorded count", snapshot.SnapshotID)
			}
			for _, entry := range rows {
				if err := ctx.Err(); err != nil {
					return err
				}
				v.Schemas[schemaname.Normalize(entry.Schema)] = true
				if entries[entry.ID] {
					continue
				}
				entries[entry.ID] = true
				if entry.ResourceID != nil {
					if old, ok := resources[*entry.ResourceID]; ok && old != entry.ID {
						v.issue("ambiguous_resource", "selected observations disagree about a resource", entry.ID)
					}
					resources[*entry.ResourceID] = entry.ID
				}
				if err := tx.AddEntry(entry); err != nil {
					return err
				}
				spec := reg.For(entry.Schema)
				if spec == nil {
					continue
				}
				if spec.Err != nil {
					v.issue("invalid_projection", spec.Err.Error(), entry.ID)
					continue
				}
				if entry.IdentityJSON == nil || entry.ResourceID == nil {
					v.issue("identity_unresolved", "record has no resolved identity", entry.ID)
					continue
				}
				data, err := readPayload(entry.StoredPath, &budget)
				if err != nil {
					v.issue("payload_unavailable", err.Error(), entry.ID)
					continue
				}
				redact.Apply(data, spec.Sensitive)
				projected := projection.Compute(spec, entry.ID, *entry.ResourceID, data)
				for _, message := range projected.Omitted {
					v.issue("projection_omitted", message, entry.ID)
				}
				for _, fact := range projected.Facts {
					fact.Source = "snapshot:" + snapshot.SnapshotID
					fact.ValidStart = snapshot.CreatedAt.Unix()
					if _, err := tx.AddFact(fact); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (v *View) issue(code, message, entry string) {
	v.Diagnostics = append(v.Diagnostics, projection.Diagnostic{Code: code, Message: message, EntryID: entry})
}

func readPayload(path string, budget *int64) (any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, *budget+1))
	if err != nil {
		return nil, err
	}
	*budget -= int64(len(b))
	if *budget < 0 {
		return nil, fmt.Errorf("selected evidence exceeds 256 MiB payload budget")
	}
	return idgen.DecodeJSONExact(b)
}
