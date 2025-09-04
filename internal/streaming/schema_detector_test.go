package streaming

import (
	"testing"
)

func TestSimpleSchemaDetector(t *testing.T) {
	detector := NewSimpleSchemaDetector(10)

	// Test AWS EC2 instance data
	ec2Data := map[string]interface{}{
		"InstanceId":   "i-1234567890abcdef0",
		"InstanceType": "t2.micro",
		"State": map[string]interface{}{
			"Name": "running",
			"Code": 16,
		},
		"ImageId": "ami-12345678",
		"Tags": []interface{}{
			map[string]interface{}{
				"Key":   "Name",
				"Value": "test-instance",
			},
		},
	}

	chunk := &ProcessedChunk{
		Objects: []interface{}{ec2Data},
		Format:  "json",
		Metadata: map[string]interface{}{
			"source": "aws-api",
		},
	}

	// Add sample
	err := detector.AddSample(chunk)
	if err != nil {
		t.Errorf("Failed to add sample: %v", err)
	}

	// Detect schema
	detection, err := detector.DetectSchema()
	if err != nil {
		t.Errorf("Failed to detect schema: %v", err)
	}

	if detection == nil {
		t.Fatal("No schema detection returned")
	}

	if detection.SchemaName != "aws.ec2-instance" {
		t.Errorf("Expected schema 'aws.ec2-instance', got '%s'", detection.SchemaName)
	}

	if detection.Confidence <= 0 {
		t.Errorf("Expected positive confidence, got %f", detection.Confidence)
	}
}

func TestKubernetesSchemaDetection(t *testing.T) {
	detector := NewSimpleSchemaDetector(10)

	// Test Kubernetes Pod data
	podData := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "test-pod",
			"namespace": "default",
		},
		"spec": map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"name":  "test-container",
					"image": "nginx:latest",
				},
			},
		},
		"status": map[string]interface{}{
			"phase": "Running",
		},
	}

	chunk := &ProcessedChunk{
		Objects: []interface{}{podData},
		Format:  "yaml",
		Metadata: map[string]interface{}{
			"source": "kubectl",
		},
	}

	// Add sample
	err := detector.AddSample(chunk)
	if err != nil {
		t.Errorf("Failed to add sample: %v", err)
	}

	// Detect schema
	detection, err := detector.DetectSchema()
	if err != nil {
		t.Errorf("Failed to detect schema: %v", err)
	}

	if detection == nil {
		t.Fatal("No schema detection returned")
	}

	if detection.SchemaName != "k8s.pod" {
		t.Errorf("Expected schema 'k8s.pod', got '%s'", detection.SchemaName)
	}
}

func TestS3BucketSchemaDetection(t *testing.T) {
	detector := NewSimpleSchemaDetector(10)

	// Test S3 Bucket data
	bucketData := map[string]interface{}{
		"Name":         "my-test-bucket",
		"CreationDate": "2024-01-15T10:30:00Z",
		"Region":       "us-east-1",
		"Tags": []interface{}{
			map[string]interface{}{
				"Key":   "Environment",
				"Value": "test",
			},
		},
	}

	chunk := &ProcessedChunk{
		Objects: []interface{}{bucketData},
		Format:  "json",
		Metadata: map[string]interface{}{
			"source": "aws-s3-api",
		},
	}

	// Add sample
	err := detector.AddSample(chunk)
	if err != nil {
		t.Errorf("Failed to add sample: %v", err)
	}

	// Detect schema
	detection, err := detector.DetectSchema()
	if err != nil {
		t.Errorf("Failed to detect schema: %v", err)
	}

	if detection == nil {
		t.Fatal("No schema detection returned")
	}

	if detection.SchemaName != "aws.s3-bucket" {
		t.Errorf("Expected schema 'aws.s3-bucket', got '%s'", detection.SchemaName)
	}
}

func TestUnknownSchemaDetection(t *testing.T) {
	detector := NewSimpleSchemaDetector(10)

	// Test unknown data structure
	unknownData := map[string]interface{}{
		"randomField1": "value1",
		"randomField2": 42,
		"randomField3": []interface{}{"a", "b", "c"},
	}

	chunk := &ProcessedChunk{
		Objects: []interface{}{unknownData},
		Format:  "json",
		Metadata: map[string]interface{}{
			"source": "unknown",
		},
	}

	// Add sample
	err := detector.AddSample(chunk)
	if err != nil {
		t.Errorf("Failed to add sample: %v", err)
	}

	// Detect schema
	detection, err := detector.DetectSchema()
	if err != nil {
		t.Errorf("Failed to detect schema: %v", err)
	}

	if detection == nil {
		t.Fatal("No schema detection returned")
	}

	if detection.SchemaName != "unknown" {
		t.Errorf("Expected schema 'unknown', got '%s'", detection.SchemaName)
	}

	if detection.Confidence != 0.0 {
		t.Errorf("Expected confidence 0.0 for unknown schema, got %f", detection.Confidence)
	}
}

func TestMultipleSamples(t *testing.T) {
	detector := NewSimpleSchemaDetector(5)

	// Add multiple EC2 instance samples
	for i := 0; i < 3; i++ {
		ec2Data := map[string]interface{}{
			"InstanceId":   "i-" + string(rune('1'+i)) + "234567890abcdef",
			"InstanceType": "t2.micro",
			"State": map[string]interface{}{
				"Name": "running",
			},
		}

		chunk := &ProcessedChunk{
			Objects:  []interface{}{ec2Data},
			Format:   "json",
			Metadata: map[string]interface{}{},
		}

		err := detector.AddSample(chunk)
		if err != nil {
			t.Errorf("Failed to add sample %d: %v", i, err)
		}
	}

	// Check confidence increases with more samples
	confidence := detector.GetConfidence()
	if confidence <= 0 {
		t.Errorf("Expected positive confidence, got %f", confidence)
	}

	// Detect schema
	detection, err := detector.DetectSchema()
	if err != nil {
		t.Errorf("Failed to detect schema: %v", err)
	}

	if detection.SchemaName != "aws.ec2-instance" {
		t.Errorf("Expected schema 'aws.ec2-instance', got '%s'", detection.SchemaName)
	}

	if detection.Samples != 3 {
		t.Errorf("Expected 3 samples, got %d", detection.Samples)
	}
}

func TestCustomPattern(t *testing.T) {
	detector := NewSimpleSchemaDetector(10)

	// Add custom pattern
	customPattern := SchemaPattern{
		Name:        "custom.user",
		Description: "Custom User Schema",
		Fields: []FieldPattern{
			{Name: "id", Type: "integer", Required: true},
			{Name: "username", Type: "string", Required: true},
		},
		Optional: []FieldPattern{
			{Name: "email", Type: "string"},
			{Name: "active", Type: "boolean"},
		},
	}

	detector.AddPattern(customPattern)

	// Test data matching custom pattern
	userData := map[string]interface{}{
		"id":       123,
		"username": "testuser",
		"email":    "test@example.com",
		"active":   true,
	}

	chunk := &ProcessedChunk{
		Objects:  []interface{}{userData},
		Format:   "json",
		Metadata: map[string]interface{}{},
	}

	err := detector.AddSample(chunk)
	if err != nil {
		t.Errorf("Failed to add sample: %v", err)
	}

	detection, err := detector.DetectSchema()
	if err != nil {
		t.Errorf("Failed to detect schema: %v", err)
	}

	if detection.SchemaName != "custom.user" {
		t.Errorf("Expected schema 'custom.user', got '%s'", detection.SchemaName)
	}
}

func TestReset(t *testing.T) {
	detector := NewSimpleSchemaDetector(10)

	// Add sample
	ec2Data := map[string]interface{}{
		"InstanceId": "i-1234567890abcdef0",
		"State": map[string]interface{}{
			"Name": "running",
		},
	}

	chunk := &ProcessedChunk{
		Objects:  []interface{}{ec2Data},
		Format:   "json",
		Metadata: map[string]interface{}{},
	}

	err := detector.AddSample(chunk)
	if err != nil {
		t.Errorf("Failed to add sample: %v", err)
	}

	// Check we have samples
	if detector.GetConfidence() == 0 {
		t.Error("Expected non-zero confidence before reset")
	}

	// Reset
	detector.Reset()

	// Check samples are cleared
	if detector.GetConfidence() != 0 {
		t.Error("Expected zero confidence after reset")
	}

	// Should fail to detect schema with no samples
	_, err = detector.DetectSchema()
	if err == nil {
		t.Error("Expected error when detecting schema with no samples")
	}
}
