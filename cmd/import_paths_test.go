package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeImportFixture(t *testing.T, root, rel, content string) string {
	t.Helper()
	path := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestResolveFilePathsExpandsDirectories(t *testing.T) {
	root := t.TempDir()
	a := writeImportFixture(t, root, "a.json", `{"a":1}`)
	b := writeImportFixture(t, root, "b.yaml", "b: 1\n")
	c := writeImportFixture(t, root, "c.csv.gz", "")
	writeImportFixture(t, root, "notes.txt", "ignored")
	writeImportFixture(t, root, ".hidden.json", `{}`)
	nested := writeImportFixture(t, root, "sub/nested.ndjson", "{}\n")
	writeImportFixture(t, root, ".pudl/data/x.json", `{}`)

	paths, err := resolveFilePaths(root, false)
	require.NoError(t, err)
	require.Equal(t, []string{a, b, c}, paths, "non-recursive lists only supported top-level files")

	paths, err = resolveFilePaths(root, true)
	require.NoError(t, err)
	require.Equal(t, []string{a, b, c, nested}, paths, "recursive descends but skips hidden entries")

	empty := t.TempDir()
	paths, err = resolveFilePaths(empty, true)
	require.NoError(t, err)
	require.Empty(t, paths)
}

func TestImportJSONReportsEachFileOutcome(t *testing.T) {
	cliWorkspace(t)
	dir := t.TempDir()
	writeImportFixture(t, dir, "one.json", `{"kind":"Widget","name":"one"}`)
	writeImportFixture(t, dir, "two.json", `{"kind":"Widget","name":"two"}`)

	r := runCLI(t, "import", "--path", dir, "--json")
	require.NoError(t, r.Err, r.Stderr)
	var first []importOutcome
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &first), r.Stdout)
	require.Len(t, first, 2)
	for _, outcome := range first {
		require.Equal(t, importStatusImported, outcome.Status)
		require.NotEmpty(t, outcome.ID)
	}
	require.Contains(t, r.Stderr, "Importing 2 files", "batch progress belongs on stderr")

	// Re-importing reports duplicates as skipped, still one entry per file.
	r = runCLI(t, "import", "--path", filepath.Join(dir, "*.json"), "--json")
	require.NoError(t, r.Err, r.Stderr)
	var second []importOutcome
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &second), r.Stdout)
	require.Len(t, second, 2)
	for _, outcome := range second {
		require.Equal(t, importStatusSkipped, outcome.Status)
		require.True(t, outcome.Skipped)
	}

	// A file that cannot be imported is reported as failed and fails the command.
	bad := writeImportFixture(t, dir, "bad.json", `{"unterminated":`)
	r = runCLI(t, "import", "--path", bad, "--json")
	require.Error(t, r.Err)
	var failed []importOutcome
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &failed), r.Stdout)
	require.Len(t, failed, 1)
	require.Equal(t, importStatusFailed, failed[0].Status)
	require.Equal(t, bad, failed[0].SourcePath)
	require.NotEmpty(t, failed[0].Error)
}

func TestListLimitCapsTotalResults(t *testing.T) {
	cliWorkspace(t)
	dir := t.TempDir()
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		writeImportFixture(t, dir, name+".json", `{"kind":"Widget","name":"`+name+`"}`)
	}
	require.NoError(t, runCLI(t, "import", "--path", dir).Err)

	list := func(args ...string) (entries []map[string]any, totalEntries, totalMatched int) {
		t.Helper()
		r := runCLI(t, append([]string{"list", "--json"}, args...)...)
		require.NoError(t, r.Err, r.Stderr)
		var out struct {
			Entries      []map[string]any `json:"entries"`
			TotalEntries int              `json:"total_entries"`
			TotalMatched int              `json:"total_matched"`
		}
		require.NoError(t, json.Unmarshal([]byte(r.Stdout), &out), r.Stdout)
		return out.Entries, out.TotalEntries, out.TotalMatched
	}

	entries, total, matched := list()
	require.Len(t, entries, 5)
	require.Equal(t, 5, total)
	require.Equal(t, 5, matched)

	entries, total, matched = list("--limit", "3")
	require.Len(t, entries, 3, "--limit caps results even with the default page size")
	require.Equal(t, 3, total)
	require.Equal(t, 5, matched)

	entries, total, _ = list("--limit", "3", "--per-page", "2", "--page", "2")
	require.Len(t, entries, 1, "the last page stops at the limit")
	require.Equal(t, 3, total)

	entries, _, _ = list("--limit", "3", "--per-page", "2", "--page", "3")
	require.Empty(t, entries, "pages past the limit are empty")
}
