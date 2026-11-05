package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/doctor"
	"github.com/chazu/pudl/internal/sameresource"
)

// workspaceSameResource gathers the shared identity check's input: declared
// kinds, kinds that schemas and rules name, and the catalog's grouping.
func workspaceSameResource(ctx context.Context, configDir string) doctor.SameResourceInput {
	in := doctor.SameResourceInput{DeclaredKinds: map[string]bool{}, UsedKinds: map[string][]string{}}
	rules, err := datalog.LoadRulesFromPaths(queryRulePaths(configDir)...)
	if err != nil {
		in.Err = fmt.Errorf("load rules: %w", err)
		return in
	}
	reg, err := projectionRegistry()
	if err != nil {
		in.Err = err
		return in
	}
	src := reg.Source()
	for _, schema := range src.GetAvailableSchemas() {
		m, ok := src.GetSchemaMetadata(schema)
		if !ok || m.SameResource == nil {
			continue
		}
		for _, d := range m.SameResource.Decls {
			addUse(in.UsedKinds, d.Kind, "schema "+schema)
		}
	}
	for _, r := range rules {
		if r.Head.Rel != sameresource.LinkRelation {
			continue
		}
		if t, ok := r.Head.Args[sameresource.ArgKind]; ok && !t.IsVariable() {
			if kind, ok := t.Value.(string); ok {
				addUse(in.UsedKinds, kind, "rule "+r.Describe())
			}
		}
	}

	// Diagnostics must not create a catalog.
	if _, err := os.Stat(filepath.Join(configDir, "data", "sqlite", "catalog.db")); os.IsNotExist(err) {
		in.DeclaredKinds, in.Err = declaredKinds(nil, rules)
		return in
	}
	db, err := database.NewCatalogDB(configDir)
	if err != nil {
		in.Err = fmt.Errorf("open catalog: %w", err)
		return in
	}
	defer db.Close()
	if in.DeclaredKinds, err = declaredKinds(db, rules); err != nil {
		in.Err = err
		return in
	}
	wg, err := loadWorkspaceGroups(ctx, db, rules)
	if err != nil {
		in.Err = err
		return in
	}
	in.Groups = wg.Index
	return in
}

// declaredKinds reads the kinds same_resource_kind declares. Kinds are
// ground facts in rule files, so with no catalog they are read from the
// rules themselves.
func declaredKinds(db *database.CatalogDB, rules []datalog.Rule) (map[string]bool, error) {
	out := map[string]bool{}
	if db == nil {
		for _, r := range rules {
			if r.Head.Rel != sameresource.KindRelation || len(r.Body) > 0 {
				continue
			}
			if kind, ok := r.Head.Args[sameresource.ArgKind].Value.(string); ok {
				out[kind] = true
			}
		}
		return out, nil
	}
	tuples, err := datalog.Evaluate(db, rules, sameresource.KindRelation, nil, datalog.TemporalScope{})
	if err != nil {
		return nil, fmt.Errorf("evaluate %s: %w", sameresource.KindRelation, err)
	}
	for _, t := range tuples {
		if kind, ok := t.Args[sameresource.ArgKind].(string); ok {
			out[kind] = true
		}
	}
	return out, nil
}

func addUse(uses map[string][]string, kind, where string) {
	for _, w := range uses[kind] {
		if w == where {
			return
		}
	}
	uses[kind] = append(uses[kind], where)
}
