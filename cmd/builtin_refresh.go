package cmd

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/chazu/pudl/internal/importer"
)

// refreshBuiltinSchemas brings the invocation's built-in schemas up to this
// pudl's before any command reads them. pudl owns those files. When only
// import refreshed them, read commands (graph vocab, doctor, ui, query,
// check) ran on an older pudl's built-ins and reported what was missing from
// them as problems in the user's own files.
//
// A failed refresh never fails the command: it warns on errOut, and the command runs on
// the files the workspace has.
func refreshBuiltinSchemas(errOut io.Writer) {
	schemaPath := effectiveSchemaPath(nil)
	if schemaPath == "" {
		return
	}
	refreshed, err := importer.RefreshBootstrapSchemas(schemaPath)
	switch {
	case err != nil:
		fmt.Fprintf(errOut, "⚠️  built-in schemas not updated: %v\n", err)
	case refreshed:
		fmt.Fprintf(errOut, "Updated pudl's built-in schemas in %s\n", filepath.Join(schemaPath, "pudl"))
	}
}
