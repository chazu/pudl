package database

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/idgen"
)

// ComputeFactID produces a content-addressed ID for a fact.
// ID = SHA256(relation + "\x00" + canonical_args + "\x00" + valid_start + "\x00" + source)
func ComputeFactID(relation, args string, validStart int64, source string) string {
	canonical := canonicalizeJSON(args)
	payload := fmt.Sprintf("%s\x00%s\x00%d\x00%s", relation, canonical, validStart, source)
	hash := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", hash)
}

// canonicalizeJSON sorts object keys and normalizes number spellings without
// rounding through float64 (see idgen.CanonicalNumber). Invalid/non-object
// input retains the legacy raw representation. Existing persisted IDs are never
// recomputed on open.
func canonicalizeJSON(raw string) string {
	if !json.Valid([]byte(raw)) {
		return raw
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var obj map[string]interface{}
	if err := decoder.Decode(&obj); err != nil {
		return raw
	}
	canonical, err := json.Marshal(idgen.NormalizeJSONNumbers(obj))
	if err != nil {
		return raw
	}
	return string(canonical)
}
