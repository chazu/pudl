package importer

import (
	"fmt"
	"time"

	"pudl/internal/database"
)

// updateCatalog updates the main data catalog with the new import
func (i *Importer) updateCatalog(metadata ImportMetadata, storedPath, metadataPath string) error {
	// Parse timestamp
	importTime, err := time.Parse(time.RFC3339, metadata.ImportMetadata.Timestamp)
	if err != nil {
		return fmt.Errorf("failed to parse import timestamp: %w", err)
	}

	// Create new catalog entry for database
	entry := database.CatalogEntry{
		ID:              metadata.ID,
		StoredPath:      storedPath,
		MetadataPath:    metadataPath,
		ImportTimestamp: importTime,
		Format:          metadata.ImportMetadata.Format,
		Origin:          metadata.SourceInfo.Origin,
		Schema:          metadata.SchemaInfo.CueDefinition,
		Confidence:      0.8, // Default confidence for now
		RecordCount:     metadata.ImportMetadata.RecordCount,
		SizeBytes:       metadata.ImportMetadata.SizeBytes,
	}

	// Add to database
	return i.catalogDB.AddEntry(entry)
}
