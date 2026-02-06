package identity

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"pudl/internal/schemaname"
)

// ComputeResourceID returns a deterministic resource_id.
// For schemas with identity_fields: SHA256(normalized_schema + "\x00" + canonical_json(values))
// For catchall (empty identityValues): SHA256(normalized_schema + "\x00" + contentHash)
func ComputeResourceID(schema string, identityValues map[string]interface{}, contentHash string) string {
	normalized := schemaname.Normalize(schema)

	var identityComponent string
	if len(identityValues) == 0 {
		// Catchall: use content hash as the identity component
		identityComponent = contentHash
	} else {
		// Has identity fields: use canonical JSON of values
		canonical, err := CanonicalIdentityJSON(identityValues)
		if err != nil {
			// Fallback to content hash if canonical JSON fails
			identityComponent = contentHash
		} else {
			identityComponent = canonical
		}
	}

	input := normalized + "\x00" + identityComponent
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash)
}

// CanonicalIdentityJSON produces deterministic JSON from identity values.
// Keys are sorted alphabetically for determinism.
func CanonicalIdentityJSON(values map[string]interface{}) (string, error) {
	if len(values) == 0 {
		return "{}", nil
	}

	// Sort keys for determinism
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build ordered map for marshaling
	ordered := make([]keyValue, len(keys))
	for i, k := range keys {
		ordered[i] = keyValue{Key: k, Value: values[k]}
	}

	// Marshal with sorted keys using a custom approach
	result := "{"
	for i, kv := range ordered {
		if i > 0 {
			result += ","
		}
		keyJSON, err := json.Marshal(kv.Key)
		if err != nil {
			return "", fmt.Errorf("failed to marshal key %q: %w", kv.Key, err)
		}
		valJSON, err := json.Marshal(kv.Value)
		if err != nil {
			return "", fmt.Errorf("failed to marshal value for key %q: %w", kv.Key, err)
		}
		result += string(keyJSON) + ":" + string(valJSON)
	}
	result += "}"

	return result, nil
}

type keyValue struct {
	Key   string
	Value interface{}
}
