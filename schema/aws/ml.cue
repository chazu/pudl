package aws

// SageMakerModel represents an AWS SageMaker Model
#SageMakerModel: {
	// PUDL metadata
	_pudl: {
		schema_type: "base"
		resource_type: "aws.sagemaker.model"
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
	externalId: string & =~"^arn:aws:sagemaker:"
	name: string
	type: "AI_MODEL"
	nativeType: "sagemaker#model"
	
	// Technology info
	technology: {
		name: "AWS SageMaker Model"
		categories: [...{
			id: string
			name: string
		}]
		stackLayer: "MACHINE_LEARNING_AND_AI"
	}
	
	// Cloud platform info
	cloudPlatform: "AWS"
	cloudAccount: {
		id: string
		externalId: string
		cloudProvider: "AWS"
	}
	
	// Status and location
	status?: string | null
	region: string
	regionLocation: string
	
	// Tags and metadata
	tags?: [...{
		key: string
		value: string
	}] | null
	
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

// Generic AWS Resource - fallback for unrecognized AWS resources
#Resource: {
	// PUDL metadata
	_pudl: {
		schema_type: "base"
		resource_type: "aws.generic.resource"
		cascade_priority: 70
		cascade_fallback: ["unknown.#CatchAll"]
		identity_fields: ["id", "externalId"]
		tracked_fields: ["name", "type", "status", "tags", "updatedAt"]
		compliance_level: "permissive"
	}

	// Legacy metadata
	_identity: ["id", "externalId"]
	_tracked: ["name", "type", "status", "tags", "updatedAt"]
	_version: "v1.0"

	// Core fields
	id: string
	externalId?: string
	name?: string
	type?: string
	nativeType?: string
	
	// Technology info
	technology?: {
		name?: string
		categories?: [...{
			id?: string
			name?: string
		}]
		stackLayer?: string
	}
	
	// Cloud platform info
	cloudPlatform: "AWS"
	cloudAccount?: {
		id?: string
		externalId?: string
		cloudProvider?: "AWS"
	}
	
	// Status and location
	status?: string | null
	region?: string | null
	regionLocation?: string | null
	
	// Tags and metadata
	tags?: [...{
		key: string
		value: string
	}] | null
	
	// Timestamps
	createdAt?: string | null
	updatedAt?: string | null
	deletedAt?: string | null
	firstSeen?: string
	
	// Additional fields - flexible for unknown resource types
	projects?: [...] | null
	typeFields?: {...} | null
	resourceGroup?: {...} | null
	isOpenToAllInternet?: bool | null
	isAccessibleFromInternet?: bool | null
	
	// Allow additional fields for extensibility
	...
}
