package repair

import (
	"context"
	"fmt"

	"github.com/chazu/pudl/internal/database"
)

// schemaNames rewrites catalog schema names to canonical
// `<package>.#<Definition>` form (formerly `pudl schema migrate`). Inputs such
// as `pudl.schemas/aws/ec2@v0:#Instance`, `aws/ec2:#Instance` or `core.#Item`
// normalize via schemaname.Normalize. Idempotent.
var schemaNames = Repair{
	Name:      "schema-names",
	Condition: "catalog entries whose schema names are not in canonical <package>.#<Definition> form",
	Diagnose: func(ctx context.Context, env *Env) (Result, error) {
		pending, err := env.DB.PendingSchemaNameMigrations()
		if err != nil {
			return Result{}, err
		}
		return schemaNameResult(pending, 0), nil
	},
	Apply: func(ctx context.Context, env *Env, dryRun bool) (Result, error) {
		if dryRun {
			pending, err := env.DB.PendingSchemaNameMigrations()
			if err != nil {
				return Result{}, err
			}
			return schemaNameResult(pending, 0), nil
		}
		changed, err := env.DB.MigrateSchemaNames()
		if err != nil {
			return Result{}, err
		}
		return schemaNameResult(changed, len(changed)), nil
	},
}

func schemaNameResult(changes []database.SchemaNameChange, written int) Result {
	result := Result{Pending: len(changes), Changed: written}
	for _, c := range changes {
		result.Changes = append(result.Changes, Change{Subject: c.ID, From: c.From, To: c.To})
	}
	switch {
	case len(changes) == 0:
		result.Message = "All schema names are in canonical format"
	case written > 0:
		result.Message = fmt.Sprintf("Normalized %d schema names", written)
	default:
		result.Message = fmt.Sprintf("%d schema names are not in canonical format", len(changes))
	}
	return result
}
