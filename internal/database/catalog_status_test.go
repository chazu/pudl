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
		ImportTimestamp: time.Now(),
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

	// Add an entry with a target
	entry := CatalogEntry{
		ID:              "status-valid-001",
		StoredPath:      "/test/data.json",
		MetadataPath:    "/test/data.meta",
		ImportTimestamp: time.Now(),
		Format:          "json",
		Origin:          "test",
		Schema:          "test.#App",
		Confidence:      1.0,
		RecordCount:     1,
		SizeBytes:       50,
		Target:          defNamePtr,
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
			Schema: "pudl/core.#Item", Target: &d, EntryType: &et,
		}))
		require.NoError(t, db.UpdateStatus(def, status))
	}
	mk("a", "web", "converging")   // this model, pending
	mk("b", "api", "drifted")      // this model, not converging -> untouched
	mk("c", "other", "converging") // another model -> must NOT be promoted

	n, err := db.PromoteConvergingToClean([]string{"web", "api", "absent"}, "mymodel")
	require.NoError(t, err)
	assert.Equal(t, 1, n, "only the converging in-scope def is promoted")

	statuses, err := db.GetTargetStatuses()
	require.NoError(t, err)
	got := map[string]string{}
	for _, s := range statuses {
		got[s.Target] = s.Status
	}
	assert.Equal(t, "clean", got["web"], "converging in-scope -> clean")
	assert.Equal(t, "drifted", got["api"], "non-converging untouched")
	assert.Equal(t, "converging", got["other"], "out-of-scope model untouched")
}

// Two models declaring a resource with the same identity name share one target
// key, so the name alone cannot say whose pending row it is. The tag can, when
// there is one: a row tagged to another model must survive this model's clean
// drift, while this model's own tagged rows and untagged rows still promote.
func TestPromoteConvergingToClean_SkipsRowsTaggedToAnotherModel(t *testing.T) {
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
			Schema: "pudl/core.#Item", Target: &d, EntryType: &et, Tags: &tags,
		}))
		require.NoError(t, db.UpdateStatus(def, status))
	}
	// The defect: `nginx` is declared by both models. Model A's drift going clean
	// used to promote model B's row because they key on the same target name.
	mk("a", "nginx", "modelB", "converging")
	mk("b", "cache", "modelA", "converging")
	mk("c", "untagged-db", "", "converging")

	// A row with no tags column at all — json_extract over NULL must yield NULL
	// rather than excluding the row, or the fallback stops promoting the very
	// rows it exists for.
	nullTagDef := "no-tags-queue"
	nullTagEntryType := "manifest-action"
	require.NoError(t, db.AddEntry(CatalogEntry{
		ID: "d", StoredPath: "d.json", MetadataPath: "d.meta",
		ImportTimestamp: time.Now(), Format: "json", Origin: "t",
		Schema: "pudl/core.#Item", Target: &nullTagDef, EntryType: &nullTagEntryType,
	}))
	require.NoError(t, db.UpdateStatus(nullTagDef, "converging"))

	// modelA's promotion, with `nginx` in its candidate defs (both models declare it).
	n, err := db.PromoteConvergingToClean([]string{"nginx", "cache", "untagged-db", nullTagDef}, "modelA")
	require.NoError(t, err)
	assert.Equal(t, 3, n, "modelA's own row and both untagged rows promote; modelB's does not")

	got := map[string]string{}
	statuses, err := db.GetTargetStatuses()
	require.NoError(t, err)
	for _, s := range statuses {
		got[s.Target] = s.Status
	}
	assert.Equal(t, "converging", got["nginx"], "another model's pending row is not promoted by this model's clean drift")
	assert.Equal(t, "clean", got["cache"], "this model's own tagged row promotes")
	assert.Equal(t, "clean", got["untagged-db"], "an untagged row still promotes — the fallback exists for it")
	assert.Equal(t, "clean", got[nullTagDef], "a row with a NULL tags column still promotes")
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
			Schema: "pudl/core.#Item", Target: &d, EntryType: &et, Tags: &tags,
		}))
		require.NoError(t, db.UpdateStatus(def, status))
	}
	mk("a", "Deployment/web", "mymodel", "converging")  // this model, pending
	mk("b", "Service/api", "mymodel", "drifted")        // this model, not converging -> untouched
	mk("c", "Deployment/x", "othermodel", "converging") // another model -> must NOT promote
	mk("d", "untagged", "", "converging")               // untagged -> must NOT promote by model

	n, err := db.PromoteConvergingToCleanByModel("mymodel")
	require.NoError(t, err)
	assert.Equal(t, 1, n, "only mymodel's converging row promotes")

	got := map[string]string{}
	statuses, err := db.GetTargetStatuses()
	require.NoError(t, err)
	for _, s := range statuses {
		got[s.Target] = s.Status
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

func TestGetTargetStatuses(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	entryType := "artifact"

	// Create entries for 3 targets with different statuses
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
			ImportTimestamp: time.Now().Add(time.Duration(i) * time.Second),
			Format:          "json",
			Origin:          "test",
			Schema:          "test.#App",
			Confidence:      1.0,
			RecordCount:     1,
			SizeBytes:       50,
			Target:          &defName,
			EntryType:       &entryType,
		}
		require.NoError(t, db.AddEntry(entry))
		require.NoError(t, db.UpdateStatus(d.name, d.status))
	}

	statuses, err := db.GetTargetStatuses()
	require.NoError(t, err)
	require.Len(t, statuses, 3)

	// Verify each target has the correct status (ordered by target name)
	statusMap := make(map[string]string)
	for _, s := range statuses {
		statusMap[s.Target] = s.Status
	}

	assert.Equal(t, "clean", statusMap["app_a"])
	assert.Equal(t, "drifted", statusMap["app_b"])
	assert.Equal(t, "converging", statusMap["app_c"])
}
