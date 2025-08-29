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

	for _, dir := range []string{unknownDir, awsDir, k8sDir} {
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
