package cmd

import (
	"context"
	"fmt"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/resourcegroup"
)

// workspaceGroups is the workspace's shared-identity grouping, with each
// grouped resource's current entry.
type workspaceGroups struct {
	Index   *resourcegroup.Index
	EntryOf map[string]database.CatalogEntry // by resource_id
}

// loadWorkspaceGroups evaluates same_resource over db's current entries with
// the workspace's rules and schemas.
func loadWorkspaceGroups(ctx context.Context, db *database.CatalogDB, rules []datalog.Rule) (*workspaceGroups, error) {
	reg, err := projectionRegistry()
	if err != nil {
		return nil, err
	}
	entries, err := db.CurrentResourceEntries()
	if err != nil {
		return nil, fmt.Errorf("read current entries: %w", err)
	}
	wg := &workspaceGroups{EntryOf: map[string]database.CatalogEntry{}}
	schemaOf := map[string]string{}
	for _, e := range entries {
		if e.ResourceID == nil || *e.ResourceID == "" {
			continue
		}
		wg.EntryOf[*e.ResourceID] = e
		schemaOf[*e.ResourceID] = e.Schema
	}
	wg.Index, err = resourcegroup.Load(ctx, resourcegroup.Input{
		DB: db, Rules: rules, SchemaOf: schemaOf,
		Metadata: resourcegroup.MetadataFunc(reg.Source().GetSchemaMetadata),
	})
	if err != nil {
		return nil, err
	}
	return wg, nil
}
