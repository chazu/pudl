package core

// Item is the universal fallback schema for any individual piece of data.
// All more specific schemas should cascade to this as their ultimate fallback.
#Item: {
	// PUDL metadata for cascading validation
	_pudl: {
		schema_type:      "catchall"
		resource_type:    "unknown"
		identity_fields: []
		tracked_fields: []
	}

	// Accept any structure
	...
}

// Collection represents a collection of related data items (e.g., NDJSON files)
#Collection: {
	// PUDL metadata for cascading validation
	_pudl: {
		schema_type:      "collection"
		resource_type:    "generic.collection"
		identity_fields: ["collection_id"]
		tracked_fields: ["item_count", "item_schemas", "collection_metadata"]
	}

	// Core collection fields
	collection_id:     string & =~"^[a-zA-Z0-9_-]+$"
	original_filename: string
	format:            "ndjson" | "json-array" | "csv-multi" | "yaml-multi"
	item_count:        int & >=0

	// Schema distribution within collection
	item_schemas: [...{
		schema:     string
		count:      int & >=0
		confidence: number & >=0 & <=1
	}]

	// Flexible collection-level metadata - can accommodate any type of collection
	collection_metadata: {
		source_info: {
			original_path:    string
			file_size_bytes:  int & >=0
			import_timestamp: string
			origin:           string
		}
		processing_info: {
			parsing_method:      "streaming" | "memory"
			processing_time_ms?: int & >=0
			errors_encountered?: int & >=0
		}
		content_summary?: {
			data_types?: [...string]
			date_range?: {
				earliest?: string
				latest?:   string
			}
			common_fields?: [...string]
		}
		// Allow any additional metadata fields for flexibility
		...
	}

	// Optional: First few items for preview (not stored for large collections)
	sample_items?: [...#Item] & len(<=10)
}

