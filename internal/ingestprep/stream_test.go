package ingestprep

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArrayFramingAndLimits(t *testing.T) {
	for _, source := range []string{`[]`, `[1,true,null,"a\\\"b",{},[2,3]]`, `[ {"a":["]",{"b":"}"}]} ]`} {
		var expected []json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(source), &expected))
		var actual []json.RawMessage
		count, err := Array(strings.NewReader(source), 1024, func(_ int, r json.RawMessage) error { actual = append(actual, r); return nil })
		require.NoError(t, err)
		require.Equal(t, len(expected), count)
		for i := range actual {
			require.JSONEq(t, string(expected[i]), string(actual[i]))
		}
	}
	for _, source := range []string{`[1,]`, `[1] 2`, `[1`, `["a]`, `[{]`, `[1 2]`, `[,1]`, `[[1,]]`, `[truefalse]`} {
		_, err := Array(strings.NewReader(source), 1024, func(int, json.RawMessage) error { return nil })
		require.Error(t, err, source)
	}
	_, err := Array(strings.NewReader(`["123456789"]`), 5, func(int, json.RawMessage) error { return nil })
	require.ErrorContains(t, err, "record byte limit")
	_, err = NDJSON(strings.NewReader(strings.Repeat(" ", 100)+"\n"), 10, func(int, json.RawMessage) error { return nil })
	require.ErrorContains(t, err, "record byte limit")
}

func TestReaderLimitsAreErrorsAndCancellationSurvives(t *testing.T) {
	_, err := io.ReadAll(&Reader{Source: strings.NewReader("abcd"), Remaining: 3})
	require.ErrorContains(t, err, "byte limit")
	bytes, err := io.ReadAll(&Reader{Source: strings.NewReader("abc"), Remaining: 3})
	require.NoError(t, err)
	require.Equal(t, "abc", string(bytes))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = io.ReadAll(&Reader{Context: ctx, Source: strings.NewReader("abc"), Remaining: 3})
	require.ErrorIs(t, err, context.Canceled)
}

func FuzzArrayMatchesJSON(f *testing.F) {
	for _, seed := range []string{`[]`, `[1,true,null,"x",{},[2]]`, `[{"x":"\\\"[]"}]`, `[1,]`, `[0]false`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 8192 {
			return
		}
		var expected []json.RawMessage
		valid := json.Unmarshal([]byte(source), &expected) == nil && strings.HasPrefix(strings.TrimSpace(source), "[")
		var actual []json.RawMessage
		_, err := Array(strings.NewReader(source), 8192, func(_ int, r json.RawMessage) error { actual = append(actual, r); return nil })
		if valid {
			require.NoError(t, err)
			require.Len(t, actual, len(expected))
			for i := range actual {
				require.JSONEq(t, string(expected[i]), string(actual[i]))
			}
		} else {
			require.Error(t, err)
		}
	})
}
