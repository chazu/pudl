package aws

// SecurityGroup represents an AWS EC2 Security Group
#SecurityGroup: {
	// PUDL metadata
	_pudl: {
		schema_type: "base"
		resource_type: "aws.ec2.security_group"
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
	externalId: string & =~"^sg-"
	name: string
	type: "FIREWALL"
	nativeType: "securityGroup"
	
	// Technology info
	technology: {
		name: "AWS EC2 Security Group"
		categories: [...{
			id: string
			name: string
		}]
		stackLayer: "NETWORKING"
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

// Secret represents an AWS Secrets Manager Secret
#Secret: {
	// PUDL metadata
	_pudl: {
		schema_type: "base"
		resource_type: "aws.secretsmanager.secret"
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
	externalId: string & =~"^arn:aws:secretsmanager:"
	name: string
	type: "SECRET"
	nativeType: "secret"
	
	// Technology info
	technology: {
		name: "AWS Secret"
		categories: [...{
			id: string
			name: string
		}]
		stackLayer: "SECURITY_AND_IDENTITY"
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

// IAMPolicy represents an AWS IAM Policy (inline or assume role)
#IAMPolicy: {
	// PUDL metadata
	_pudl: {
		schema_type: "base"
		resource_type: "aws.iam.policy"
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
	externalId: string & =~"^arn:aws:iam:"
	name: string
	type: "RAW_ACCESS_POLICY"
	nativeType: "inlinePolicy" | "assumeRolePolicy"
	
	// Technology info
	technology: {
		name: "AWS IAM Inline Policy" | "AWS IAM Assumed Role Policy"
		categories: [...{
			id: string
			name: string
		}]
		stackLayer: "CLOUD_ENTITLEMENTS"
	}
	
	// Cloud platform info
	cloudPlatform: "AWS"
	cloudAccount: {
		id: string
		externalId: string
		cloudProvider: "AWS"
	}
	
	// Status and location (policies don't have region)
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
	
	// Additional fields
	projects?: [...] | null
	typeFields?: {...} | null
	resourceGroup?: {...} | null
	isOpenToAllInternet?: bool | null
	isAccessibleFromInternet?: bool | null
}
