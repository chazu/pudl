package database

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureStatusColumn(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pudl-test-status-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create database and add an entry
	db, err := NewCatalogDB(tmpDir)
	require.NoError(t, err)

	entry := CatalogEntry{
		ID:              "status-test-001",
		StoredPath:      filepath.Join(tmpDir, "test.json"),
		MetadataPath:    filepath.Join(tmpDir, "test.meta"),
		ImportTimestamp:  time.Now(),
		Format:          "json",
		Origin:          "test",
		Schema:          "pudl/core.#Item",
		Confidence:      0.5,
		RecordCount:     1,
		SizeBytes:       100,
	}
	require.NoError(t, db.AddEntry(entry))
	db.Close()

	// Re-open database — migration should run
	db2, err := NewCatalogDB(tmpDir)
	require.NoError(t, err)
	defer db2.Close()

	// Verify status column exists
	exists, err := db2.columnExists("catalog_entries", "status")
	require.NoError(t, err)
	assert.True(t, exists, "status column should exist after migration")

	// Verify default value is "unknown"
	retrieved, err := db2.GetEntry(entry.ID)
	require.NoError(t, err)
	require.NotNil(t, retrieved.Status)
	assert.Equal(t, "unknown", *retrieved.Status)
}

func TestUpdateStatus_Valid(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	defName := "my_app"
	defNamePtr := &defName
	entryType := "artifact"

	// Add an entry with a definition
	entry := CatalogEntry{
		ID:              "status-valid-001",
		StoredPath:      "/test/data.json",
		MetadataPath:    "/test/data.meta",
		ImportTimestamp:  time.Now(),
		Format:          "json",
		Origin:          "test",
		Schema:          "test.#App",
		Confidence:      1.0,
		RecordCount:     1,
		SizeBytes:       50,
		Definition:      defNamePtr,
		EntryType:       &entryType,
	}
	require.NoError(t, db.AddEntry(entry))

	// Test each valid status value
	validStatuses := []string{"unknown", "clean", "drifted", "converging", "failed"}
	for _, status := range validStatuses {
		err := db.UpdateStatus(defName, status)
		require.NoError(t, err, "UpdateStatus should succeed for %q", status)

		// Verify it stuck
		retrieved, err := db.GetEntry(entry.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved.Status)
		assert.Equal(t, status, *retrieved.Status, "status should be %q after update", status)
	}
}

func TestPromoteConvergingToClean(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	mk := func(id, def, status string) {
		d := def
		et := "manifest-action"
		require.NoError(t, db.AddEntry(CatalogEntry{
			ID: id, StoredPath: id + ".json", MetadataPath: id + ".meta",
			ImportTimestamp: time.Now(), Format: "json", Origin: "t",
			Schema: "pudl/core.#Item", Definition: &d, EntryType: &et,
		}))
		require.NoError(t, db.UpdateStatus(def, status))
	}
	mk("a", "web", "converging") // this model, pending
	mk("b", "api", "drifted")    // this model, not converging -> untouched
	mk("c", "other", "converging") // another model -> must NOT be promoted

	n, err := db.PromoteConvergingToClean([]string{"web", "api", "absent"})
	require.NoError(t, err)
	assert.Equal(t, 1, n, "only the converging in-scope def is promoted")

	statuses, err := db.GetDefinitionStatuses()
	require.NoError(t, err)
	got := map[string]string{}
	for _, s := range statuses {
		got[s.Definition] = s.Status
	}
	assert.Equal(t, "clean", got["web"], "converging in-scope -> clean")
	assert.Equal(t, "drifted", got["api"], "non-converging untouched")
	assert.Equal(t, "converging", got["other"], "out-of-scope model untouched")
}

func TestPromoteConvergingToCleanByModel(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	mk := func(id, def, model, status string) {
		d := def
		et := "manifest-action"
		tags := fmt.Sprintf(`{"exit_code":0,"model":%q}`, model)
		if model == "" {
			tags = `{"exit_code":0}`
		}
		require.NoError(t, db.AddEntry(CatalogEntry{
			ID: id, StoredPath: id + ".json", MetadataPath: id + ".meta",
			ImportTimestamp: time.Now(), Format: "json", Origin: "t",
			Schema: "pudl/core.#Item", Definition: &d, EntryType: &et, Tags: &tags,
		}))
		require.NoError(t, db.UpdateStatus(def, status))
	}
	mk("a", "Deployment/web", "mymodel", "converging") // this model, pending
	mk("b", "Service/api", "mymodel", "drifted")       // this model, not converging -> untouched
	mk("c", "Deployment/x", "othermodel", "converging") // another model -> must NOT promote
	mk("d", "untagged", "", "converging")               // untagged -> must NOT promote by model

	n, err := db.PromoteConvergingToCleanByModel("mymodel")
	require.NoError(t, err)
	assert.Equal(t, 1, n, "only mymodel's converging row promotes")

	got := map[string]string{}
	statuses, err := db.GetDefinitionStatuses()
	require.NoError(t, err)
	for _, s := range statuses {
		got[s.Definition] = s.Status
	}
	assert.Equal(t, "clean", got["Deployment/web"], "tagged converging -> clean (k8s Kind/name target)")
	assert.Equal(t, "drifted", got["Service/api"], "non-converging untouched")
	assert.Equal(t, "converging", got["Deployment/x"], "other model untouched")
	assert.Equal(t, "converging", got["untagged"], "untagged untouched by model promote")

	// empty model is a no-op
	n, err = db.PromoteConvergingToCleanByModel("")
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestUpdateStatus_Invalid(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	err := db.UpdateStatus("some_def", "bogus")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
}

func TestGetDefinitionStatuses(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	entryType := "artifact"

	// Create entries for 3 definitions with different statuses
	defs := []struct {
		name   string
		status string
	}{
		{"app_a", "clean"},
		{"app_b", "drifted"},
		{"app_c", "converging"},
	}

	for i, d := range defs {
		defName := d.name
		entry := CatalogEntry{
			ID:              fmt.Sprintf("def-status-%03d", i),
			StoredPath:      fmt.Sprintf("/test/%s.json", d.name),
			MetadataPath:    fmt.Sprintf("/test/%s.meta", d.name),
			ImportTimestamp:  time.Now().Add(time.Duration(i) * time.Second),
			Format:          "json",
			Origin:          "test",
			Schema:          "test.#App",
			Confidence:      1.0,
			RecordCount:     1,
			SizeBytes:       50,
			Definition:      &defName,
			EntryType:       &entryType,
		}
		require.NoError(t, db.AddEntry(entry))
		require.NoError(t, db.UpdateStatus(d.name, d.status))
	}

	statuses, err := db.GetDefinitionStatuses()
	require.NoError(t, err)
	require.Len(t, statuses, 3)

	// Verify each definition has the correct status (ordered by definition name)
	statusMap := make(map[string]string)
	for _, s := range statuses {
		statusMap[s.Definition] = s.Status
	}

	assert.Equal(t, "clean", statusMap["app_a"])
	assert.Equal(t, "drifted", statusMap["app_b"])
	assert.Equal(t, "converging", statusMap["app_c"])
}
