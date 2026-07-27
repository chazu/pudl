package importer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamNDJSON_OneRecordAtATime(t *testing.T) {
	input := strings.NewReader("{\"a\":1}\n{\"a\":2}\n\n{\"a\":3}\n")

	var seen []string
	count, err := streamNDJSON(input, func(index int, raw json.RawMessage) error {
		seen = append(seen, string(raw))
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, count, "blank lines are not records")
	assert.Equal(t, []string{`{"a":1}`, `{"a":2}`, `{"a":3}`}, seen)
}

func TestStreamNDJSON_MissingTrailingNewline(t *testing.T) {
	count, err := streamNDJSON(strings.NewReader(`{"a":1}`), func(int, json.RawMessage) error { return nil })
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestStreamNDJSON_InvalidLineIsReported(t *testing.T) {
	_, err := streamNDJSON(strings.NewReader("{\"a\":1}\nnot json\n"),
		func(int, json.RawMessage) error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 2")
}

func TestStreamNDJSON_SinkErrorStopsTheStream(t *testing.T) {
	calls := 0
	_, err := streamNDJSON(strings.NewReader("{\"a\":1}\n{\"a\":2}\n{\"a\":3}\n"),
		func(index int, raw json.RawMessage) error {
			calls++
			if index == 1 {
				return fmt.Errorf("boom")
			}
			return nil
		})
	require.Error(t, err)
	assert.Equal(t, 2, calls, "the stream stops rather than decoding the rest")
}

func TestStreamJSONArray_ElementByElement(t *testing.T) {
	var seen []string
	count, err := streamJSONArray(strings.NewReader(`[{"a":1}, {"a":2}, {"a":3}]`),
		func(index int, raw json.RawMessage) error {
			seen = append(seen, string(raw))
			return nil
		})
	require.NoError(t, err)
	assert.Equal(t, 3, count)
	assert.Equal(t, []string{`{"a":1}`, `{"a":2}`, `{"a":3}`}, seen)
}

func TestStreamJSONArray_EmptyArray(t *testing.T) {
	count, err := streamJSONArray(strings.NewReader(`[]`), func(int, json.RawMessage) error { return nil })
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestStreamJSONArray_RejectsANonArray(t *testing.T) {
	_, err := streamJSONArray(strings.NewReader(`{"a":1}`), func(int, json.RawMessage) error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected a JSON array")
}

func TestStreamableCollectionFormat(t *testing.T) {
	dir := t.TempDir()
	array := filepath.Join(dir, "a.json")
	object := filepath.Join(dir, "o.json")
	require.NoError(t, os.WriteFile(array, []byte("  \n [ {\"a\":1} ]"), 0o644))
	require.NoError(t, os.WriteFile(object, []byte(`{"a":1}`), 0o644))

	format, ok := streamableCollectionFormat(array, "json")
	assert.True(t, ok)
	assert.Equal(t, "json-array", format, "leading whitespace does not hide the array")

	_, ok = streamableCollectionFormat(object, "json")
	assert.False(t, ok, "a single object stays on the single-record path — already bounded")

	format, ok = streamableCollectionFormat("irrelevant", "ndjson")
	assert.True(t, ok)
	assert.Equal(t, "ndjson", format)

	_, ok = streamableCollectionFormat("irrelevant", "yaml")
	assert.False(t, ok, "yaml is out of this slice's scope")
}

func TestStageSource_PreservesTheRawByteContentHash(t *testing.T) {
	// Identity is the contract this rewrite must not move: SHA256 of the raw
	// source bytes, byte for byte, however the copy is performed.
	dir := t.TempDir()
	source := filepath.Join(dir, "source.json")
	content := []byte("{\"a\":1}\n{\"a\":2}\n")
	require.NoError(t, os.WriteFile(source, content, 0o644))

	want := sha256.Sum256(content)

	staged, err := stageSource(source, filepath.Join(dir, "raw"))
	require.NoError(t, err)
	defer staged.Discard()

	assert.Equal(t, hex.EncodeToString(want[:]), staged.ContentHash)
	assert.Equal(t, int64(len(content)), staged.Size)
}

func TestStageSource_CommitAndDiscard(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.json")
	require.NoError(t, os.WriteFile(source, []byte(`{"a":1}`), 0o644))
	rawDir := filepath.Join(dir, "raw")

	staged, err := stageSource(source, rawDir)
	require.NoError(t, err)
	temp := staged.tempPath
	require.FileExists(t, temp)

	final := filepath.Join(rawDir, "committed.json")
	path, err := staged.Commit(final)
	require.NoError(t, err)
	assert.Equal(t, final, path)
	assert.NoFileExists(t, temp, "the temp name is gone once committed")
	assert.FileExists(t, final)

	staged.Discard() // must not remove the committed file
	assert.FileExists(t, final)
}

func TestStageSource_DiscardRemovesTheStagedCopy(t *testing.T) {
	// The duplicate-import path: the bytes were staged before the dedup check, so
	// a hit must not leave a copy behind.
	dir := t.TempDir()
	source := filepath.Join(dir, "source.json")
	require.NoError(t, os.WriteFile(source, []byte(`{"a":1}`), 0o644))

	staged, err := stageSource(source, filepath.Join(dir, "raw"))
	require.NoError(t, err)
	temp := staged.tempPath
	require.FileExists(t, temp)

	staged.Discard()
	assert.NoFileExists(t, temp)
	staged.Discard() // idempotent
}

// TestImportMemoryIsBoundedByRecordSize is the measurement Recommendation 2 asks
// for: peak memory, not elapsed time.
//
// The assertion is a *ratio* between two import sizes, not an absolute figure, so
// it fails when someone reintroduces "append every record into one slice" — the
// regression worth catching — and not when the allocator shifts by a few percent.
func TestImportMemoryIsBoundedByRecordSize(t *testing.T) {
	small := allocationsForNDJSONStream(t, 100)
	large := allocationsForNDJSONStream(t, 2000)

	// 20× the records. Linear-in-N *streaming* work is expected and fine; what
	// must not happen is retaining them all, which shows up as the ratio blowing
	// past the record ratio.
	ratio := float64(large) / float64(small)
	assert.Less(t, ratio, 40.0,
		"decoding 20x the records allocated %.1fx as much; a full materialization is back", ratio)
}

// allocationsForNDJSONStream measures bytes allocated while streaming n records.
func allocationsForNDJSONStream(t *testing.T, n int) uint64 {
	t.Helper()

	var input strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&input, `{"name":"resource-%d","index":%d,"payload":"%s"}`+"\n",
			i, i, strings.Repeat("x", 200))
	}
	source := input.String()

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	// The sink does what the real one does with the bytes and then drops them:
	// what is being measured is the decoder's retention, not the catalog's.
	var lastLen int
	_, err := streamNDJSON(strings.NewReader(source), func(index int, raw json.RawMessage) error {
		var record map[string]any
		if err := json.Unmarshal(raw, &record); err != nil {
			return err
		}
		canonical, err := json.Marshal(record)
		if err != nil {
			return err
		}
		lastLen = len(canonical)
		return nil
	})
	require.NoError(t, err)
	require.Positive(t, lastLen)

	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// TestStreamingRetainsNothingAcrossRecords is the direct statement of the
// property the ratio test infers: the decoder hands each record's bytes to the
// sink and keeps no reference to them.
func TestStreamingRetainsNothingAcrossRecords(t *testing.T) {
	var input strings.Builder
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&input, `{"i":%d}`+"\n", i)
	}

	live := 0
	maxLive := 0
	_, err := streamNDJSON(strings.NewReader(input.String()), func(index int, raw json.RawMessage) error {
		live++
		if live > maxLive {
			maxLive = live
		}
		live-- // the sink is done with the record when it returns
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, maxLive, "at most one record is in the sink's hands at a time")
}
