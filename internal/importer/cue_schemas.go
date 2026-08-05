package importer

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/chazu/pudl/internal/systemmodel"
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
	if err := fs.WalkDir(bootstrapSchemas, "bootstrap", func(path string, d fs.DirEntry, err error) error {
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
	}); err != nil {
		return err
	}

	// Install the #SystemModel schema (single-sourced from the systemmodel
	// package, which also uses it to load/validate instances) so it shows up in
	// `pudl schema list` alongside the other built-ins.
	smDir := filepath.Join(schemaPath, "pudl", "systemmodel")
	if err := os.MkdirAll(smDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(smDir, "systemmodel.cue"), []byte(systemmodel.SchemaCUE()), 0644)
}

// bootstrapSchemaFiles returns every file that pudl init owns in the schema
// repository. Keep this derived from the embedded tree so adding a built-in
// schema automatically updates both fresh initialization and repair of an
// existing workspace.
func bootstrapSchemaFiles() ([]string, error) {
	var files []string
	err := fs.WalkDir(bootstrapSchemas, "bootstrap", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, path[len("bootstrap/"):])
		return nil
	})
	if err != nil {
		return nil, err
	}
	files = append(files, filepath.Join("pudl", "systemmodel", "systemmodel.cue"))
	return files, nil
}

// BootstrapPackages returns the set of package paths (e.g. "pudl/core") that
// are shipped as built-in bootstrap schemas.
func BootstrapPackages() map[string]bool {
	packages := make(map[string]bool)
	fs.WalkDir(bootstrapSchemas, "bootstrap", func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == "bootstrap" {
			return nil
		}
		rel := path[len("bootstrap/"):]
		packages[rel] = true
		return nil
	})
	// Installed programmatically (single-sourced from the systemmodel package),
	// not from the embedded bootstrap tree — but still a built-in.
	packages["pudl/systemmodel"] = true
	return packages
}

// ensureBasicSchemas verifies that required schema files exist.
// If the schema repository is initialized (cue.mod exists) but bootstrap
// schemas are missing, it copies them automatically. This handles the case
// where new bootstrap schemas are added after the user has already run
// 'pudl init'.
func (e *EnhancedImporter) ensureBasicSchemas() error {
	// Check cue.mod/module.cue exists (required for CUE module loading)
	modulePath := filepath.Join(e.schemaPath, "cue.mod", "module.cue")
	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		return fmt.Errorf("schema repository not initialized: missing %s (run 'pudl init' first)", modulePath)
	}

	required, err := bootstrapSchemaFiles()
	if err != nil {
		return fmt.Errorf("list bootstrap schemas: %w", err)
	}
	for _, relPath := range required {
		checkPath := filepath.Join(e.schemaPath, relPath)
		if _, err := os.Stat(checkPath); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("check bootstrap schema %s: %w", checkPath, err)
			}
			if copyErr := copyBootstrapSchemasTo(e.schemaPath); copyErr != nil {
				return fmt.Errorf("failed to copy bootstrap schemas: %w", copyErr)
			}
			if _, err := os.Stat(checkPath); err != nil {
				return fmt.Errorf("schema repository not initialized: missing %s after repair: %w", checkPath, err)
			}
			break // One copy installs every built-in file.
		}
	}

	return nil
}
