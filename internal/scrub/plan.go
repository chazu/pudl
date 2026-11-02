// Package scrub redacts plaintext stored before a schema declared a field in
// `_pudl.sensitive_fields`: payloads, collection files, `.meta` files, legacy
// and snapshot files, and stored reports. Entries whose content changes get a
// re-derived ID, with every reference rewritten in one transaction, and their
// current projected facts are re-projected. Fact history is left unchanged.
// Collected values stay in memory; nothing the package returns contains one.
// See docs/design/2026-11-01-sensitive-data-scrub.md.
package scrub

import (
	"context"
	"os"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/fieldpath"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/projection"
	"github.com/chazu/pudl/internal/redact"
	"github.com/chazu/pudl/internal/schemaname"
)

// Scrubber diagnoses and applies the scrub over one catalog.
type Scrubber struct {
	DB          *database.CatalogDB
	Inferrer    *inference.SchemaInferrer
	Projections *projection.Registry
	Scope       Scope
}

// Scope selects the payloads checked by path. Values collected from them are
// searched for in every unstructured artifact regardless of scope.
type Scope struct{ EntryID, Schema, Origin string }

// Subject is one artifact the scrub reports: never its values.
type Subject struct {
	Kind   string   `json:"kind"`
	ID     string   `json:"id"`
	NewID  string   `json:"new_id,omitempty"`
	Schema string   `json:"schema,omitempty"`
	Paths  []string `json:"paths,omitempty"`
	Merged bool     `json:"merged,omitempty"`
}

// Plan is what the scrub would change. It holds values in memory and must not
// be serialized; Subjects is its printable form.
type Plan struct {
	rewrites    []*rewrite
	reports     []database.ReportBlob // Key and Kind only
	Unsupported []Subject             // non-JSON or compressed payloads holding plaintext
	Errors      []string
	Leftovers   []string // files an interrupted scrub superseded
	Uncompacted []string // scrubs whose compaction is pending
	values      *valueSet
}

// ShortValuesSkipped counts the distinct collected values too short to search.
func (p *Plan) ShortValuesSkipped() int { return p.values.skipped() }

// Pending counts what an apply would do.
func (p *Plan) Pending() int {
	n := len(p.rewrites) + len(p.reports) + len(p.Unsupported) + len(p.Leftovers)
	if len(p.Uncompacted) > 0 {
		n++
	}
	return n
}

// Subjects lists the rewrites and report rewrites, with old and new IDs.
func (p *Plan) Subjects() []Subject {
	var out []Subject
	for _, rw := range p.rewrites {
		subject := Subject{Kind: rw.kind, ID: rw.entry.ID, Schema: rw.entry.Schema, Paths: rw.paths, Merged: rw.mergeInto}
		if rw.newID != rw.entry.ID {
			subject.NewID = rw.newID
		}
		out = append(out, subject)
	}
	for _, r := range p.reports {
		out = append(out, Subject{Kind: r.Kind, ID: r.Key, Paths: []string{valueMatch}})
	}
	return out
}

// valueSearchSchemas are entries without a usable schema whose files quote
// payload values: legacy mu manifests and drift observations, and observe
// snapshot summaries.
var valueSearchSchemas = map[string]bool{}

func init() {
	for _, name := range []string{"pudl/mu.#DriftObservation", "pudl/mu.#Manifest", "pudl/mu.#ManifestAction", "pudl/mu.#PlanOutput", "pudl/core.#ObserveSnapshot"} {
		valueSearchSchemas[schemaname.Normalize(name)] = true
	}
}

// Plan diagnoses the catalog without writing.
func (s *Scrubber) Plan(ctx context.Context) (*Plan, error) {
	plan := &Plan{values: newValueSet()}
	if err := s.pending(plan); err != nil {
		return nil, err
	}
	reg := redact.NewRegistry(s.Inferrer)
	if !reg.Any() {
		return plan, nil
	}
	found, err := s.detect(ctx, reg, plan)
	if err != nil || len(found) == 0 {
		return plan, err
	}
	planned := map[string]bool{}
	for _, f := range found {
		kind := "document"
		if isItem(f.entry) {
			kind = "item"
		}
		rw, err := payloadRewrite(f.entry, kind, f.paths, f.redacted)
		if err != nil {
			plan.Errors = append(plan.Errors, idgen.HashToProquint(f.entry.ID)+": "+err.Error())
			continue
		}
		plan.rewrites, planned[f.entry.ID] = append(plan.rewrites, rw), true
	}
	if err := s.planParents(found, reg, plan, planned); err != nil {
		return nil, err
	}
	if err := s.searchValues(ctx, plan, planned); err != nil {
		return nil, err
	}
	targets := map[string]bool{}
	rewrites := plan.rewrites[:0]
	for _, rw := range plan.rewrites {
		s.finish(rw, plan.values)
		if rw.payload == nil && rw.meta == nil {
			continue // the pre-filter matched, but no string held a value
		}
		rewrites = append(rewrites, rw)
		if rw.newID != rw.entry.ID {
			exists, err := s.DB.EntryExists(rw.newID)
			if err != nil {
				return nil, err
			}
			rw.mergeInto = exists || targets[rw.newID]
			targets[rw.newID] = true
		}
	}
	plan.rewrites = rewrites
	return plan, nil
}

// planParents re-encodes each import collection holding a flagged item. Observe
// snapshots hold a summary, not the records; value search covers them.
func (s *Scrubber) planParents(found []*finding, reg *redact.Registry, plan *Plan, planned map[string]bool) error {
	byCollection := map[string]map[int][]fieldpath.Path{}
	for _, f := range found {
		if !isItem(f.entry) {
			continue
		}
		memberships, err := s.DB.ItemCollections(f.entry.ID)
		if err != nil {
			return err
		}
		meta := readMeta(f.entry.MetadataPath)
		paths, err := reg.For(metaString(meta, "schema_info", "intended_schema"), f.entry.Schema)
		if err != nil {
			continue // detectEntry already resolved these paths
		}
		for collectionID, index := range memberships {
			if byCollection[collectionID] == nil {
				byCollection[collectionID] = map[int][]fieldpath.Path{}
			}
			byCollection[collectionID][index] = paths
		}
	}
	for collectionID, redactions := range byCollection {
		parent, err := s.DB.GetEntry(collectionID)
		if err != nil || !isCollection(*parent) || valueSearchSchemas[parent.Schema] || planned[parent.ID] {
			continue
		}
		rw, err := collectionRewrite(*parent, redactions)
		if err != nil {
			plan.Errors = append(plan.Errors, idgen.HashToProquint(parent.ID)+": "+err.Error())
			continue
		}
		if rw != nil {
			plan.rewrites, planned[parent.ID] = append(plan.rewrites, rw), true
		}
	}
	return nil
}

// searchValues finds the collected values in every .meta file, in legacy and
// snapshot payloads, and in stored reports. It reads only when a value of
// searchable length was collected.
func (s *Scrubber) searchValues(ctx context.Context, plan *Plan, planned map[string]bool) error {
	if plan.values.empty() {
		return nil
	}
	err := s.DB.EachEntryContext(ctx, database.FilterOptions{}, func(entry database.CatalogEntry) error {
		if planned[entry.ID] {
			return nil
		}
		if valueSearchSchemas[entry.Schema] {
			if raw, err := os.ReadFile(entry.StoredPath); err == nil && plan.values.mentionedIn(raw) {
				if record, err := idgen.DecodeJSONExact(raw); err == nil {
					if replaced, n := plan.values.replace(record); n > 0 {
						rw, err := payloadRewrite(entry, "legacy", []string{valueMatch}, replaced)
						if err != nil {
							return err
						}
						plan.rewrites, planned[entry.ID] = append(plan.rewrites, rw), true
						return nil
					}
				}
			}
		}
		if raw, err := os.ReadFile(entry.MetadataPath); err == nil && plan.values.mentionedIn(raw) {
			plan.rewrites, planned[entry.ID] = append(plan.rewrites, &rewrite{entry: entry, kind: "metadata", paths: []string{valueMatch}}), true
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.DB.EachReportBlob(ctx, func(blob database.ReportBlob) error {
		if !plan.values.mentionedIn(blob.JSON) {
			return nil
		}
		if _, changed, err := plan.values.rewriteJSON(blob.JSON); err == nil && changed {
			plan.reports = append(plan.reports, database.ReportBlob{Kind: blob.Kind, Key: blob.Key})
		}
		return nil
	})
}

// pending records files an interrupted scrub superseded that still exist and
// no entry references, and scrubs whose compaction did not finish.
func (s *Scrubber) pending(plan *Plan) error {
	oldFiles, uncompacted, err := s.DB.PendingScrubs()
	if err != nil {
		return err
	}
	plan.Uncompacted = uncompacted
	seen := map[string]bool{}
	for _, path := range oldFiles {
		if seen[path] {
			continue
		}
		seen[path] = true
		if _, err := os.Lstat(path); err != nil {
			continue
		}
		referenced, err := s.DB.IsArtifactReferenced(path)
		if err != nil {
			return err
		}
		if !referenced {
			plan.Leftovers = append(plan.Leftovers, path)
		}
	}
	return nil
}
