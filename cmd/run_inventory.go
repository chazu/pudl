package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/inference"
)

// identityResolver returns the declared identity_fields for a schema, or nil when
// the schema is unknown or declares none. In the run path it is backed by the
// inference graph (see schemaIdentityResolver); tests inject a stub.
type identityResolver func(schema string) []string

// recordIdentity derives a stable match key for a record: its _schema plus the
// values of the schema's declared identity_fields (resolved from the inference
// graph). When the schema declares no identity_fields — or a declared field is
// absent from the record — it falls back to the first present of name | path | id,
// which covers the linux/fs/k8s desired shapes.
func recordIdentity(rec map[string]any, identity identityResolver) (key string, label string, ok bool) {
	schema, _ := rec["_schema"].(string)

	if identity != nil {
		if fields := identity(schema); len(fields) > 0 {
			vals := make([]string, 0, len(fields))
			complete := true
			for _, f := range fields {
				v, present := rec[f]
				if !present {
					complete = false
					break
				}
				vals = append(vals, fmt.Sprintf("%v", v))
			}
			if complete {
				joined := strings.Join(vals, "/")
				return fmt.Sprintf("%s|%s", schema, joined), fmt.Sprintf("%s/%s", shortSchema(schema), joined), true
			}
		}
	}

	// Fallback: schema declares no identity_fields, or one is missing from the record.
	for _, k := range []string{"name", "path", "id"} {
		if v, present := rec[k]; present {
			return fmt.Sprintf("%s|%v", schema, v), fmt.Sprintf("%s/%v", shortSchema(schema), v), true
		}
	}
	return "", "", false
}

// modelResourceDefs returns the candidate catalog definition names for a model's
// desired resources — the bare resource names that `ingest-manifest` keys
// per-resource status on (targetToDefinition(action.Target)). Each desired record
// contributes the values of its declared identity_fields, else the first present
// of name | path | id. The set is a superset used only to scope a converging->clean
// promotion, so extra candidates that match nothing are harmless.
func modelResourceDefs(desired []map[string]any, identity identityResolver) []string {
	seen := map[string]bool{}
	var defs []string
	add := func(v any) {
		s := fmt.Sprintf("%v", v)
		if s != "" && !seen[s] {
			seen[s] = true
			defs = append(defs, s)
		}
	}
	for _, d := range desired {
		schema, _ := d["_schema"].(string)
		matched := false
		if identity != nil {
			for _, f := range identity(schema) {
				if v, ok := d[f]; ok {
					add(v)
					matched = true
				}
			}
		}
		if !matched {
			for _, k := range []string{"name", "path", "id"} {
				if v, ok := d[k]; ok {
					add(v)
					break
				}
			}
		}
	}
	return defs
}

// schemaIdentityResolver builds an identityResolver backed by the schema inferrer:
// each schema's declared identity_fields from the inference graph. Records whose
// schema is unknown or declares none fall back to the name|path|id heuristic.
func schemaIdentityResolver() (identityResolver, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	inferrer, err := inference.Shared(effectiveSchemaPaths(cfg)...)
	if err != nil {
		return nil, fmt.Errorf("init schema inferrer: %w", err)
	}
	return func(schema string) []string {
		if meta, ok := inferrer.GetSchemaMetadata(schema); ok {
			return meta.IdentityFields
		}
		return nil
	}, nil
}

func shortSchema(s string) string {
	if s == "" {
		return "?"
	}
	return s
}

// fieldsDiffer returns a description of the first desired field not satisfied by
// the observed record (missing or unequal), or "" if every desired field matches.
// Observed may carry extra fields — ensure-present semantics, not equality.
func fieldsDiffer(desired, observed map[string]any) string {
	for k, dv := range desired {
		if k == "_schema" {
			continue
		}
		ov, ok := observed[k]
		if !ok {
			return fmt.Sprintf("%s missing (want %v)", k, dv)
		}
		if fmt.Sprint(ov) != fmt.Sprint(dv) {
			return fmt.Sprintf("%s: %v → want %v", k, ov, dv)
		}
	}
	return ""
}

// inventorySetDiff compares desired records against observed (inventory) records
// by identity, ensure-present semantics: a desired record with no match is
// "missing"; a match whose fields differ is "changed". Extra observed records are
// ignored (prune is deferred, matching host-converge V1).
func inventorySetDiff(desired, observed []map[string]any, identity identityResolver) []ResourceDrift {
	obs := make(map[string]map[string]any, len(observed))
	for _, o := range observed {
		if k, _, ok := recordIdentity(o, identity); ok {
			obs[k] = o
		}
	}
	var drifted []ResourceDrift
	for _, d := range desired {
		k, label, ok := recordIdentity(d, identity)
		if !ok {
			continue // un-keyable desired record; skip (can't match)
		}
		o, found := obs[k]
		if !found {
			drifted = append(drifted, ResourceDrift{Resource: label, Reason: "missing"})
			continue
		}
		if diff := fieldsDiffer(d, o); diff != "" {
			drifted = append(drifted, ResourceDrift{Resource: label, Reason: "changed", Diff: diff})
		}
	}
	return drifted
}

// observeScopeFilter turns a scope string and the result of looking it up as a
// snapshot collection into the filter the observe query should use: either a
// collection ID (the scope names a snapshot) or an origin (the compatibility
// path for explicit catalog callers and tests).
//
// Only a *not-found* may fall back to the origin filter. Any other lookup
// failure is fatal, because falling back on a genuine database error filters by
// an origin that matches nothing, hands the set-diff an empty observed set, and
// reports every desired resource `missing` — a transient DB fault rendered as a
// confident "everything is drifted", which under --converge means re-applying
// the entire model.
func observeScopeFilter(scope string, lookupErr error) (collectionID, origin string, err error) {
	switch {
	case lookupErr == nil:
		return scope, "", nil
	case errors.GetErrorCode(lookupErr) == errors.ErrCodeNotFound:
		return "", scope, nil
	default:
		return "", "", fmt.Errorf("resolve observe scope %q: %w", scope, lookupErr)
	}
}

// loadObservedRecords reads the inventory records ingested for this run from the
// catalog (observe items by origin) and returns them as maps.
func loadObservedRecords(db *database.CatalogDB, scope string) ([]map[string]any, error) {
	filter := database.FilterOptions{EntryTypes: []string{"observe"}, CollectionType: "item"}
	// A snapshot ID is the normal scope for a live inventory run. Keep origin
	// filtering as a compatibility path for explicit catalog callers and tests.
	//
	// Only a *not-found* justifies that fallback — see observeScopeFilter.
	if scope != "" {
		_, lookupErr := db.GetCollectionByID(scope)
		collectionID, origin, err := observeScopeFilter(scope, lookupErr)
		if err != nil {
			return nil, err
		}
		filter.CollectionID, filter.Origin = collectionID, origin
	}
	res, err := db.QueryEntries(database.FilterOptions{
		EntryTypes:     filter.EntryTypes,
		CollectionType: filter.CollectionType,
		Origin:         filter.Origin,
		CollectionID:   filter.CollectionID,
	}, database.QueryOptions{})
	if err != nil {
		return nil, fmt.Errorf("query observed records: %w", err)
	}
	var records []map[string]any
	for _, e := range res.Entries {
		data, err := os.ReadFile(e.StoredPath)
		if err != nil {
			return nil, fmt.Errorf("read observed record %s: %w", e.StoredPath, err)
		}
		var rec map[string]any
		if err := json.Unmarshal(data, &rec); err != nil {
			return nil, fmt.Errorf("parse observed record %s: %w", e.StoredPath, err)
		}
		records = append(records, rec)
	}
	return records, nil
}

// runInventoryDrift computes drift for an inventory model: desired vs the
// observed records in the catalog (set-diff by identity). For inventory
// observers (host) that dump records — distinct from the differential path
// (k8s), where the plugin does the diff.
//
// The scope is required. Without one the query returns every observation in the
// catalog — every model, every host, all time — so a desired record could be
// satisfied by an unrelated model's observation and the run could report clean
// against records it has nothing to do with. Callers must pass either the
// snapshot they just populated or an explicitly requested replay scope.
//
// Verified is deliberately left false here: this function cannot tell whether
// its scope names a fresh snapshot or a stale replay, so the caller that chose
// the scope is the one that gets to claim verification.
func runInventoryDrift(db *database.CatalogDB, scope string, desired []map[string]any, identity identityResolver) (ModelDriftResult, error) {
	if strings.TrimSpace(scope) == "" {
		return ModelDriftResult{}, fmt.Errorf("inventory drift requires a catalog scope (snapshot ID or origin); refusing to compare against every observation in the catalog")
	}
	observed, err := loadObservedRecords(db, scope)
	if err != nil {
		return ModelDriftResult{}, err
	}
	drifted := inventorySetDiff(desired, observed, identity)
	return ModelDriftResult{Clean: len(drifted) == 0, Drifted: drifted}, nil
}
