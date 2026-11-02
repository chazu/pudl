package secretscan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/chazu/pudl/internal/redact"
	"github.com/chazu/pudl/internal/secrets"
)

// The detector skips redacted values by their placeholder; the two packages
// must agree on it.
func TestRedactedPlaceholderMatches(t *testing.T) {
	assert.Equal(t, redact.Placeholder, secrets.RedactedValue)
}

func TestScanWithoutSchemaSource(t *testing.T) {
	s := New(nil, nil)
	record := map[string]any{"jwt": "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.c2lnbmF0dXJlLXZhbHVl"}
	assert.Equal(t, []secrets.Finding{{Path: "jwt", Kind: secrets.JWT}}, s.Scan(record, "any.#Schema"))
}
