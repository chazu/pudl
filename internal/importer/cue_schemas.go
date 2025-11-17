package importer

import (
	"os"
	"path/filepath"
)

// createBasicSchemas creates basic CUE schema files in the schema directory
func (i *Importer) createBasicSchemas() error {
	// Create schema directories under pudl/ for local schemas
	pudlDir := filepath.Join(i.schemaPath, "pudl")
	unknownDir := filepath.Join(pudlDir, "unknown")
	collectionsDir := filepath.Join(pudlDir, "collections")

	for _, dir := range []string{unknownDir, collectionsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	// Create unknown/catchall.cue
	catchallSchema := `package unknown

// CatchAll schema for unclassified data
#CatchAll: {
	// PUDL metadata for cascading validation
	_pudl: {
		schema_type: "catchall"
		resource_type: "unknown"
		cascade_priority: 0
		identity_fields: []
		tracked_fields: []
		compliance_level: "permissive"
	}

	// Accept any structure
	...
}
`
	if err := os.WriteFile(filepath.Join(unknownDir, "catchall.cue"), []byte(catchallSchema), 0644); err != nil {
		return err
	}

	// Create collections/collections.cue
	collectionsSchema := `package collections

// Collection represents a collection of related data items (e.g., NDJSON files)
#Collection: {
	// PUDL metadata for cascading validation
	_pudl: {
		schema_type: "collection"
		resource_type: "generic.collection"
		cascade_priority: 75
		cascade_fallback: ["unknown.#CatchAll"]
		identity_fields: ["collection_id"]
		tracked_fields: ["item_count", "item_schemas", "collection_metadata"]
		compliance_level: "permissive"
	}

	// Core collection fields
	collection_id: string & =~"^[a-zA-Z0-9_-]+$"
	original_filename: string
	format: "ndjson" | "json-array" | "csv-multi" | "yaml-multi"
	item_count: int & >=0

	// Schema distribution within collection
	item_schemas: [...{
		schema: string
		count: int & >=0
		confidence: number & >=0 & <=1
	}]

	// Flexible collection-level metadata - can accommodate any type of collection
	collection_metadata: {
		source_info: {
			original_path: string
			file_size_bytes: int & >=0
			import_timestamp: string
			origin: string
		}
		processing_info: {
			parsing_method: "streaming" | "memory"
			processing_time_ms?: int & >=0
			errors_encountered?: int & >=0
		}
		content_summary?: {
			data_types?: [...string]
			date_range?: {
				earliest?: string
				latest?: string
			}
			common_fields?: [...string]
		}
		// Allow any additional metadata fields for flexibility
		...
	}

	// Optional: First few items for preview (not stored for large collections)
	sample_items?: [...#CollectionItem] & len(<=10)
}

// CollectionItem represents an individual item within a collection
#CollectionItem: {
	// PUDL metadata for collection items
	_pudl: {
		schema_type: "collection_item"
		resource_type: "generic.collection_item"
		cascade_priority: 60
		identity_fields: ["item_id", "collection_id"]
		tracked_fields: ["item_data"]
		parent_collection?: string
		item_index?: int
	}

	// Item identification
	item_id: string
	collection_id: string
	item_index: int & >=0

	// Item metadata
	item_metadata: {
		extracted_at: string
		schema_assigned: string
		schema_confidence: number & >=0 & <=1
		size_bytes?: int & >=0
		validation_status: "valid" | "invalid" | "warning" | "unknown"
		validation_errors?: [...string]
	}

	// Flexible item data - actual content varies by item type
	item_data: {...}
}

// Generic collection catch-all for unclassified collections
#CatchAllCollection: {
	// PUDL metadata for cascading validation
	_pudl: {
		schema_type: "catchall_collection"
		resource_type: "generic.catchall_collection"
		cascade_priority: 5
		cascade_fallback: ["unknown.#CatchAll"]
		identity_fields: ["collection_id"]
		tracked_fields: ["item_count", "format"]
		compliance_level: "permissive"
	}

	// Minimal collection structure - accepts any collection-like data
	collection_id: string
	original_filename?: string
	format?: string
	item_count?: int & >=0

	// Accept any additional collection metadata
	...
}
`
	if err := os.WriteFile(filepath.Join(collectionsDir, "collections.cue"), []byte(collectionsSchema), 0644); err != nil {
		return err
	}

	return nil
}

// ensureBasicSchemas ensures that basic schema files exist
func (i *Importer) ensureBasicSchemas() error {
	// Check if catchall schema exists
	catchallPath := filepath.Join(i.schemaPath, "pudl", "unknown", "catchall.cue")
	if _, err := os.Stat(catchallPath); os.IsNotExist(err) {
		return i.createBasicSchemas()
	}
	return nil
}
