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
	// No identity or tracking for unknown data
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
	// Identity fields - used for resource tracking
	_identity: ["InstanceId"]
	
	// Tracked fields - monitored for changes
	_tracked: ["State", "PrivateIpAddress", "Tags", "SecurityGroups"]
	
	// Schema version for evolution tracking
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

	// Create k8s/resources.cue
	k8sSchema := `package k8s

// Pod defines the schema for Kubernetes Pod resources
#Pod: {
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
