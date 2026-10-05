package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/validator"
)

// importSession is what every import mode — single file, batch, stdin —
// shares: one importer, and one resolution of --schema. Before, each mode set
// these up separately and only single-file import resolved the schema name.
type importSession struct {
	imp       *importer.EnhancedImporter
	schema    string                    // resolved --schema, or ""
	validator *validator.ChainValidator // set when schema is set
}

// newImportSession loads configuration, opens the importer, and resolves the
// --schema flag.
func newImportSession() (*importSession, error) {
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return nil, errors.NewConfigError("Failed to load configuration", err)
	}

	imp, err := importer.NewEnhancedImporterWithSchemaPaths(cfg.DataPath, effectivePudlDir(), effectiveSchemaPaths(cfg)...)
	if err != nil {
		if os.Getenv("PUDL_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "DEBUG: Enhanced importer error: %+v\n", err)
		}
		return nil, errors.NewSystemError("Failed to initialize enhanced importer", err)
	}

	schema, chain, err := resolveImportSchema(cfg, importSchema)
	if err != nil {
		imp.Close()
		return nil, err
	}
	return &importSession{imp: imp, schema: schema, validator: chain}, nil
}

// Close releases the importer.
func (s *importSession) Close() {
	s.imp.Close()
}

// options builds the import options for one source file.
func (s *importSession) options(path, origin string) importer.ImportOptions {
	return importer.ImportOptions{
		SourcePath:     path,
		Origin:         origin, // auto-detected from the path when empty
		ManualSchema:   s.schema,
		ChainValidator: s.validator,
	}
}

// resolveImportSchema resolves a --schema value to a loaded schema name and
// returns the validator that will check records against it.
//
// A name the validator cannot resolve is an error, with one exception: a
// module-versioned CUE reference (e.g. "mu/aws@v1#EC2Instance") names a schema
// that may only be resolvable later via `pudl reclassify`, so it is kept as
// given and recorded unverified.
func resolveImportSchema(cfg *config.Config, input string) (string, *validator.ChainValidator, error) {
	if input == "" {
		return "", nil, nil
	}
	chain, err := validator.NewChainValidator(effectiveSchemaPaths(cfg)...)
	if err != nil {
		return "", nil, errors.WrapError(errors.ErrCodeValidationFailed, "Failed to create chain validator", err)
	}
	resolved, err := chain.ResolveSchemaName(input)
	if err != nil {
		if strings.Contains(input, "@") {
			return input, chain, nil
		}
		return "", nil, errors.NewSchemaNotFoundError(input, nil)
	}
	return resolved, chain, nil
}

// workspaceImportOrigin is the origin file imports default to: the explicit
// --origin, else the workspace's origin when inside one, else "" (detect from
// the file name).
func workspaceImportOrigin() string {
	if importOrigin == "" && wsPolicy.InWorkspace() {
		return wsPolicy.EffectiveOrigin
	}
	return importOrigin
}
