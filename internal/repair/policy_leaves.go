package repair

import (
	"context"
	"fmt"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/validator"
)

// policyLeaves moves entries assigned to a policy schema onto the nearest type
// in the policy's base_schema chain. Policies are evaluated against resources,
// never assigned to them; entries classified before that rule are moved here.
//
// Only the schema changes. resource_id is namespaced by the inheritance-family
// root, which a policy shares with its type, so identity, version history and
// facts keyed on the resource stay as they are.
var policyLeaves = Repair{
	Name:      "policy-leaves",
	Condition: "entries assigned to a policy schema instead of the type it specializes",
	Diagnose: func(ctx context.Context, env *Env) (Result, error) {
		return movePolicyLeaves(env, true)
	},
	Apply: func(ctx context.Context, env *Env, dryRun bool) (Result, error) {
		return movePolicyLeaves(env, dryRun)
	},
}

func movePolicyLeaves(env *Env, dryRun bool) (Result, error) {
	all, err := env.DB.QueryEntries(database.FilterOptions{}, database.QueryOptions{Limit: 0})
	if err != nil {
		return Result{}, fmt.Errorf("query catalog: %w", err)
	}
	lookup := env.Inferrer.GetSchemaMetadata

	var result Result
	for _, entry := range all.Entries {
		if meta, ok := lookup(entry.Schema); !ok || !meta.IsPolicy() {
			continue
		}
		proquint := idgen.HashToProquint(entry.ID)
		result.Pending++
		target := validator.NearestType(entry.Schema, lookup)
		if target == "" {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: policy %s reaches no type through base_schema", proquint, entry.Schema))
			continue
		}
		result.Changes = append(result.Changes, Change{Subject: proquint, From: entry.Schema, To: target})
		if dryRun {
			continue
		}
		entry.Schema = target
		if err := env.DB.UpdateEntry(entry); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to update: %v", proquint, err))
			continue
		}
		result.Changed++
	}
	result.Message = fmt.Sprintf("%d entries assigned to a policy schema", result.Pending)
	return result, nil
}
