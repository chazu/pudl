package infrastructure

import (
	"fmt"
)

// GetTestDataSet returns a curated test dataset by name
func GetTestDataSet(name string) (*TestDataSet, bool) {
	datasets := map[string]*TestDataSet{
		"aws-production-sample":    getAWSProductionSample(),
		"k8s-cluster-snapshot":     getK8sClusterSnapshot(),
		"mixed-environment":        getMixedEnvironment(),
		"large-dataset":           getLargeDataset(),
		"corrupted-data":          getCorruptedDataset(),
		"minimal-test":            getMinimalTestDataset(),
	}
	
	dataset, exists := datasets[name]
	return dataset, exists
}

// getAWSProductionSample returns a realistic AWS production environment sample
func getAWSProductionSample() *TestDataSet {
	generator := NewTestFileGenerator()
	
	return &TestDataSet{
		Name:        "AWS Production Environment Sample",
		Description: "Realistic AWS resource data from production-like environment (100% synthetic)",
		Files: []TestFile{
			{
				Name:            "aws-ec2-instances.json",
				Content:         generator.GenerateAWSEC2Response(15),
				ExpectedRecords: 15,
				ExpectedSchema:  "aws.#EC2Instance",
				ExpectedOrigin:  "aws-ec2-describe-instances",
				Format:          "json",
			},
			{
				Name:            "aws-s3-buckets.json",
				Content:         generator.GenerateAWSS3Response(8),
				ExpectedRecords: 8,
				ExpectedSchema:  "aws.#S3Bucket",
				ExpectedOrigin:  "aws-s3-list-buckets",
				Format:          "json",
			},
		},
		Metadata: DataSetMetadata{
			TotalFiles:   2,
			TotalRecords: 23,
			TotalSize:    0, // Will be calculated when files are created
			Formats:      []string{"json"},
			Origins:      []string{"aws-ec2-describe-instances", "aws-s3-list-buckets"},
			Schemas:      []string{"aws.#EC2Instance", "aws.#S3Bucket"},
		},
	}
}

// getK8sClusterSnapshot returns a Kubernetes cluster state snapshot
func getK8sClusterSnapshot() *TestDataSet {
	generator := NewTestFileGenerator()
	
	return &TestDataSet{
		Name:        "Kubernetes Cluster State Snapshot",
		Description: "Complete cluster state from kubectl get all --all-namespaces (100% synthetic)",
		Files: []TestFile{
			{
				Name:            "k8s-pods.yaml",
				Content:         generator.GenerateKubernetesPods(25),
				ExpectedRecords: 25,
				ExpectedSchema:  "k8s.#Pod",
				ExpectedOrigin:  "k8s-get-pods",
				Format:          "yaml",
			},
			{
				Name:            "k8s-services.yaml",
				Content:         generator.GenerateKubernetesServices(10),
				ExpectedRecords: 10,
				ExpectedSchema:  "k8s.#Service",
				ExpectedOrigin:  "k8s-get-services",
				Format:          "yaml",
			},
		},
		Metadata: DataSetMetadata{
			TotalFiles:   2,
			TotalRecords: 35,
			TotalSize:    0,
			Formats:      []string{"yaml"},
			Origins:      []string{"k8s-get-pods", "k8s-get-services"},
			Schemas:      []string{"k8s.#Pod", "k8s.#Service"},
		},
	}
}

// getMixedEnvironment returns a mixed cloud/on-prem environment dataset
func getMixedEnvironment() *TestDataSet {
	generator := NewTestFileGenerator()
	
	return &TestDataSet{
		Name:        "Mixed Environment Dataset",
		Description: "Combination of AWS, Kubernetes, and generic data sources (100% synthetic)",
		Files: []TestFile{
			{
				Name:            "aws-instances.json",
				Content:         generator.GenerateAWSEC2Response(10),
				ExpectedRecords: 10,
				ExpectedSchema:  "aws.#EC2Instance",
				ExpectedOrigin:  "aws-ec2-describe-instances",
				Format:          "json",
			},
			{
				Name:            "k8s-workloads.yaml",
				Content:         generator.GenerateKubernetesPods(15),
				ExpectedRecords: 15,
				ExpectedSchema:  "k8s.#Pod",
				ExpectedOrigin:  "k8s-get-pods",
				Format:          "yaml",
			},
			{
				Name:            "application-logs.ndjson",
				Content:         generator.GenerateLargeNDJSON(50),
				ExpectedRecords: 50,
				ExpectedSchema:  "logs.#LogEntry",
				ExpectedOrigin:  "application-logs",
				Format:          "ndjson",
			},
		},
		Metadata: DataSetMetadata{
			TotalFiles:   3,
			TotalRecords: 75,
			TotalSize:    0,
			Formats:      []string{"json", "yaml", "ndjson"},
			Origins:      []string{"aws-ec2-describe-instances", "k8s-get-pods", "application-logs"},
			Schemas:      []string{"aws.#EC2Instance", "k8s.#Pod", "logs.#LogEntry"},
		},
	}
}

// getLargeDataset returns a large dataset for performance testing
func getLargeDataset() *TestDataSet {
	generator := NewTestFileGenerator()
	
	return &TestDataSet{
		Name:        "Large Dataset for Performance Testing",
		Description: "Large synthetic dataset for testing performance and scalability",
		Files: []TestFile{
			{
				Name:            "large-aws-instances.json",
				Content:         generator.GenerateAWSEC2Response(500),
				ExpectedRecords: 500,
				ExpectedSchema:  "aws.#EC2Instance",
				ExpectedOrigin:  "aws-ec2-describe-instances",
				Format:          "json",
			},
			{
				Name:            "large-k8s-pods.yaml",
				Content:         generator.GenerateKubernetesPods(300),
				ExpectedRecords: 300,
				ExpectedSchema:  "k8s.#Pod",
				ExpectedOrigin:  "k8s-get-pods",
				Format:          "yaml",
			},
			{
				Name:            "large-logs.ndjson",
				Content:         generator.GenerateLargeNDJSON(1000),
				ExpectedRecords: 1000,
				ExpectedSchema:  "logs.#LogEntry",
				ExpectedOrigin:  "application-logs",
				Format:          "ndjson",
			},
		},
		Metadata: DataSetMetadata{
			TotalFiles:   3,
			TotalRecords: 1800,
			TotalSize:    0,
			Formats:      []string{"json", "yaml", "ndjson"},
			Origins:      []string{"aws-ec2-describe-instances", "k8s-get-pods", "application-logs"},
			Schemas:      []string{"aws.#EC2Instance", "k8s.#Pod", "logs.#LogEntry"},
		},
	}
}

// getCorruptedDataset returns a dataset with various types of data corruption
func getCorruptedDataset() *TestDataSet {
	generator := NewTestFileGenerator()
	
	return &TestDataSet{
		Name:        "Corrupted Data Test Dataset",
		Description: "Dataset with various types of data corruption for error handling tests",
		Files: []TestFile{
			{
				Name:            "corrupted-missing-brace.json",
				Content:         generator.GenerateCorruptedJSON("missing_brace"),
				ExpectedRecords: 0, // Should fail to parse
				ExpectedSchema:  "",
				ExpectedOrigin:  "corrupted-test",
				Format:          "json",
			},
			{
				Name:            "corrupted-invalid-json.json",
				Content:         generator.GenerateCorruptedJSON("invalid_json"),
				ExpectedRecords: 0, // Should fail to parse
				ExpectedSchema:  "",
				ExpectedOrigin:  "corrupted-test",
				Format:          "json",
			},
			{
				Name:            "corrupted-truncated.json",
				Content:         generator.GenerateCorruptedJSON("truncated"),
				ExpectedRecords: 0, // Should fail to parse
				ExpectedSchema:  "",
				ExpectedOrigin:  "corrupted-test",
				Format:          "json",
			},
			{
				Name:            "corrupted-invalid-utf8.json",
				Content:         generator.GenerateCorruptedJSON("invalid_utf8"),
				ExpectedRecords: 0, // Should fail to parse
				ExpectedSchema:  "",
				ExpectedOrigin:  "corrupted-test",
				Format:          "json",
			},
		},
		Metadata: DataSetMetadata{
			TotalFiles:   4,
			TotalRecords: 0, // All files should fail to parse
			TotalSize:    0,
			Formats:      []string{"json"},
			Origins:      []string{"corrupted-test"},
			Schemas:      []string{},
		},
	}
}

// getMinimalTestDataset returns a minimal dataset for quick tests
func getMinimalTestDataset() *TestDataSet {
	return &TestDataSet{
		Name:        "Minimal Test Dataset",
		Description: "Small dataset for quick validation tests",
		Files: []TestFile{
			{
				Name:    "simple-aws.json",
				Content: getSimpleAWSInstance(),
				ExpectedRecords: 1,
				ExpectedSchema:  "aws.#EC2Instance",
				ExpectedOrigin:  "aws-ec2-describe-instances",
				Format:          "json",
			},
			{
				Name:    "simple-k8s.yaml",
				Content: getSimpleK8sPod(),
				ExpectedRecords: 1,
				ExpectedSchema:  "k8s.#Pod",
				ExpectedOrigin:  "k8s-get-pods",
				Format:          "yaml",
			},
		},
		Metadata: DataSetMetadata{
			TotalFiles:   2,
			TotalRecords: 2,
			TotalSize:    0,
			Formats:      []string{"json", "yaml"},
			Origins:      []string{"aws-ec2-describe-instances", "k8s-get-pods"},
			Schemas:      []string{"aws.#EC2Instance", "k8s.#Pod"},
		},
	}
}

// getSimpleAWSInstance returns a simple AWS EC2 instance JSON
func getSimpleAWSInstance() string {
	return `{
  "Reservations": [
    {
      "Instances": [
        {
          "InstanceId": "i-1234567890abcdef0",
          "ImageId": "ami-12345678",
          "State": {
            "Code": 16,
            "Name": "running"
          },
          "PrivateDnsName": "ip-10-0-1-100.ec2.internal",
          "PublicDnsName": "ec2-203-0-113-25.compute-1.amazonaws.com",
          "InstanceType": "t3.micro",
          "KeyName": "test-key",
          "LaunchTime": "2023-01-01T12:00:00.000Z",
          "Placement": {
            "AvailabilityZone": "us-east-1a",
            "Tenancy": "default"
          },
          "VpcId": "vpc-12345678",
          "SubnetId": "subnet-12345678",
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
        }
      ],
      "OwnerId": "123456789012",
      "ReservationId": "r-1234567890abcdef0"
    }
  ]
}`
}

// getSimpleK8sPod returns a simple Kubernetes Pod YAML
func getSimpleK8sPod() string {
	return `apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: default
  uid: 12345678-1234-1234-1234-123456789012
  creationTimestamp: "2023-01-01T12:00:00Z"
  labels:
    app: test-app
    version: v1.0
spec:
  containers:
  - name: test-container
    image: test-registry/test-app:latest
    ports:
    - containerPort: 8080
      protocol: TCP
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 500m
        memory: 256Mi
  restartPolicy: Always
  nodeName: node-1
status:
  phase: Running
  podIP: 10.244.1.100
  startTime: "2023-01-01T12:00:00Z"`
}

// ListAvailableDataSets returns a list of all available test datasets
func ListAvailableDataSets() []string {
	return []string{
		"aws-production-sample",
		"k8s-cluster-snapshot", 
		"mixed-environment",
		"large-dataset",
		"corrupted-data",
		"minimal-test",
	}
}

// GetDataSetInfo returns information about a specific dataset
func GetDataSetInfo(name string) string {
	dataset, exists := GetTestDataSet(name)
	if !exists {
		return fmt.Sprintf("Dataset '%s' not found", name)
	}
	
	return fmt.Sprintf(`Dataset: %s
Description: %s
Files: %d
Expected Records: %d
Formats: %v
Origins: %v
Schemas: %v`,
		dataset.Name,
		dataset.Description,
		dataset.Metadata.TotalFiles,
		dataset.Metadata.TotalRecords,
		dataset.Metadata.Formats,
		dataset.Metadata.Origins,
		dataset.Metadata.Schemas,
	)
}
