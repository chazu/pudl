package collections

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

	// Legacy metadata (for backward compatibility)
	_identity: ["collection_id"]
	_tracked: ["item_count", "item_schemas", "collection_metadata"]
	_version: "v1.0"

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
	
	// Collection-level metadata
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

	// Legacy metadata
	_identity: ["item_id", "collection_id"]
	_tracked: ["item_data"]
	_version: "v1.0"

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

// CloudInventoryCollection - Specific schema for cloud resource inventories
#CloudInventoryCollection: #Collection & {
	_pudl: {
		resource_type: "cloud.inventory_collection"
		cascade_priority: 85
	}
	
	format: "ndjson"
	
	// Enhanced metadata for cloud inventories
	collection_metadata: {
		cloud_info: {
			cloud_provider?: "AWS" | "Azure" | "GCP" | "Multi-Cloud"
			account_ids?: [...string]
			regions?: [...string]
			resource_types?: [...string]
			collection_date?: string
		}
		statistics: {
			total_resources: int & >=0
			active_resources?: int & >=0
			inactive_resources?: int & >=0
			resource_type_breakdown?: {...}
		}
	}
}

// LogCollection - Specific schema for log file collections
#LogCollection: #Collection & {
	_pudl: {
		resource_type: "logs.log_collection"
		cascade_priority: 85
	}
	
	format: "ndjson"
	
	collection_metadata: {
		log_info: {
			log_type?: "application" | "system" | "security" | "audit" | "access"
			service_name?: string
			environment?: "production" | "staging" | "development" | "test"
			log_level_distribution?: {
				error?: int
				warn?: int
				info?: int
				debug?: int
			}
		}
		time_range?: {
			start_time?: string
			end_time?: string
			duration_hours?: number
		}
	}
}

// APIResponseCollection - For collections of API responses
#APIResponseCollection: #Collection & {
	_pudl: {
		resource_type: "api.response_collection"
		cascade_priority: 85
	}
	
	collection_metadata: {
		api_info: {
			endpoint?: string
			method?: "GET" | "POST" | "PUT" | "DELETE" | "PATCH"
			api_version?: string
			response_codes?: [...int]
			pagination_info?: {
				total_pages?: int
				items_per_page?: int
				has_more?: bool
			}
		}
	}
}

// MetricsCollection - For collections of metrics and monitoring data
#MetricsCollection: #Collection & {
	_pudl: {
		resource_type: "metrics.metrics_collection"
		cascade_priority: 85
	}

	format: "ndjson"

	collection_metadata: {
		metrics_info: {
			metric_types?: [...string]
			time_series?: bool
			aggregation_level?: "raw" | "minute" | "hour" | "day"
			source_system?: string
			namespace?: string
		}
		time_range?: {
			start_time?: string
			end_time?: string
			resolution?: string
		}
	}
}

// DatabaseCollection - For collections of database records/exports
#DatabaseCollection: #Collection & {
	_pudl: {
		resource_type: "database.record_collection"
		cascade_priority: 85
	}

	collection_metadata: {
		database_info: {
			database_type?: "postgresql" | "mysql" | "mongodb" | "redis" | "elasticsearch"
			table_name?: string
			schema_name?: string
			export_type?: "full" | "incremental" | "snapshot"
			query_used?: string
		}
		record_info: {
			primary_key_field?: string
			timestamp_field?: string
			record_version?: string
		}
	}
}
