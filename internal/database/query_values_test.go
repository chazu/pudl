package database

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueryParameterUsesActualGoFloatValue(t *testing.T) {
	// This binary64 value is an exact integer, but JSON's shortest decimal
	// spelling names another integer. Both operands must keep their own value.
	f := math.Nextafter(0x1p63, 0)
	encoded, err := json.Marshal(f)
	require.NoError(t, err)
	fromFloat, err := QueryParameter(f)
	require.NoError(t, err)
	require.Equal(t, int64(f), fromFloat)
	fromJSON, err := QueryParameter(json.Number(encoded))
	require.NoError(t, err)
	require.NotEqual(t, fromJSON, fromFloat)
}

func TestQueryNumberNormalizesAtNumericBoundaries(t *testing.T) {
	for _, tc := range []struct {
		text string
		want interface{}
	}{
		{"-0.000e999999999999999999", int64(0)},
		{"9.223372036854775807e18", int64(math.MaxInt64)},
		{"-9.223372036854775808e18", int64(math.MinInt64)},
		{"5e-324", math.SmallestNonzeroFloat64},
		{"0.1", float64(0.1)},
	} {
		got, err := QueryNumber(json.Number(tc.text))
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
}
