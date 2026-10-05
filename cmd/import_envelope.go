package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/mubridge"
	"github.com/chazu/pudl/internal/muschemas"
)

// recordItemSchemas writes the item_schemas rows associated with an
// import. The inferred schema (whatever pudl assigned) is always
// recorded with status=inferred. If the import came from an envelope,
// the declared CUE ref is also recorded with one of:
//
//   - declared        — (module, version) already in pudl's schema cache
//   - auto_registered — envelope carried inline definitions, written to cache
//   - unresolved      — declared without definitions and not cached;
//     tag for `pudl reclassify`
//
// It writes through the importer's open catalog rather than opening another
// connection per imported file.
func recordItemSchemas(db *database.CatalogDB, result *importer.ImportResult, env *unwrappedEnvelope) error {
	if result == nil || result.ID == "" {
		return nil
	}

	if result.AssignedSchema != "" {
		if err := db.AddItemSchema(database.ItemSchema{
			ItemID:    result.ID,
			SchemaRef: result.AssignedSchema,
			Status:    database.ItemSchemaStatusInferred,
		}); err != nil {
			return fmt.Errorf("record inferred: %w", err)
		}
	}
	if env == nil {
		return nil
	}
	cache, err := muschemas.New(SchemaCacheRoot())
	if err != nil {
		return fmt.Errorf("open schema cache: %w", err)
	}
	status, err := classifyEnvelopeSchema(cache, env.envelope)
	if err != nil {
		return fmt.Errorf("classify envelope: %w", err)
	}
	if err := db.AddItemSchema(database.ItemSchema{
		ItemID:    result.ID,
		SchemaRef: env.envelope.Schema.CanonicalRef(),
		Status:    status,
	}); err != nil {
		return fmt.Errorf("record envelope ref: %w", err)
	}
	return nil
}

// importOneWithEnvelope is the single import path shared by regular, batch,
// and stdin imports. Envelope extraction, payload import, and schema metadata
// recording must have identical behavior in all three modes.
func importOneWithEnvelope(imp *importer.EnhancedImporter, opts importer.ImportOptions) (*importer.ImportResult, error) {
	originalPath := opts.SourcePath
	tempDir, err := imp.TempDir()
	if err != nil {
		return nil, err
	}
	envelope, cleanup, err := extractEnvelopeIfPresent(originalPath, tempDir)
	if err != nil {
		return nil, fmt.Errorf("unwrap envelope: %w", err)
	}
	defer cleanup()
	if envelope != nil {
		// The payload is imported from a temp file, but the import is still
		// named — origin, reported source — by the file the user gave.
		if opts.OriginPath == "" {
			opts.OriginPath = originalPath
		}
		opts.SourcePath = envelope.dataPath
	}

	result, err := imp.ImportFileWithFriendlyIDs(opts)
	if err != nil {
		return nil, err
	}
	if result != nil {
		result.SourcePath = originalPath
	}
	if result != nil && !result.Skipped {
		if err := recordItemSchemas(imp.CatalogDB(), result, envelope); err != nil {
			return nil, fmt.Errorf("record import schema metadata: %w", err)
		}
	}
	return result, nil
}

// classifyEnvelopeSchema resolves an envelope's schema portion against
// pudl's schema cache and returns the status to record. Conflicts
// between inline definitions and an existing cached version surface
// as an error rather than silently picking one.
func classifyEnvelopeSchema(cache *muschemas.Cache, env *mubridge.Envelope) (string, error) {
	if cache.Has(env.Schema.Module, env.Schema.Version) {
		if len(env.Definitions) > 0 {
			if err := registerEnvelopeDefinitions(cache, env); err != nil {
				return "", err
			}
		}
		return database.ItemSchemaStatusDeclared, nil
	}
	if len(env.Definitions) > 0 {
		if err := registerEnvelopeDefinitions(cache, env); err != nil {
			return "", err
		}
		return database.ItemSchemaStatusAutoRegistered, nil
	}
	return database.ItemSchemaStatusUnresolved, nil
}

func registerEnvelopeDefinitions(cache *muschemas.Cache, env *mubridge.Envelope) error {
	files := make([]muschemas.File, 0, len(env.Definitions))
	for _, d := range env.Definitions {
		files = append(files, muschemas.File{RelPath: d.Path, Content: []byte(d.Content)})
	}
	return cache.Insert(env.Schema.Module, env.Schema.Version, files)
}

// unwrappedEnvelope holds the parsed envelope plus the temp data path
// that the importer should ingest. cleanup is called by the caller
// (via defer) regardless of whether an envelope was detected.
type unwrappedEnvelope struct {
	envelope *mubridge.Envelope
	dataPath string
}

// extractEnvelopeIfPresent reads absPath, detects whether it is an
// envelope, and (if so) materializes the inner data to a temp file in
// tempDir — the workspace's import scratch directory, never the source's
// directory, which may be read-only. The temp name keeps the original
// basename so extension-based format detection is unaffected; origin is
// derived from the original path by the caller. Returns (nil, noopCleanup,
// nil) for raw inputs.
func extractEnvelopeIfPresent(absPath, tempDir string) (*unwrappedEnvelope, func(), error) {
	noop := func() {}
	// Avoid reading an ordinary large JSON document merely to discover that it
	// is not an envelope. The supported wire format puts the schema key near the
	// top-level object; only candidates are passed to the full envelope decoder,
	// which then materializes the inner payload.
	probeFile, err := os.Open(absPath)
	if err != nil {
		return nil, noop, err
	}
	prefix, readErr := io.ReadAll(io.LimitReader(probeFile, 64*1024))
	closeErr := probeFile.Close()
	if readErr != nil {
		return nil, noop, readErr
	}
	if closeErr != nil {
		return nil, noop, closeErr
	}
	if !bytes.Contains(prefix, []byte(`"schema"`)) {
		return nil, noop, nil
	}

	f, err := os.Open(absPath)
	if err != nil {
		return nil, noop, err
	}
	env, _, err := mubridge.Unwrap(f)
	f.Close()
	if err != nil {
		return nil, noop, err
	}
	if env == nil {
		return nil, noop, nil
	}

	tmp, err := os.CreateTemp(tempDir, ".envelope-*-"+filepath.Base(absPath))
	if err != nil {
		return nil, noop, fmt.Errorf("create temp: %w", err)
	}
	if _, err := tmp.Write(env.Data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, noop, fmt.Errorf("write inner payload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return nil, noop, err
	}
	cleanup := func() { os.Remove(tmp.Name()) }
	return &unwrappedEnvelope{envelope: env, dataPath: tmp.Name()}, cleanup, nil
}
