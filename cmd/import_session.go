package cmd

import (
	"context"
	"fmt"
	"github.com/chazu/pudl/internal/ingestprep"
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
	ctx       context.Context
	limits    ingestprep.Limits
	imp       *importer.EnhancedImporter
	schema    string                    // resolved --schema, or ""
	validator *validator.ChainValidator // set when schema is set
	set       []importer.FieldAssignment
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
			fmt.Fprintf(errw(), "DEBUG: Enhanced importer error: %+v\n", err)
		}
		return nil, errors.NewSystemError("Failed to initialize enhanced importer", err)
	}

	schema, chain, err := resolveImportSchema(cfg, importSchema)
	if err != nil {
		imp.Close()
		return nil, err
	}
	set, err := parseCLIAssignments("set", importSet)
	if err != nil {
		imp.Close()
		return nil, errors.NewInputError(err.Error())
	}
	return &importSession{imp: imp, schema: schema, validator: chain, set: set}, nil
}

// Close releases the importer.
func (s *importSession) Close() {
	s.imp.Close()
}

// options builds the import options for one source file.
func (s *importSession) options(path, origin string) importer.ImportOptions {
	return importer.ImportOptions{
		SourcePath:          path,
		Context:             s.ctx,
		Limits:              s.limits,
		Explain:             importExplain,
		Origin:              origin, // auto-detected from the path when empty
		ManualSchema:        s.schema,
		AllowSchemaFallback: importAllowSchemaFallback,
		ChainValidator:      s.validator,
		Set:                 s.set,
		DryRun:              importDryRun,
	}
}

// resolveImportSchema resolves a --schema value to a loaded schema name and
// returns the validator that will check records against it.
//
// A name the validator cannot resolve is an error, with one exception: a
// module-versioned CUE reference can be retained unverified only when the
// caller explicitly selected --allow-schema-fallback.
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
		if strings.Contains(input, "@") && importAllowSchemaFallback {
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

// finish runs after a session's imports. New and re-observed records were
// projected as they were committed; this catches up everything else — a facts
// block added since the data was imported, entries reassigned — so facts
// match the catalog when the command returns. A dry run writes nothing.
func (s *importSession) finish() {
	if importDryRun {
		return
	}
	syncProjectionsQuietly(s.ctx, s.imp.CatalogDB())
}
