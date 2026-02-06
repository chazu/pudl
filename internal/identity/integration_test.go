package identity

import (
	"testing"
)

// TestIdentityFlow_ImportSameFileTwice verifies that importing the same content
// produces the same resource_id and content_hash.
func TestIdentityFlow_ImportSameFileTwice(t *testing.T) {
	schema := "aws/ec2.#Instance"
	contentHash := "abc123def456789012345678901234567890123456789012345678901234abcd"
	identityFields := []string{"id", "region"}

	data := map[string]interface{}{
		"id":     "i-1234567890abcdef0",
		"region": "us-east-1",
		"state":  "running",
	}

	// First import
	values1, err := ExtractFieldValues(data, identityFields)
	if err != nil {
		t.Fatalf("extract1 failed: %v", err)
	}
	resourceID1 := ComputeResourceID(schema, values1, contentHash)

	// Second import (same data)
	values2, err := ExtractFieldValues(data, identityFields)
	if err != nil {
		t.Fatalf("extract2 failed: %v", err)
	}
	resourceID2 := ComputeResourceID(schema, values2, contentHash)

	// Same content → same resource_id
	if resourceID1 != resourceID2 {
		t.Error("same content should produce same resource_id")
	}
}

// TestIdentityFlow_ModifiedContent verifies that importing modified content
// with the same identity fields produces the same resource_id (different version).
func TestIdentityFlow_ModifiedContent(t *testing.T) {
	schema := "aws/ec2.#Instance"
	identityFields := []string{"id", "region"}

	// Version 1
	data1 := map[string]interface{}{
		"id":     "i-1234567890abcdef0",
		"region": "us-east-1",
		"state":  "running",
	}
	contentHash1 := "aaa0000000000000000000000000000000000000000000000000000000000001"

	values1, _ := ExtractFieldValues(data1, identityFields)
	resourceID1 := ComputeResourceID(schema, values1, contentHash1)

	// Version 2 — same identity fields, different content
	data2 := map[string]interface{}{
		"id":     "i-1234567890abcdef0",
		"region": "us-east-1",
		"state":  "stopped", // changed
	}
	contentHash2 := "bbb0000000000000000000000000000000000000000000000000000000000002"

	values2, _ := ExtractFieldValues(data2, identityFields)
	resourceID2 := ComputeResourceID(schema, values2, contentHash2)

	// Same identity → same resource_id (different version in the catalog)
	if resourceID1 != resourceID2 {
		t.Error("same identity fields should produce same resource_id regardless of content")
	}
}

// TestIdentityFlow_CatchallSchema verifies that catchall schemas use content hash as identity.
func TestIdentityFlow_CatchallSchema(t *testing.T) {
	schema := "pudl/core.#Item"
	// No identity fields for catchall

	contentHash1 := "ccc0000000000000000000000000000000000000000000000000000000000001"
	contentHash2 := "ddd0000000000000000000000000000000000000000000000000000000000002"

	resourceID1 := ComputeResourceID(schema, nil, contentHash1)
	resourceID2 := ComputeResourceID(schema, nil, contentHash2)

	// Different content → different resource_id (catchall entries are effectively immutable)
	if resourceID1 == resourceID2 {
		t.Error("catchall with different content should produce different resource_id")
	}
}

// TestIdentityFlow_SchemaChangeRecomputesIdentity verifies that changing the schema
// produces a different resource_id (reinfer scenario).
func TestIdentityFlow_SchemaChangeRecomputesIdentity(t *testing.T) {
	contentHash := "eee0000000000000000000000000000000000000000000000000000000000001"
	identityFields := []string{"id"}

	data := map[string]interface{}{
		"id":   "test-123",
		"name": "test resource",
	}

	values, _ := ExtractFieldValues(data, identityFields)

	// Original schema
	resourceID1 := ComputeResourceID("pudl/core.#Item", values, contentHash)

	// Reinferred to more specific schema
	resourceID2 := ComputeResourceID("aws/ec2.#Instance", values, contentHash)

	// Different schema → different resource_id
	if resourceID1 == resourceID2 {
		t.Error("different schemas should produce different resource_id")
	}
}
