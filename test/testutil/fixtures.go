package testutil

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TestDataFixtures provides common test data for various formats
type TestDataFixtures struct{}

// NewTestDataFixtures creates a new fixtures instance
func NewTestDataFixtures() *TestDataFixtures {
	return &TestDataFixtures{}
}

// ValidJSON returns a valid JSON object for testing
func (f *TestDataFixtures) ValidJSON() string {
	return `{
  "name": "test-item",
  "type": "example",
  "count": 42,
  "active": true,
  "metadata": {
    "created": "2024-01-01T00:00:00Z",
    "tags": ["test", "example"]
  }
}`
}

// ValidJSONArray returns a valid JSON array for testing
func (f *TestDataFixtures) ValidJSONArray() string {
	return `[
  {
    "id": 1,
    "name": "item-1",
    "value": 100
  },
  {
    "id": 2,
    "name": "item-2",
    "value": 200
  }
]`
}

// InvalidJSON returns malformed JSON for error testing
func (f *TestDataFixtures) InvalidJSON() string {
	return `{
  "name": "test-item",
  "type": "example"
  "count": 42,  // missing comma
  "active": true
}`
}

// ValidYAML returns a valid YAML document for testing
func (f *TestDataFixtures) ValidYAML() string {
	return `name: test-item
type: example
count: 42
active: true
metadata:
  created: "2024-01-01T00:00:00Z"
  tags:
    - test
    - example`
}

// InvalidYAML returns malformed YAML for error testing
func (f *TestDataFixtures) InvalidYAML() string {
	return `name: test-item
type: example
  count: 42  # incorrect indentation
active: true`
}

// ValidNDJSON returns a valid NDJSON collection for testing
func (f *TestDataFixtures) ValidNDJSON() string {
	return `{"id": 1, "name": "item-1", "type": "user"}
{"id": 2, "name": "item-2", "type": "user"}
{"id": 3, "name": "item-3", "type": "admin"}`
}

// LargeNDJSON returns a larger NDJSON collection for streaming tests
func (f *TestDataFixtures) LargeNDJSON(itemCount int) string {
	var lines []string
	for i := 1; i <= itemCount; i++ {
		item := map[string]interface{}{
			"id":     i,
			"name":   fmt.Sprintf("item-%d", i),
			"type":   "generated",
			"value":  i * 10,
			"active": i%2 == 0,
		}
		jsonBytes, _ := json.Marshal(item)
		lines = append(lines, string(jsonBytes))
	}
	return strings.Join(lines, "\n")
}

// KubernetesPod returns a sample Kubernetes Pod YAML for testing
func (f *TestDataFixtures) KubernetesPod() string {
	return `apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: default
  labels:
    app: test-app
spec:
  containers:
  - name: main
    image: nginx:latest
    ports:
    - containerPort: 80
      protocol: TCP
  restartPolicy: Always`
}

// AWSInstance returns a sample AWS EC2 instance JSON for testing
func (f *TestDataFixtures) AWSInstance() string {
	return `{
  "InstanceId": "i-1234567890abcdef0",
  "ImageId": "ami-12345678",
  "State": {
    "Code": 16,
    "Name": "running"
  },
  "PrivateDnsName": "ip-10-0-0-1.ec2.internal",
  "PublicDnsName": "ec2-203-0-113-12.compute-1.amazonaws.com",
  "StateTransitionReason": "",
  "InstanceType": "t2.micro",
  "KeyName": "my-key-pair",
  "LaunchTime": "2024-01-01T12:00:00.000Z",
  "Placement": {
    "AvailabilityZone": "us-east-1a",
    "GroupName": "",
    "Tenancy": "default"
  },
  "Monitoring": {
    "State": "disabled"
  },
  "SubnetId": "subnet-12345678",
  "VpcId": "vpc-12345678",
  "PrivateIpAddress": "10.0.0.1",
  "PublicIpAddress": "203.0.113.12",
  "Architecture": "x86_64",
  "RootDeviceType": "ebs",
  "RootDeviceName": "/dev/sda1",
  "BlockDeviceMappings": [
    {
      "DeviceName": "/dev/sda1",
      "Ebs": {
        "VolumeId": "vol-1234567890abcdef0",
        "Status": "attached",
        "AttachTime": "2024-01-01T12:00:00.000Z",
        "DeleteOnTermination": true
      }
    }
  ],
  "Tags": [
    {
      "Key": "Name",
      "Value": "test-instance"
    },
    {
      "Key": "Environment",
      "Value": "test"
    }
  ]
}`
}

// CSVData returns sample CSV data for testing
func (f *TestDataFixtures) CSVData() string {
	return `id,name,email,age,active
1,John Doe,john@example.com,30,true
2,Jane Smith,jane@example.com,25,false
3,Bob Johnson,bob@example.com,35,true`
}

// SchemaFixtures provides common CUE schema fixtures
type SchemaFixtures struct{}

// BasicSchema returns a basic CUE schema for testing
func (s *SchemaFixtures) BasicSchema() string {
	return `package unknown

#BasicItem: {
	name:   string
	type:   string
	count:  int & >=0
	active: bool
	
	// PUDL metadata
	_pudl: {
		schema_type:      "unknown"
		resource_type:    "basic_item"
		identity_fields:  ["name"]
		tracked_fields:   ["count", "active"]
	}
}`
}

// KubernetesSchema returns a Kubernetes Pod schema for testing
func (s *SchemaFixtures) KubernetesSchema() string {
	return `package k8s

#Pod: {
	apiVersion: "v1"
	kind:       "Pod"
	metadata: {
		name:      string
		namespace: string | *"default"
		labels?: [string]: string
	}
	spec: {
		containers: [...{
			name:  string
			image: string
			ports?: [...{
				containerPort: int
				protocol?:     "TCP" | "UDP"
			}]
		}]
		restartPolicy?: "Always" | "OnFailure" | "Never"
	}
	
	// PUDL metadata
	_pudl: {
		schema_type:      "kubernetes"
		resource_type:    "pod"
		identity_fields:  ["metadata.name", "metadata.namespace"]
		tracked_fields:   ["spec.containers"]
	}
}`
}

// ConfigFixtures provides configuration file fixtures
type ConfigFixtures struct{}
