package importer

import (
	"encoding/json"
	"os"
)

// ImportMetadata represents the metadata structure for imported data
type ImportMetadata struct {
	ID               string           `json:"id"`
	SourceInfo       SourceInfo       `json:"source_info"`
	ImportMetadata   ImportMeta       `json:"import_metadata"`
	SchemaInfo       SchemaInfo       `json:"schema_info"`
	ResourceTracking ResourceTracking `json:"resource_tracking"`
}

// SourceInfo contains information about the data source
type SourceInfo struct {
	Origin       string `json:"origin"`
	OriginalPath string `json:"original_path"`
	CommandHint  string `json:"command_hint,omitempty"`
	Confidence   string `json:"confidence"`
}

// ImportMeta contains metadata about the import operation
type ImportMeta struct {
	Timestamp   string `json:"timestamp"`
	Format      string `json:"format"`
	SizeBytes   int64  `json:"size_bytes"`
	RecordCount int    `json:"record_count"`
}

// SchemaInfo contains information about schema assignment
type SchemaInfo struct {
	CuePackage       string   `json:"cue_package"`
	CueDefinition    string   `json:"cue_definition"`
	SchemaFile       string   `json:"schema_file"`
	SchemaVersion    string   `json:"schema_version"`
	ValidationStatus string   `json:"validation_status"`
	ValidationErrors []string `json:"validation_errors,omitempty"`
	IntendedSchema   string   `json:"intended_schema,omitempty"`
}

// ResourceTracking contains information for tracking resource changes
type ResourceTracking struct {
	IdentityFields []string               `json:"identity_fields"`
	TrackedFields  []string               `json:"tracked_fields"`
	ResourceID     string                 `json:"resource_id,omitempty"`
	ContentHash    string                 `json:"content_hash,omitempty"`
	IdentityValues map[string]interface{} `json:"identity_values,omitempty"`
	Version        int                    `json:"version,omitempty"`
}

// CatalogEntry represents an entry in the data catalog
type CatalogEntry struct {
	ID              string  `json:"id"`
	StoredPath      string  `json:"stored_path"`
	MetadataPath    string  `json:"metadata_path"`
	ImportTimestamp string  `json:"import_timestamp"`
	Format          string  `json:"format"`
	Origin          string  `json:"origin"`
	Schema          string  `json:"schema"`
	Confidence      float64 `json:"confidence"`
	RecordCount     int     `json:"record_count"`
	SizeBytes       int64   `json:"size_bytes"`
}

// SchemaAssignment represents a schema assignment in the catalog
type SchemaAssignment struct {
	DataID               string  `json:"data_id"`
	CuePackage           string  `json:"cue_package"`
	CueDefinition        string  `json:"cue_definition"`
	AssignmentMethod     string  `json:"assignment_method"`
	Confidence           float64 `json:"confidence"`
	AssignmentTimestamp  string  `json:"assignment_timestamp"`
}

// SchemaRegistry represents the schema registry catalog
type SchemaRegistry struct {
	Schemas     map[string]SchemaRegistryEntry `json:"schemas"`
	LastUpdated string                         `json:"last_updated"`
	Version     string                         `json:"version"`
}

// SchemaRegistryEntry represents an entry in the schema registry
type SchemaRegistryEntry struct {
	FilePath       string   `json:"file_path"`
	Version        string   `json:"version"`
	IdentityFields []string `json:"identity_fields"`
	TrackedFields  []string `json:"tracked_fields"`
	LastModified   string   `json:"last_modified"`
}

// saveMetadata saves metadata to a JSON file
func (i *Importer) saveMetadata(metadata ImportMetadata, path string) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

