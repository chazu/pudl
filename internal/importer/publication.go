package importer

import (
	"encoding/json"
	"github.com/chazu/pudl/internal/inference"
	"os"
	"time"

	"github.com/chazu/pudl/internal/database"
)

func (e *EnhancedImporter) publishMetadata(metadata ImportMetadata, destination string, opts ImportOptions) error {
	if opts.publication == nil {
		return e.saveMetadata(metadata, destination)
	}
	dir, err := e.TempDir()
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "metadata-*")
	if err != nil {
		return err
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	defer os.Remove(path)
	if err := e.saveMetadata(metadata, path); err != nil {
		return err
	}
	// A process may have died after publishing metadata but before SQL commit.
	// Under the artifact lock only a proven-unreferenced orphan can be replaced.
	if _, err := os.Lstat(destination); err == nil {
		if _, err := e.catalogDB.RemoveCommittedOrphan(destination); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return opts.publication.Publish(path, destination)
}

func existingImportResult(entry *database.CatalogEntry, source string) *ImportResult {
	r := &ImportResult{ID: entry.ID, SourcePath: source, StoredPath: entry.StoredPath, MetadataPath: entry.MetadataPath, DetectedFormat: entry.Format, DetectedOrigin: entry.Origin, AssignedSchema: entry.Schema, SchemaConfidence: entry.Confidence, RecordCount: entry.RecordCount, SizeBytes: entry.SizeBytes, ImportTimestamp: entry.ImportTimestamp.Format(time.RFC3339), Skipped: true, SkipReason: "content already exists in catalog"}
	if entry.ContentHash != nil {
		r.ContentHash = *entry.ContentHash
	}
	if entry.ResourceID != nil {
		r.ResourceID = *entry.ResourceID
	}
	if entry.Version != nil {
		r.Version = *entry.Version
	}
	return r
}

func loadOriginalExplanation(path string) *inference.InferenceTrace {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var metadata ImportMetadata
	if json.Unmarshal(body, &metadata) != nil {
		return nil
	}
	return metadata.SchemaInfo.Explanation
}
