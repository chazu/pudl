package importer

import (
	"os"
	"path/filepath"
)

// createBasicSchemas creates basic CUE schema files in the schema directory
func (i *Importer) createBasicSchemas() error {
	// Create schema directories
	unknownDir := filepath.Join(i.schemaPath, "unknown")
	awsDir := filepath.Join(i.schemaPath, "aws")
	k8sDir := filepath.Join(i.schemaPath, "k8s")
	collectionsDir := filepath.Join(i.schemaPath, "collections")

	for _, dir := range []string{unknownDir, awsDir, k8sDir, collectionsDir} {
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

	// Legacy metadata (for backward compatibility)
	_identity: []
	_tracked: []
	_version: "v1.0"

	// Accept any structure
	...
}
`
	if err := os.WriteFile(filepath.Join(unknownDir, "catchall.cue"), []byte(catchallSchema), 0644); err != nil {
		return err
	}

	// Create aws/ec2.cue
	ec2Schema := `package aws

import "time"

// EC2Instance defines the schema for AWS EC2 instance data
#EC2Instance: {
	// PUDL metadata for cascading validation
	_pudl: {
		schema_type: "base"
		resource_type: "aws.ec2.instance"
		cascade_priority: 100
		cascade_fallback: ["aws.#Resource", "unknown.#CatchAll"]
		identity_fields: ["InstanceId"]
		tracked_fields: ["State", "PrivateIpAddress", "Tags", "SecurityGroups"]
		compliance_level: "permissive"
	}

	// Legacy metadata (for backward compatibility)
	_identity: ["InstanceId"]
	_tracked: ["State", "PrivateIpAddress", "Tags", "SecurityGroups"]
	_version: "v1.0"

	// Actual data schema
	InstanceId: string & =~"^i-[0-9a-f]{8,17}$"
	State: {
		Code: int
		Name: "pending" | "running" | "shutting-down" | "terminated" | "stopping" | "stopped"
	}
	InstanceType: string
	LaunchTime?: time.Time
	PrivateIpAddress?: string
	Tags?: [...{
		Key: string
		Value: string
	}]
	SecurityGroups?: [...{
		GroupId: string
		GroupName: string
	}]
}

// S3Bucket defines the schema for AWS S3 bucket data
#S3Bucket: {
	// PUDL metadata for cascading validation
	_pudl: {
		schema_type: "base"
		resource_type: "aws.s3.bucket"
		cascade_priority: 100
		cascade_fallback: ["aws.#Resource", "unknown.#CatchAll"]
		identity_fields: ["Name"]
		tracked_fields: ["CreationDate", "BucketPolicy", "Tags"]
		compliance_level: "permissive"
	}

	// Legacy metadata (for backward compatibility)
	_identity: ["Name"]
	_tracked: ["CreationDate", "BucketPolicy", "Tags"]
	_version: "v1.0"

	Name: string
	CreationDate: time.Time
	BucketPolicy?: string
	Tags?: [...{
		Key: string
		Value: string
	}]
}

// Generic AWS API Response
#APIResponse: {
	// PUDL metadata for cascading validation
	_pudl: {
		schema_type: "base"
		resource_type: "aws.api.response"
		cascade_priority: 50
		cascade_fallback: ["aws.#Resource", "unknown.#CatchAll"]
		identity_fields: []
		tracked_fields: []
		compliance_level: "permissive"
	}

	// Legacy metadata (for backward compatibility)
	_identity: []
	_tracked: []
	_version: "v1.0"

	ResponseMetadata: {
		RequestId: string
		HTTPStatusCode?: int
		HTTPHeaders?: {...}
	}
	...
}

// Generic AWS Resource
#Resource: {
	// PUDL metadata for cascading validation
	_pudl: {
		schema_type: "generic"
		resource_type: "aws.resource"
		cascade_priority: 10
		cascade_fallback: ["unknown.#CatchAll"]
		identity_fields: []
		tracked_fields: []
		compliance_level: "permissive"
	}

	// Legacy metadata (for backward compatibility)
	_identity: []
	_tracked: []
	_version: "v1.0"

	// Accept any AWS resource structure
	...
}
`
	if err := os.WriteFile(filepath.Join(awsDir, "ec2.cue"), []byte(ec2Schema), 0644); err != nil {
		return err
	}

	// Create aws/compliant-ec2.cue (example policy schema)
	compliantEC2Schema := `package aws

import "time"

// CompliantEC2Instance defines a policy-compliant AWS EC2 instance
#CompliantEC2Instance: {
	// PUDL metadata for cascading validation
	_pudl: {
		schema_type: "policy"
		resource_type: "aws.ec2.instance"
		base_schema: "aws.#EC2Instance"
		cascade_priority: 200
		cascade_fallback: ["aws.#EC2Instance", "aws.#Resource", "unknown.#CatchAll"]
		identity_fields: ["InstanceId"]
		tracked_fields: ["State", "PrivateIpAddress", "Tags", "SecurityGroups"]
		compliance_level: "strict"
	}

	// Inherit from base EC2Instance schema
	#EC2Instance

	// Business rule constraints
	InstanceType: "t3.micro" | "t3.small" | "t3.medium"  // Only approved types

	// Required tags for compliance
	Tags: [...{Key: string, Value: string}] & [
		{Key: "Environment", Value: "prod" | "staging" | "dev"},
		{Key: "Owner", Value: string & =~"^[a-zA-Z0-9._%+-]+@company\\.com$"},
		...
	]

	// Security group restrictions
	SecurityGroups: [...{
		GroupId: string & =~"^sg-[0-9a-f]{8,17}$"
		GroupName: string & !="default"  // No default security groups
	}]
}
`
	if err := os.WriteFile(filepath.Join(awsDir, "compliant-ec2.cue"), []byte(compliantEC2Schema), 0644); err != nil {
		return err
	}

	// Create k8s/resources.cue
	k8sSchema := `package k8s

// Pod defines the schema for Kubernetes Pod resources
#Pod: {
	// PUDL metadata for cascading validation
	_pudl: {
		schema_type: "base"
		resource_type: "k8s.pod"
		cascade_priority: 100
		cascade_fallback: ["k8s.#Resource", "unknown.#CatchAll"]
		identity_fields: ["metadata.name", "metadata.namespace"]
		tracked_fields: ["status", "spec"]
		compliance_level: "permissive"
	}

	// Legacy metadata (for backward compatibility)
	_identity: ["metadata.name", "metadata.namespace"]
	_tracked: ["status", "spec"]
	_version: "v1.0"

	apiVersion: "v1"
	kind: "Pod"
	metadata: {
		name: string
		namespace?: string
		labels?: {...}
		annotations?: {...}
	}
	spec: {
		containers: [...{
			name: string
			image: string
			...
		}]
		...
	}
	status?: {
		phase?: "Pending" | "Running" | "Succeeded" | "Failed" | "Unknown"
		...
	}
}

// Generic Kubernetes Resource
#Resource: {
	// PUDL metadata for cascading validation
	_pudl: {
		schema_type: "generic"
		resource_type: "k8s.resource"
		cascade_priority: 10
		cascade_fallback: ["unknown.#CatchAll"]
		identity_fields: ["metadata.name", "metadata.namespace"]
		tracked_fields: ["status", "spec"]
		compliance_level: "permissive"
	}

	// Legacy metadata (for backward compatibility)
	_identity: ["metadata.name", "metadata.namespace"]
	_tracked: ["status", "spec"]
	_version: "v1.0"

	apiVersion: string
	kind: string
	metadata: {
		name: string
		namespace?: string
		...
	}
	...
}
`
	if err := os.WriteFile(filepath.Join(k8sDir, "resources.cue"), []byte(k8sSchema), 0644); err != nil {
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

	// Legacy metadata
	_identity: ["collection_id"]
	_tracked: ["item_count", "format"]
	_version: "v1.0"

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
	catchallPath := filepath.Join(i.schemaPath, "unknown", "catchall.cue")
	if _, err := os.Stat(catchallPath); os.IsNotExist(err) {
		return i.createBasicSchemas()
	}
	return nil
}
