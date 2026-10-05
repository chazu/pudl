package acute

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func observedRecords(recs ...map[string]any) []ObservedRecord {
	out := make([]ObservedRecord, 0, len(recs))
	for _, r := range recs {
		out = append(out, ObservedRecord{Data: r})
	}
	return out
}

func TestInventorySetDiff(t *testing.T) {
	observed := observedRecords(
		map[string]any{"_schema": "pudl/linux.#Package", "name": "podman", "state": "present"},
		map[string]any{"_schema": "pudl/linux.#Package", "name": "restic", "state": "present"},
	)
	desired := []map[string]any{
		{"_schema": "pudl/linux.#Package", "name": "podman", "state": "present"}, // satisfied
		{"_schema": "pudl/linux.#Package", "name": "htop", "state": "present"},   // missing
		{"_schema": "pudl/linux.#Package", "name": "restic", "state": "absent"},  // changed
	}
	drift := InventorySetDiff(desired, observed, nil) // nil resolver -> name|path|id fallback
	require.Len(t, drift, 2)

	assert.Equal(t, DriftMissing, drift[0].Reason)
	assert.Contains(t, drift[0].Resource, "htop")
	assert.Equal(t, DriftChanged, drift[1].Reason)
	assert.Contains(t, drift[1].Resource, "restic")
	assert.Equal(t, "state: present → want absent", drift[1].Diff)
	assert.Equal(t, []FieldDiff{{Path: "state", Expected: "absent", Observed: "present"}}, drift[1].Fields)
}

// schema-driven identity: match on declared identity_fields (composite), not the
// name|path|id fallback (these records carry none of those).
func TestInventorySetDiff_SchemaDrivenIdentity(t *testing.T) {
	identity := func(schema string) []string {
		if schema == "pudl/artifact.#ImageRef" {
			return []string{"source", "tag"}
		}
		return nil
	}
	observed := observedRecords(
		map[string]any{"_schema": "pudl/artifact.#ImageRef", "source": "ghcr.io/o/i", "tag": "v1", "digest": "sha256:aaa"},
	)
	desired := []map[string]any{
		{"_schema": "pudl/artifact.#ImageRef", "source": "ghcr.io/o/i", "tag": "v1", "digest": "sha256:bbb"},
		{"_schema": "pudl/artifact.#ImageRef", "source": "ghcr.io/o/i", "tag": "v2", "digest": "sha256:aaa"},
	}
	drift := InventorySetDiff(desired, observed, identity)
	require.Len(t, drift, 2)
	assert.Equal(t, DriftChanged, drift[0].Reason)
	assert.Contains(t, drift[0].Diff, "digest")
	assert.Equal(t, DriftMissing, drift[1].Reason)
	assert.Contains(t, drift[1].Resource, "v2")
}

func TestInventorySetDiff_AllSatisfied(t *testing.T) {
	recs := []map[string]any{{"_schema": "s", "name": "a", "x": "1"}}
	assert.Empty(t, InventorySetDiff(recs, observedRecords(recs...), nil))
}

func TestInventorySetDiff_ExtrasIgnored(t *testing.T) {
	observed := observedRecords(map[string]any{"_schema": "s", "name": "a"}, map[string]any{"_schema": "s", "name": "extra"})
	desired := []map[string]any{{"_schema": "s", "name": "a"}}
	assert.Empty(t, InventorySetDiff(desired, observed, nil))
}

// A desired set whose records carry no identity used to be skipped record by
// record, so the model reported clean having compared nothing.
func TestInventorySetDiff_UnidentifiableIsNotClean(t *testing.T) {
	desired := []map[string]any{
		{"_schema": "s", "state": "present"},
		{"_schema": "s", "state": "absent"},
	}
	observed := observedRecords(map[string]any{"_schema": "s", "state": "present"})
	drift := InventorySetDiff(desired, observed, nil)
	require.Len(t, drift, 2)
	for i, d := range drift {
		assert.Equal(t, DriftUnidentifiable, d.Reason)
		assert.Equal(t, []string{"s/desired[0]", "s/desired[1]"}[i], d.Resource)
		assert.NotEmpty(t, d.Diff)
	}
}

func TestInventorySetDiff_DuplicateObservedIdentityIsAmbiguous(t *testing.T) {
	early := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	late := early.Add(time.Hour)
	observed := []ObservedRecord{
		{Data: map[string]any{"_schema": "s", "name": "a", "state": "present"}, ObservedAt: &early},
		{Data: map[string]any{"_schema": "s", "name": "a", "state": "absent"}, ObservedAt: &late},
	}
	desired := []map[string]any{{"_schema": "s", "name": "a", "state": "present"}}
	drift := InventorySetDiff(desired, observed, nil)
	require.Len(t, drift, 1)
	assert.Equal(t, DriftAmbiguous, drift[0].Reason)
	require.NotNil(t, drift[0].ObservedAt)
	assert.True(t, drift[0].ObservedAt.Equal(late))
}

// The same record observed twice is one resource, not an ambiguity.
func TestInventorySetDiff_IdenticalDuplicatesAreOneRecord(t *testing.T) {
	rec := map[string]any{"_schema": "s", "name": "a", "n": float64(1)}
	same := map[string]any{"_schema": "s", "name": "a", "n": json.Number("1")}
	desired := []map[string]any{{"_schema": "s", "name": "a", "n": int64(1)}}
	assert.Empty(t, InventorySetDiff(desired, observedRecords(rec, same), nil))
}

func TestInventorySetDiff_AllFieldsInPathOrder(t *testing.T) {
	desired := []map[string]any{{"_schema": "s", "name": "a", "zeta": "z", "alpha": "a", "mid": "m"}}
	observed := observedRecords(map[string]any{"_schema": "s", "name": "a", "zeta": "Z", "mid": "M"})
	for range 20 { // map iteration order must not leak into the report
		drift := InventorySetDiff(desired, observed, nil)
		require.Len(t, drift, 1)
		require.Equal(t, []FieldDiff{
			{Path: "alpha", Expected: "a", Missing: true},
			{Path: "mid", Expected: "m", Observed: "M"},
			{Path: "zeta", Expected: "z", Observed: "Z"},
		}, drift[0].Fields)
		assert.Equal(t, "alpha missing (want a); mid: M → want m; zeta: Z → want z", drift[0].Diff)
	}
}

func TestInventorySetDiff_ObservedAtCarried(t *testing.T) {
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	observed := []ObservedRecord{{Data: map[string]any{"_schema": "s", "name": "a", "x": "old"}, ObservedAt: &at}}
	drift := InventorySetDiff([]map[string]any{{"_schema": "s", "name": "a", "x": "new"}}, observed, nil)
	require.Len(t, drift, 1)
	require.NotNil(t, drift[0].ObservedAt)
	assert.True(t, drift[0].ObservedAt.Equal(at))
}
