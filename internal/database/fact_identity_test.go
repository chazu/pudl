package database

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFactIDRetainsUnroundedLegacyNumbers(t *testing.T) {
	for _, raw := range []string{
		`{"n":1.0}`, `{"n":-0}`, `{"n":1e-7}`, `{"n":1e-6}`,
		`{"n":1e20}`, `{"n":1e21}`, `{"n":1.25}`, `{"n":0.1}`,
		`{"n":9007199254740992}`, `{"nested":[1e3,{"n":-0.125}]}`,
	} {
		// Compatibility oracle: the previous ID algorithm on values for which
		// its float64 round trip did not change the JSON number's decimal value.
		var value map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(raw), &value))
		canonical, err := json.Marshal(value)
		require.NoError(t, err)
		legacy := sha256.Sum256([]byte(fmt.Sprintf("r\x00%s\x001\x00scanner", canonical)))
		require.Equal(t, fmt.Sprintf("%x", legacy), ComputeFactID("r", raw, 1, "scanner"), raw)
	}
}

func TestFactIDHandlesLargeExponentsWithoutExpansion(t *testing.T) {
	first := ComputeFactID("r", `{"n":1e100000000000000000000}`, 1, "s")
	equivalent := ComputeFactID("r", `{"n":10e99999999999999999999}`, 1, "s")
	different := ComputeFactID("r", `{"n":1e100000000000000000001}`, 1, "s")
	require.Equal(t, first, equivalent)
	require.NotEqual(t, first, different)
}
