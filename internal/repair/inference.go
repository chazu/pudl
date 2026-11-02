package repair

import (
	"context"
	"fmt"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/inference"
)

// inferenceDrift re-runs schema inference and reassigns entries whose inferred
// schema changed, recomputing their identity under the new schema (formerly
// `pudl schema reinfer`). By default it covers only the drift doctor's catalog
// scan found, and skips any explicit assignment (importer.ExplicitAssignment:
// producer entries, declared _schema, imports with --schema) whatever the scan
// reported; Options.IncludeExplicit re-infers every entry in scope.
var inferenceDrift = Repair{
	Name:      "inference",
	Condition: "inferred catalog entries whose schema inference now selects a different schema",
	Diagnose: func(ctx context.Context, env *Env) (Result, error) {
		return reinfer(env, true)
	},
	Apply: func(ctx context.Context, env *Env, dryRun bool) (Result, error) {
		return reinfer(env, dryRun)
	},
}

func reinfer(env *Env, dryRun bool) (Result, error) {
	opts := env.Options
	if !opts.IncludeExplicit && !opts.DriftScanned {
		return Result{Status: StatusSkipped, Message: "catalog entries were not scanned for inference drift"}, nil
	}
	entries, err := reinferCandidates(env.DB, opts)
	if err != nil {
		return Result{}, err
	}

	var result Result
	checked := 0
	for _, entry := range entries {
		proquint := idgen.HashToProquint(entry.ID)
		data, err := importer.DecodeStoredFile(entry.StoredPath, entry.Format)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to load data: %v", proquint, err))
			continue
		}
		if !opts.IncludeExplicit && importer.ExplicitAssignment(entry, data) {
			continue
		}
		checked++
		collectionType := ""
		if entry.CollectionType != nil {
			collectionType = *entry.CollectionType
		}
		inferred, err := env.Inferrer.Infer(data, inference.InferenceHints{
			Origin:         entry.Origin,
			Format:         entry.Format,
			CollectionType: collectionType,
		})
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: inference failed: %v", proquint, err))
			continue
		}
		if inferred.Schema == entry.Schema {
			continue
		}
		result.Pending++
		result.Changes = append(result.Changes, Change{Subject: proquint, From: entry.Schema, To: fmt.Sprintf("%s (%.2f)", inferred.Schema, inferred.Confidence)})
		if dryRun {
			continue
		}
		entry.Schema = inferred.Schema
		entry.Confidence = inferred.Confidence
		recomputeIdentity(&entry, data, env.Inferrer)
		if err := env.DB.UpdateEntry(entry); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to update: %v", proquint, err))
			continue
		}
		result.Changed++
	}
	result.Message = fmt.Sprintf("%d of %d entries checked infer a different schema", result.Pending, checked)
	return result, nil
}

// reinferCandidates selects the entries to re-infer: the scanned drift, or with
// IncludeExplicit the entry, schema and origin scope.
func reinferCandidates(db *database.CatalogDB, opts Options) ([]database.CatalogEntry, error) {
	var ids []string
	switch {
	case !opts.IncludeExplicit:
		ids = opts.DriftEntryIDs
	case opts.EntryID != "":
		entry, err := db.GetEntryByProquint(opts.EntryID)
		if err != nil {
			if entry, err = db.GetEntry(opts.EntryID); err != nil {
				return nil, fmt.Errorf("entry not found: %s", opts.EntryID)
			}
		}
		return []database.CatalogEntry{*entry}, nil
	default:
		result, err := db.QueryEntries(database.FilterOptions{Schema: opts.Schema, Origin: opts.Origin},
			database.QueryOptions{Limit: 0, SortBy: "import_timestamp", Reverse: true})
		if err != nil {
			return nil, fmt.Errorf("query catalog entries: %w", err)
		}
		return result.Entries, nil
	}
	entries := make([]database.CatalogEntry, 0, len(ids))
	for _, id := range ids {
		entry, err := db.GetEntry(id)
		if err != nil {
			return nil, fmt.Errorf("get entry %s: %w", id, err)
		}
		entries = append(entries, *entry)
	}
	return entries, nil
}
