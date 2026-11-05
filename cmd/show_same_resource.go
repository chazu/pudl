package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/lister"
)

// sameResourceGroup is `pudl show`'s view of the entry's shared identity:
// the other cataloged records that describe the same resource.
type sameResourceGroup struct {
	GroupID string               `json:"group_id"`
	Kinds   []string             `json:"kinds"`
	Others  []sameResourceMember `json:"others"`
}

// sameResourceMember is another record of the same resource: its resource,
// current entry and schema.
type sameResourceMember struct {
	ResourceID string `json:"resource_id"`
	EntryID    string `json:"entry_id"`
	Proquint   string `json:"proquint"`
	Schema     string `json:"schema"`
}

// entrySameResource returns the entry's group, or nil when no other record
// describes its resource. A failure is a warning: the rest of `pudl show`
// still prints.
func entrySameResource(ctx context.Context, entry lister.ListEntry) *sameResourceGroup {
	if entry.ResourceID == nil || *entry.ResourceID == "" {
		return nil
	}
	group, err := loadEntrySameResource(ctx, *entry.ResourceID)
	if err != nil {
		fmt.Fprintf(errw(), "⚠️  same resource unavailable: %v\n", err)
		return nil
	}
	return group
}

func loadEntrySameResource(ctx context.Context, resourceID string) (*sameResourceGroup, error) {
	configDir := effectivePudlDir()
	db, err := database.OpenCatalogDBReadOnly(configDir)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rules, err := datalog.LoadRulesFromPaths(queryRulePaths(configDir)...)
	if err != nil {
		return nil, fmt.Errorf("load rules: %w", err)
	}
	wg, err := loadWorkspaceGroups(ctx, db, rules)
	if err != nil {
		return nil, err
	}
	g, ok := wg.Index.GroupOf(resourceID)
	if !ok {
		return nil, nil
	}
	out := &sameResourceGroup{GroupID: g.ID, Kinds: g.Kinds, Others: []sameResourceMember{}}
	for _, member := range g.Members {
		if member == resourceID {
			continue
		}
		e := wg.EntryOf[member]
		out.Others = append(out.Others, sameResourceMember{
			ResourceID: member, EntryID: e.ID, Proquint: idgen.HashToProquint(e.ID), Schema: e.Schema,
		})
	}
	return out, nil
}

func printSameResource(g *sameResourceGroup) {
	if g == nil {
		return
	}
	fmt.Fprintf(outw(), "Same resource as (%s):\n", strings.Join(g.Kinds, ", "))
	for _, m := range g.Others {
		fmt.Fprintf(outw(), "  %s  %s\n", m.Proquint, m.Schema)
	}
}
