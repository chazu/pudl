package importer

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const deploymentJSON = `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web","namespace":"prod"},"spec":{"replicas":3}}`

func writeGzip(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	zw := gzip.NewWriter(f)
	_, err = zw.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())
}

func TestImport_RepeatedValueRecordsFullSize(t *testing.T) {
	content := `{"kind":"Thing","name":"alpha","blob":"` + strings.Repeat("z", 300000) + `"}`
	imp, source, _ := collectionFixture(t, "big.json", content)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)
	assert.Equal(t, 1, result.RecordCount)
	assert.Equal(t, int64(len(content)), result.SizeBytes)
}

func TestImport_GzipIsDecompressedThroughThePipeline(t *testing.T) {
	imp, plain, _ := collectionFixture(t, "deploy.json", deploymentJSON)
	compressed := filepath.Join(filepath.Dir(plain), "deploy-copy.json.gz")
	writeGzip(t, compressed, deploymentJSON)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: compressed})
	require.NoError(t, err)
	assert.Equal(t, "json", result.DetectedFormat)
	assert.Equal(t, int64(len(deploymentJSON)), result.SizeBytes, "size is the decompressed size")
	assert.Equal(t, "pudl/k8s.#Resource", result.AssignedSchema, "inference sees the decompressed Deployment")
	assert.Equal(t, compressed, result.SourcePath)

	stored, err := os.ReadFile(result.StoredPath)
	require.NoError(t, err)
	assert.Equal(t, deploymentJSON, string(stored), "raw storage holds the decompressed bytes")

	// The same bytes uncompressed are the same content.
	again, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: plain})
	require.NoError(t, err)
	assert.True(t, again.Skipped)

	leftovers, _ := filepath.Glob(filepath.Join(imp.dataPath, "tmp", "*"))
	assert.Empty(t, leftovers, "the decompressed temp copy is removed")
}

func TestImport_ManualSchemaIsHonoredAndValidated(t *testing.T) {
	imp, source, _ := collectionFixture(t, "deploy.json", deploymentJSON)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{
		SourcePath:   source,
		ManualSchema: "pudl/core.#Item",
	})
	require.NoError(t, err)
	assert.Equal(t, "pudl/core.#Item", result.AssignedSchema, "a satisfiable manual schema wins over inference")
	require.NotNil(t, result.ValidationResult)
	assert.True(t, result.ValidationResult.Valid)
}

func TestImport_ManualSchemaFallsBackWhenDataViolatesIt(t *testing.T) {
	imp, source, _ := collectionFixture(t, "widget.json", `{"name":"w1","color":"red"}`)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{
		SourcePath:   source,
		ManualSchema: "pudl/k8s.#Resource",
	})
	require.NoError(t, err)
	require.NotNil(t, result.ValidationResult)
	assert.False(t, result.ValidationResult.Valid)
	assert.NotEqual(t, "pudl/k8s.#Resource", result.AssignedSchema, "data that violates the schema is not assigned to it")
}

func TestImport_ManualSchemaValidatesCollectionItems(t *testing.T) {
	ndjson := deploymentJSON + "\n" + `{"name":"not-a-k8s-object"}` + "\n"
	imp, source, _ := collectionFixture(t, "mixed.ndjson", ndjson)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{
		SourcePath:   source,
		ManualSchema: "pudl/k8s.#Resource",
	})
	require.NoError(t, err)

	items, err := imp.catalogDB.GetCollectionItems(result.ID)
	require.NoError(t, err)
	require.Len(t, items, 2)
	schemas := map[string]bool{}
	for _, item := range items {
		schemas[item.Schema] = true
	}
	assert.True(t, schemas["pudl/k8s.#Resource"], "the conforming item gets the manual schema")
	assert.Len(t, schemas, 2, "the non-conforming item falls back instead of being assigned blindly")
}

func TestImport_JSONArrayCollectionRecordsJSONFormat(t *testing.T) {
	imp, source, _ := collectionFixture(t, "array.json", `[{"a":1},{"a":2}]`)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)
	assert.Equal(t, "json", result.DetectedFormat)

	entry, err := imp.catalogDB.GetEntry(result.ID)
	require.NoError(t, err)
	assert.Equal(t, "json", entry.Format)
}

func TestImport_OriginPathNamesIntermediateSources(t *testing.T) {
	imp, source, _ := collectionFixture(t, ".envelope-123-payload.json", `{"a":1}`)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{
		SourcePath: source,
		OriginPath: "/data/inventory.json",
	})
	require.NoError(t, err)
	assert.Equal(t, "inventory", result.DetectedOrigin)
	assert.Equal(t, "/data/inventory.json", result.SourcePath)
}

func TestImport_DirectoryIsRejectedClearly(t *testing.T) {
	imp, source, _ := collectionFixture(t, "x.json", `{}`)

	_, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: filepath.Dir(source)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a directory")
}
