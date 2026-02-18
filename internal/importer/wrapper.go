package importer

import (
	"strings"
)

// WrapperDetection holds the result of collection wrapper detection.
type WrapperDetection struct {
	ArrayKey    string                 // Key name of the array field (e.g., "items")
	Items       []interface{}          // Extracted array elements
	WrapperMeta map[string]interface{} // Non-array sibling fields (pagination, count, etc.)
	Score       float64                // Confidence score (0.0–1.0)
	Signals     []string               // Human-readable list of signals that contributed
}

// KnownWrapperKeys are field names commonly used for the main collection array
// in API responses. Matched case-insensitively.
var KnownWrapperKeys = []string{
	"items", "data", "results", "records", "entries", "objects",
	"resources", "hits", "values", "elements", "list", "rows",
	"nodes", "edges",
}

// KnownAttributeKeys are field names that typically hold attribute arrays on a
// resource rather than a collection of independent items. Matched case-insensitively.
var KnownAttributeKeys = []string{
	"tags", "labels", "permissions", "roles", "addresses", "emails",
	"groups", "scopes", "features", "capabilities", "attachments",
	"dependencies", "headers", "cookies", "args", "arguments",
	"env", "environment", "ports", "volumes", "rules", "conditions",
}

// PaginationKeys are field names associated with pagination or collection
// metadata. Matched exactly (covers common casing variants).
var PaginationKeys = []string{
	"next_token", "nextToken", "NextToken",
	"cursor", "next_cursor", "nextCursor",
	"continuation_token", "ContinuationToken",
	"marker", "NextMarker", "next_marker",
	"page", "page_size", "pageSize", "per_page", "perPage",
	"offset", "limit",
	"total", "totalCount", "total_count", "count", "Count",
	"has_more", "hasMore", "HasMore",
	"_links", "links", "next", "previous",
	"meta", "metadata", "_metadata",
}

const wrapperScoreThreshold = 0.50

// DetectCollectionWrapper analyzes a JSON object and determines if it's a
// collection wrapper (e.g., {"items": [...], "count": 2}).
// Returns nil if the data is not a wrapper or the score is below threshold.
func DetectCollectionWrapper(data map[string]interface{}) *WrapperDetection {
	var best *WrapperDetection

	for key, val := range data {
		arr, ok := val.([]interface{})
		if !ok || len(arr) < 1 {
			continue
		}
		// First element must be a map (object).
		if _, isMap := arr[0].(map[string]interface{}); !isMap {
			continue
		}

		score, signals := scoreCandidate(key, arr, data)
		if score < wrapperScoreThreshold {
			continue
		}

		if best == nil || score > best.Score {
			best = &WrapperDetection{
				ArrayKey:    key,
				Items:       arr,
				WrapperMeta: extractWrapperMeta(data, key),
				Score:       score,
				Signals:     signals,
			}
		}
	}

	return best
}

// scoreCandidate computes a confidence score for a single array field candidate.
func scoreCandidate(key string, arr []interface{}, allFields map[string]interface{}) (float64, []string) {
	var score float64
	var signals []string

	// +0.35 known wrapper key
	if containsFold(KnownWrapperKeys, key) {
		score += 0.35
		signals = append(signals, "known wrapper key: "+key)
	}

	// +0.25 pagination siblings
	if hasPaginationSiblings(allFields, key) {
		score += 0.25
		signals = append(signals, "pagination siblings present")
	}

	// +0.20 count matches length
	if countMatchesArrayLength(allFields, len(arr), key) {
		score += 0.20
		signals = append(signals, "count matches array length")
	}

	// +0.15 homogeneous elements
	if isHomogeneous(arr, 0.80) {
		score += 0.15
		signals = append(signals, "homogeneous elements (≥80%)")
	}

	// +0.05 few top-level keys
	if len(allFields) <= 5 {
		score += 0.05
		signals = append(signals, "few top-level keys (≤5)")
	}

	// +0.05 dominant array
	if isDominantArray(key, arr, allFields) {
		score += 0.05
		signals = append(signals, "dominant array by size")
	}

	// -0.30 known attribute key
	if containsFold(KnownAttributeKeys, key) {
		score -= 0.30
		signals = append(signals, "known attribute key: "+key+" (penalty)")
	}

	// -0.40 multiple similar arrays
	if hasMultipleSimilarArrays(allFields, key) {
		score -= 0.40
		signals = append(signals, "multiple similar arrays (penalty)")
	}

	// -0.15 many scalar fields
	if hasManyScalarFields(allFields, key) {
		score -= 0.15
		signals = append(signals, "many non-pagination scalar fields (penalty)")
	}

	return score, signals
}

// hasPaginationSiblings checks whether any sibling key matches a known
// pagination key.
func hasPaginationSiblings(fields map[string]interface{}, excludeKey string) bool {
	for k := range fields {
		if k == excludeKey {
			continue
		}
		for _, pk := range PaginationKeys {
			if k == pk {
				return true
			}
		}
	}
	return false
}

// countMatchesArrayLength checks if any numeric sibling field equals the array
// length.
func countMatchesArrayLength(fields map[string]interface{}, arrLen int, excludeKey string) bool {
	for k, v := range fields {
		if k == excludeKey {
			continue
		}
		switch n := v.(type) {
		case float64:
			if int(n) == arrLen {
				return true
			}
		case int:
			if n == arrLen {
				return true
			}
		}
	}
	return false
}

// isHomogeneous checks if at least threshold fraction of elements share the
// same set of top-level keys.
func isHomogeneous(arr []interface{}, threshold float64) bool {
	if len(arr) == 0 {
		return false
	}

	// Build a signature for each element: sorted key names joined.
	type sig = string
	counts := make(map[sig]int)
	for _, elem := range arr {
		m, ok := elem.(map[string]interface{})
		if !ok {
			continue
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		// Simple sort for deterministic signature.
		sortStrings(keys)
		s := strings.Join(keys, ",")
		counts[s]++
	}

	// Find the most common signature.
	maxCount := 0
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}

	return float64(maxCount)/float64(len(arr)) >= threshold
}

// isDominantArray estimates whether the candidate array is the largest field
// by element count.
func isDominantArray(key string, arr []interface{}, allFields map[string]interface{}) bool {
	for k, v := range allFields {
		if k == key {
			continue
		}
		if otherArr, ok := v.([]interface{}); ok && len(otherArr) >= len(arr) {
			return false
		}
	}
	return true
}

// hasMultipleSimilarArrays checks if there are ≥2 array fields (besides
// the candidate) with at least 1 object element.
func hasMultipleSimilarArrays(allFields map[string]interface{}, excludeKey string) bool {
	count := 0
	for k, v := range allFields {
		if k == excludeKey {
			continue
		}
		if arr, ok := v.([]interface{}); ok && len(arr) >= 1 {
			if _, isMap := arr[0].(map[string]interface{}); isMap {
				count++
			}
		}
	}
	return count >= 2
}

// hasManyScalarFields checks if there are more than 6 non-pagination scalar
// fields among siblings.
func hasManyScalarFields(allFields map[string]interface{}, excludeKey string) bool {
	paginationSet := make(map[string]bool, len(PaginationKeys))
	for _, pk := range PaginationKeys {
		paginationSet[pk] = true
	}

	scalarCount := 0
	for k, v := range allFields {
		if k == excludeKey {
			continue
		}
		if paginationSet[k] {
			continue
		}
		// Skip arrays and maps — count only scalars.
		switch v.(type) {
		case []interface{}, map[string]interface{}:
			continue
		}
		scalarCount++
	}
	return scalarCount > 6
}

// extractWrapperMeta returns all fields except the identified array key.
func extractWrapperMeta(fields map[string]interface{}, arrayKey string) map[string]interface{} {
	meta := make(map[string]interface{}, len(fields)-1)
	for k, v := range fields {
		if k == arrayKey {
			continue
		}
		meta[k] = v
	}
	return meta
}

// containsFold checks if needle exists in haystack using case-insensitive
// comparison.
func containsFold(haystack []string, needle string) bool {
	for _, h := range haystack {
		if strings.EqualFold(h, needle) {
			return true
		}
	}
	return false
}

// sortStrings sorts a slice of strings in place (simple insertion sort —
// fine for the small key sets we deal with).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
