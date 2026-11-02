package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/validator"
	"github.com/chazu/pudl/internal/workspace"
)

const reviewedSchemas = `package storage
#Bucket: {
	_pudl: {
		schema_type:   "base"
		resource_type: "storage.bucket"
		review: {status: "validated", by: "sre-platform", at: "2026-11-01"}
	}
	name: string
}
#Disk: {
	_pudl: {schema_type: "base", resource_type: "storage.disk"}
	name: string
}
`

func reviewWorkspace(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".pudl", "schema")
	for path, content := range map[string]string{
		"cue.mod/module.cue":  "module: \"pudl.schemas@v0\"\nlanguage: version: \"v0.14.0\"\n",
		"storage/storage.cue": reviewedSchemas,
	} {
		full := filepath.Join(root, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	previous := wsPolicy
	wsPolicy = &workspace.Policy{SchemaSearchPaths: []string{root}, ModelSearchPaths: []string{root}}
	t.Cleanup(func() { wsPolicy = previous })
	validator.ResetSharedLoaders()
	t.Cleanup(validator.ResetSharedLoaders)
	previousJSON, previousPackage := jsonOutput, schemaPackage
	t.Cleanup(func() { jsonOutput, schemaPackage = previousJSON, previousPackage })
	schemaPackage = ""
}

func TestSchemaListShowsReviewStatus(t *testing.T) {
	reviewWorkspace(t)
	jsonOutput = false
	output := captureQueryOutput(t, runSchemaListCommand)
	assert.Contains(t, output, "storage.#Bucket  type=base resource=storage.bucket review=validated by sre-platform at 2026-11-01")
	assert.Contains(t, output, "storage.#Disk  type=base resource=storage.disk review=draft")

	jsonOutput = true
	output = captureQueryOutput(t, runSchemaListCommand)
	var entries []struct {
		FullName string `json:"full_name"`
		Metadata struct {
			Review validator.SchemaReview `json:"review"`
		} `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &entries))
	reviews := map[string]validator.SchemaReview{}
	for _, e := range entries {
		reviews[e.FullName] = e.Metadata.Review
	}
	assert.Equal(t, validator.SchemaReview{Status: "validated", By: "sre-platform", At: "2026-11-01"}, reviews["storage.#Bucket"])
	assert.Equal(t, validator.SchemaReview{Status: "draft"}, reviews["storage.#Disk"])
}

func TestSchemaShowShowsReviewStatus(t *testing.T) {
	reviewWorkspace(t)
	jsonOutput = false
	output := captureQueryOutput(t, func() error { return runSchemaShowCommand("storage.#Bucket") })
	assert.Contains(t, output, "Review: validated by sre-platform at 2026-11-01")
	output = captureQueryOutput(t, func() error { return runSchemaShowCommand("storage.#Disk") })
	assert.Contains(t, output, "Review: draft")

	jsonOutput = true
	output = captureQueryOutput(t, func() error { return runSchemaShowCommand("storage.#Bucket") })
	var shown struct {
		Review validator.SchemaReview `json:"review"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &shown))
	assert.Equal(t, "validated", shown.Review.Status)
}
