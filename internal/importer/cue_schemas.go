package importer

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed bootstrap
var bootstrapSchemas embed.FS

// CopyBootstrapSchemas copies the embedded bootstrap CUE schemas to the given schema directory.
// This is used by pudl init to populate the schema repository with required base schemas.
func CopyBootstrapSchemas(schemaPath string) error {
	return copyBootstrapSchemasTo(schemaPath)
}

// copyBootstrapSchemasTo copies bootstrap CUE schema files to the specified directory
func copyBootstrapSchemasTo(schemaPath string) error {
	// Walk the embedded bootstrap schemas and copy them to the schema path
	return fs.WalkDir(bootstrapSchemas, "bootstrap", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the bootstrap root directory itself
		if path == "bootstrap" {
			return nil
		}

		// Convert embedded path to target path
		// Embedded path: bootstrap/pudl/core/core.cue
		// We want: <schemaPath>/pudl/core/core.cue
		relPath := path[len("bootstrap/"):] // Remove "bootstrap/" prefix

		targetPath := filepath.Join(schemaPath, relPath)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		// Read embedded file
		content, err := bootstrapSchemas.ReadFile(path)
		if err != nil {
			return err
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		// Write to target
		return os.WriteFile(targetPath, content, 0644)
	})
}

// ensureBasicSchemas verifies that required schema files exist.
// If the schema repository is initialized (cue.mod exists) but bootstrap
// schemas are missing, it copies them automatically. This handles the case
// where new bootstrap schemas are added after the user has already run
// 'pudl init'.
func (i *Importer) ensureBasicSchemas() error {
	// Check cue.mod/module.cue exists (required for CUE module loading)
	modulePath := filepath.Join(i.schemaPath, "cue.mod", "module.cue")
	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		return fmt.Errorf("schema repository not initialized: missing %s (run 'pudl init' first)", modulePath)
	}

	// Check if core schema exists; if not, copy bootstrap schemas to fill gaps
	corePath := filepath.Join(i.schemaPath, "pudl", "core", "core.cue")
	if _, err := os.Stat(corePath); os.IsNotExist(err) {
		if copyErr := copyBootstrapSchemasTo(i.schemaPath); copyErr != nil {
			return fmt.Errorf("failed to copy bootstrap schemas: %w", copyErr)
		}
		// Verify the copy succeeded
		if _, err := os.Stat(corePath); os.IsNotExist(err) {
			return fmt.Errorf("schema repository not initialized: missing %s (run 'pudl init' first)", corePath)
		}
	}
	return nil
}
