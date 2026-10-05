package bundle

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chazu/pudl/internal/database"
)

// Older manifest ingestion advertised a .meta file which it never created.
// Normalize only that recognized missing synthetic reference in the private
// backup catalog. Missing authored/imported metadata remains an error.
func normalizeLegacyMetadata(ctx context.Context, stage string, entries []database.CatalogEntry) ([]string, error) {
	var ids []string
	for i := range entries {
		entry := &entries[i]
		if entry.EntryType == nil || (*entry.EntryType != "manifest" && *entry.EntryType != "manifest-action") || entry.MetadataPath != entry.StoredPath+".meta" {
			continue
		}
		if _, err := os.Lstat(entry.MetadataPath); os.IsNotExist(err) {
			ids = append(ids, entry.ID)
			entry.MetadataPath = ""
		} else if err != nil {
			return nil, err
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	db, err := sql.Open("sqlite", filepath.Join(stage, filepath.FromSlash(catalogPath)))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	for _, id := range ids {
		if _, err := db.ExecContext(ctx, `UPDATE catalog_entries SET metadata_path='' WHERE id=?`, id); err != nil {
			return nil, err
		}
	}
	// The private snapshot can inherit WAL mode. Fold these repairs into the
	// main file before hashing/archiving it, even with a read-only handle open.
	var busy, logPages, checkpointed int
	if err := db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logPages, &checkpointed); err != nil {
		return nil, err
	}
	if busy != 0 {
		return nil, fmt.Errorf("private bundle catalog checkpoint is busy")
	}
	return []string{fmt.Sprintf("Normalized %d legacy manifest references to metadata files that were never created; source catalog unchanged", len(ids))}, nil
}
