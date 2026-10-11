package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/fieldpath"
	"github.com/chazu/pudl/internal/lister"
)

var showField string
var showHistory bool

func showEntryField(entry *lister.ListEntry, path string) error {
	field, err := fieldpath.Parse(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(entry.StoredPath)
	if err != nil {
		return err
	}
	if info.Size() > 64<<20 {
		return fmt.Errorf("payload exceeds 64 MiB inspection budget")
	}
	data, err := loadReinferData(entry.StoredPath, entry.Format)
	if err != nil {
		return err
	}
	if field.HasWildcard() {
		return printJSON(field.LookupAll(data))
	}
	value, ok := field.Lookup(data)
	if !ok {
		return fmt.Errorf("field %q is absent in entry %s", path, entry.ID)
	}
	return printJSON(value)
}

func showEntryHistory(ctx context.Context, entry *lister.ListEntry) error {
	if entry.ResourceID == nil || entry.IdentityJSON == nil {
		return fmt.Errorf("entry has no resolved resource identity; declare identity_fields before collecting resource history")
	}
	db, err := database.OpenCatalogDBReadOnly(effectivePudlDir())
	if err != nil {
		return err
	}
	defer db.Close()
	versions, err := db.FindByResourceID(*entry.ResourceID)
	if err != nil {
		return err
	}
	observations, err := db.ResourceObservations(ctx, *entry.ResourceID)
	if err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(map[string]any{"resource_id": *entry.ResourceID, "versions": versions, "observations": observations})
	}
	fmt.Fprintf(outw(), "Resource %s: %d stored versions, %d snapshot observations\n", *entry.ResourceID, len(versions), len(observations))
	for _, v := range versions {
		fmt.Fprintf(outw(), "  %s %s (%s)\n", v.ID, v.ImportTimestamp.Format("2006-01-02T15:04:05Z07:00"), v.Schema)
	}
	for _, o := range observations {
		fmt.Fprintf(outw(), "  snapshot %s: %s at %s (scope %s, complete %t)\n", o.SnapshotID, o.EntryID, o.ObservedAt.Format("2006-01-02T15:04:05Z07:00"), o.Scope, o.Complete)
	}
	return nil
}
