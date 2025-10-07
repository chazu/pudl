package aws

// BatchJobDefinition represents an AWS Batch Job Definition
#BatchJobDefinition: {
	// PUDL metadata
	_pudl: {
		schema_type: "base"
		resource_type: "aws.batch.job_definition"
		cascade_priority: 90
		cascade_fallback: ["aws.#Resource", "unknown.#CatchAll"]
		identity_fields: ["id", "externalId"]
		tracked_fields: ["name", "status", "tags", "updatedAt"]
		compliance_level: "permissive"
	}

	// Legacy metadata
	_identity: ["id", "externalId"]
	_tracked: ["name", "status", "tags", "updatedAt"]
	_version: "v1.0"

	// Core fields
	id: string
	externalId: string & =~"^arn:aws:batch:"
	name: string
	type: "COMPUTE_INSTANCE_CONFIGURATION"
	nativeType: "batch#jobdefinition"
	
	// Technology info
	technology: {
		name: "AWS Batch Job Definition"
		categories: [...{
			id: string
			name: string
		}]
		stackLayer: "DATA_STORES"
	}
	
	// Cloud platform info
	cloudPlatform: "AWS"
	cloudAccount: {
		id: string
		externalId: string
		cloudProvider: "AWS"
	}
	
	// Status and location
	status: "Active" | "Inactive" | null
	region: string
	regionLocation: string
	
	// Tags and metadata
	tags?: [...{
		key: string
		value: string
	}]
	
	// Timestamps
	createdAt?: string | null
	updatedAt?: string | null
	deletedAt?: string | null
	firstSeen?: string
	
	// Additional fields
	projects?: [...] | null
	typeFields?: {...} | null
	resourceGroup?: {...} | null
	isOpenToAllInternet?: bool | null
	isAccessibleFromInternet?: bool | null
}

// ComputeEnvironment represents an AWS Batch Compute Environment
#ComputeEnvironment: {
	// PUDL metadata
	_pudl: {
		schema_type: "base"
		resource_type: "aws.batch.compute_environment"
		cascade_priority: 90
		cascade_fallback: ["aws.#Resource", "unknown.#CatchAll"]
		identity_fields: ["id", "externalId"]
		tracked_fields: ["name", "status", "tags", "updatedAt"]
		compliance_level: "permissive"
	}

	// Legacy metadata
	_identity: ["id", "externalId"]
	_tracked: ["name", "status", "tags", "updatedAt"]
	_version: "v1.0"

	// Core fields
	id: string
	externalId: string & =~"^arn:aws:batch:"
	name: string
	type: "DATA_WORKLOAD"
	nativeType: "batch#computeenvironment"
	
	// Technology info
	technology: {
		name: "AWS Batch Compute Environment"
		categories: [...{
			id: string
			name: string
		}]
		stackLayer: "DATA_STORES"
	}
	
	// Cloud platform info
	cloudPlatform: "AWS"
	cloudAccount: {
		id: string
		externalId: string
		cloudProvider: "AWS"
	}
	
	// Status and location
	status: "Active" | "Inactive" | null
	region: string
	regionLocation: string
	
	// Tags and metadata
	tags?: [...{
		key: string
		value: string
	}]
	
	// Timestamps
	createdAt?: string | null
	updatedAt?: string | null
	deletedAt?: string | null
	firstSeen?: string
	
	// Additional fields
	projects?: [...] | null
	typeFields?: {...} | null
	resourceGroup?: {...} | null
	isOpenToAllInternet?: bool | null
	isAccessibleFromInternet?: bool | null
}
