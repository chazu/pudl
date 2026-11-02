package repair

import (
	"context"
	"fmt"
	"os"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/muschemas"
)

// schemaRefs upgrades item schema references that were unresolved at import
// time to declared once their module and version are in the schema cache
// (formerly `pudl reclassify`).
var schemaRefs = Repair{
	Name:      "schema-refs",
	Condition: "items tagged with a declared schema reference that was not in the schema cache at import",
	Diagnose: func(ctx context.Context, env *Env) (Result, error) {
		return reclassify(env, true)
	},
	Apply: func(ctx context.Context, env *Env, dryRun bool) (Result, error) {
		return reclassify(env, dryRun)
	},
}

func reclassify(env *Env, dryRun bool) (Result, error) {
	rows, err := env.DB.ListUnresolvedItemSchemas(env.Options.SchemaRef)
	if err != nil {
		return Result{}, fmt.Errorf("list unresolved: %w", err)
	}
	if len(rows) == 0 {
		return Result{Message: "No unresolved schema references"}, nil
	}
	// Diagnosis must not create the cache directory; a missing cache
	// resolves nothing.
	var cache *muschemas.Cache
	if _, statErr := os.Stat(env.SchemaCacheRoot); statErr == nil {
		if cache, err = muschemas.New(env.SchemaCacheRoot); err != nil {
			return Result{}, fmt.Errorf("open schema cache: %w", err)
		}
	}

	var result Result
	stillUnresolved := 0
	for _, r := range rows {
		resolved := false
		if cache != nil {
			resolved, err = refCached(cache, r.SchemaRef)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s -> %s: %v", r.ItemID, r.SchemaRef, err))
			}
		}
		if !resolved {
			stillUnresolved++
			continue
		}
		result.Pending++
		result.Changes = append(result.Changes, Change{Subject: r.ItemID, From: database.ItemSchemaStatusUnresolved + " " + r.SchemaRef, To: database.ItemSchemaStatusDeclared})
		if dryRun {
			continue
		}
		if err := env.DB.AddItemSchema(database.ItemSchema{
			ItemID:    r.ItemID,
			SchemaRef: r.SchemaRef,
			Status:    database.ItemSchemaStatusDeclared,
		}); err != nil {
			return result, fmt.Errorf("upgrade %s: %w", r.ItemID, err)
		}
		result.Changed++
	}

	result.Message = fmt.Sprintf("%d of %d unresolved references resolvable, %d still unresolved", result.Pending, len(rows), stillUnresolved)
	if !dryRun {
		result.Message = fmt.Sprintf("%d upgraded, %d still unresolved (of %d total)", result.Changed, stillUnresolved, len(rows))
	}
	if stillUnresolved > 0 {
		result.Message += fmt.Sprintf("; load the referenced schemas into the schema cache at %s", env.SchemaCacheRoot)
		// The condition remains until the schemas are cached.
		result.Status = StatusNeeded
	}
	return result, nil
}

// refCached parses ref and reports whether its (module, version) is cached.
// Definition selectors (e.g. "#EC2Instance") are accepted but not validated
// against the schema's declared types: presence of the module/version is
// sufficient to upgrade an unresolved row to declared.
func refCached(cache *muschemas.Cache, ref string) (bool, error) {
	module, version, _, err := muschemas.ParseRef(ref)
	if err != nil {
		return false, err
	}
	return cache.Has(module, version), nil
}
