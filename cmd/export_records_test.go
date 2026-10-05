package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/stretchr/testify/require"
)

func exportFixture(t *testing.T, format, content string) (*os.File, int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	spool, err := os.CreateTemp(t.TempDir(), "spool")
	require.NoError(t, err)
	t.Cleanup(func() { spool.Close() })
	count := 0
	require.NoError(t, walkExportFile(context.Background(), path, format, true, func(v any) error { count++; return jsonEncodeExport(spool, v) }))
	return spool, count
}

func jsonEncodeExport(w io.Writer, v any) error { return json.NewEncoder(w).Encode(v) }

func TestExportFormatsAndExactNumbers(t *testing.T) {
	for _, tc := range []struct{ format, input, want string }{
		{"json", `[{"serial":9007199254740993},{"serial":9007199254740994}]`, "9007199254740993"},
		{"json", `9007199254740993`, "9007199254740993"},
		{"csv", "z,a\n007,plain\n008,next\n", `"007"`},
		{"yaml", "name: one\n---\nname: two\n", `"two"`},
		{"yaml", "serial: 900719925474099312345\nfraction: 0.1234567890123456789\n", "900719925474099312345"},
		{"ndjson", "{\"n\":1}\n{\"n\":2}\n", `"n":2`},
	} {
		t.Run(tc.format+tc.want, func(t *testing.T) {
			spool, count := exportFixture(t, tc.format, tc.input)
			var out bytes.Buffer
			require.NoError(t, writeExportSpool(&out, spool, count, "json", false))
			require.Contains(t, out.String(), tc.want)
		})
	}
	spool, count := exportFixture(t, "json", `{"n":9007199254740993,"f":1e30}`)
	var yaml bytes.Buffer
	require.NoError(t, writeExportSpool(&yaml, spool, count, "yaml", false))
	require.Contains(t, yaml.String(), "n: 9007199254740993")
	require.NotContains(t, yaml.String(), `n: "`)
}

type failedExportWriter struct{}

func (failedExportWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
func TestExportCSVDeterministicFlushAndShapes(t *testing.T) {
	spool, count := exportFixture(t, "json", `[{"z":2,"a":1},{"b":3}]`)
	var out bytes.Buffer
	require.NoError(t, writeExportSpool(&out, spool, count, "csv", false))
	require.Equal(t, "a,b,z\n1,,2\n,3,\n", out.String())
	require.ErrorContains(t, writeExportSpool(failedExportWriter{}, spool, count, "csv", false), "write failed")
	scalar, n := exportFixture(t, "json", `42`)
	require.ErrorContains(t, writeExportSpool(io.Discard, scalar, n, "csv", false), "object records")
	nested, n := exportFixture(t, "json", `{"nested":{"a":1}}`)
	require.ErrorContains(t, writeExportSpool(io.Discard, nested, n, "csv", false), "nested value")
}
func TestPublishExportPreservesDestinationOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.json")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0640))
	require.Error(t, publishExport(path, func(w io.Writer) error { io.WriteString(w, "prefix"); return errors.New("failure") }))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "original", string(data))
	require.Error(t, validateExportFormat("invalid"))
	require.NoError(t, publishExport(path, func(w io.Writer) error { _, err := io.WriteString(w, "complete"); return err }))
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "complete", string(data))
}
func TestExportMembershipAndPartialInput(t *testing.T) {
	dir := t.TempDir()
	db, err := database.NewCatalogDB(dir)
	require.NoError(t, err)
	defer db.Close()
	collectionType, itemType := "collection", "item"
	index := 0
	coll := database.CatalogEntry{ID: "collection", Schema: "pudl/core.#Collection", Format: "json", Origin: "test", ImportTimestamp: time.Now(), CollectionType: &collectionType, StoredPath: "nonexistent-provenance"}
	require.NoError(t, db.AddEntry(coll))
	path := filepath.Join(dir, "record.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"serial":9007199254740993}`), 0600))
	item := database.CatalogEntry{ID: "item", Schema: "pudl/core.#Item", Format: "json", Origin: "test", ImportTimestamp: time.Now(), StoredPath: path, CollectionType: &itemType, CollectionID: &coll.ID, ItemIndex: &index}
	require.NoError(t, db.AddEntry(item))
	spool, count, omitted, err := prepareExport(context.Background(), db, []database.CatalogEntry{coll, item}, false)
	require.NoError(t, err)
	defer os.Remove(spool.Name())
	defer spool.Close()
	require.Equal(t, 1, count)
	require.Zero(t, omitted)
	badPath := filepath.Join(dir, "bad.ndjson")
	require.NoError(t, os.WriteFile(badPath, []byte("{\"prefix\":1}\ninvalid\n"), 0600))
	bad := database.CatalogEntry{ID: "bad", Format: "ndjson", StoredPath: badPath}
	_, _, _, err = prepareExport(context.Background(), db, []database.CatalogEntry{item, bad}, false)
	require.Error(t, err)
	partial, n, omitted, err := prepareExport(context.Background(), db, []database.CatalogEntry{item, bad}, true)
	require.NoError(t, err)
	defer os.Remove(partial.Name())
	defer partial.Close()
	require.Equal(t, 1, n)
	require.Equal(t, 1, omitted)
}

func TestExportCommandFailureAndPartialPublication(t *testing.T) {
	root := consolidationWorkspace(t)
	oldID, oldSchema, oldOrigin, oldFormat, oldOutput, oldPartial := exportID, exportSchema, exportOrigin, exportFormat, exportOutput, exportAllowPartial
	t.Cleanup(func() {
		exportID, exportSchema, exportOrigin, exportFormat, exportOutput, exportAllowPartial = oldID, oldSchema, oldOrigin, oldFormat, oldOutput, oldPartial
	})
	exportID, exportSchema, exportOrigin, exportFormat, exportAllowPartial = "", "", "regression", "json", false
	exportOutput = filepath.Join(t.TempDir(), "result.json")
	require.NoError(t, os.WriteFile(exportOutput, []byte("existing destination"), 0600))
	db, err := database.NewCatalogDB(root)
	require.NoError(t, err)
	readable := filepath.Join(root, "readable.json")
	require.NoError(t, os.WriteFile(readable, []byte(`{"serial":9007199254740993}`), 0600))
	for _, entry := range []database.CatalogEntry{{ID: "readable", StoredPath: readable}, {ID: "missing", StoredPath: "does-not-exist"}} {
		entry.Schema = "pudl/core.#Item"
		entry.Format = "json"
		entry.Origin = exportOrigin
		entry.ImportTimestamp = time.Now()
		require.NoError(t, db.AddEntry(entry))
	}
	require.NoError(t, db.Close())
	exportFormat = "invalid"
	require.ErrorContains(t, runExportCommand(exportCmd, nil), "unknown export format")
	data, err := os.ReadFile(exportOutput)
	require.NoError(t, err)
	require.Equal(t, "existing destination", string(data))
	exportFormat = "json"
	require.ErrorContains(t, runExportCommand(exportCmd, nil), "export entry")
	data, err = os.ReadFile(exportOutput)
	require.NoError(t, err)
	require.Equal(t, "existing destination", string(data))
	exportAllowPartial = true
	require.ErrorContains(t, runExportCommand(exportCmd, nil), "incomplete export")
	data, err = os.ReadFile(exportOutput)
	require.NoError(t, err)
	require.JSONEq(t, `{"serial":9007199254740993}`, string(data))
}

func TestExportYAMLAliasesMergeAndExactDecimals(t *testing.T) {
	spool, count := exportFixture(t, "yaml", `base: &base
  serial: 900719925474099312345
  fraction: 0.1234567890123456789
copy:
  <<: *base
  serial: 900719925474099312346
`)
	var out bytes.Buffer
	require.NoError(t, writeExportSpool(&out, spool, count, "json", false))
	require.Contains(t, out.String(), "900719925474099312345")
	require.Contains(t, out.String(), "900719925474099312346")
	require.Contains(t, out.String(), "0.1234567890123456789")
}
