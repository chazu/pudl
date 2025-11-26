package importer

import (
	"embed"
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
		// Embedded path: bootstrap/pudl/unknown/catchall.cue
		// We want: <schemaPath>/pudl/unknown/catchall.cue
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

// createBasicSchemas copies bootstrap CUE schema files to the schema directory
func (i *Importer) createBasicSchemas() error {
	return copyBootstrapSchemasTo(i.schemaPath)
}

// ensureBasicSchemas ensures that basic schema files exist
func (i *Importer) ensureBasicSchemas() error {
	// Ensure cue.mod/module.cue exists (required for CUE module loading)
	modulePath := filepath.Join(i.schemaPath, "cue.mod", "module.cue")
	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		if err := i.createCUEModule(); err != nil {
			return err
		}
	}

	// Check if catchall schema exists
	catchallPath := filepath.Join(i.schemaPath, "pudl", "unknown", "catchall.cue")
	if _, err := os.Stat(catchallPath); os.IsNotExist(err) {
		return i.createBasicSchemas()
	}
	return nil
}

// createCUEModule creates the cue.mod/module.cue file
func (i *Importer) createCUEModule() error {
	cueModDir := filepath.Join(i.schemaPath, "cue.mod")
	if err := os.MkdirAll(cueModDir, 0755); err != nil {
		return err
	}

	moduleContent := `language: version: "v0.14.0"
module: "pudl.schemas@v0"
source: kind: "self"
`
	return os.WriteFile(filepath.Join(cueModDir, "module.cue"), []byte(moduleContent), 0644)
}
