package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/chazu/pudl/internal/identity"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/projection"
	"github.com/chazu/pudl/internal/redact"
	"github.com/chazu/pudl/internal/schemaname"
)

// maxPreviewIssues caps the validation issues a dry run quotes.
const maxPreviewIssues = 5

// PreviewReport is what `pudl import --dry-run` adds to an import result: how
// the records would be classified, identified, redacted and projected, without
// anything being written.
type PreviewReport struct {
	Schemas            map[string]int `json:"schemas"`
	AlreadyCataloged   int            `json:"already_cataloged"`
	WouldReassign      int            `json:"would_reassign,omitempty"`
	ValidationFailures int            `json:"validation_failures,omitempty"`
	ValidationIssues   []string       `json:"validation_issues,omitempty"`
	Ambiguous          int            `json:"ambiguous,omitempty"`
	AmbiguousExample   string         `json:"ambiguous_example,omitempty"`
}

// Preview analyzes a source exactly as an import would — --set, schema
// assignment, redaction, identity — and writes nothing: no staging, no catalog
// rows, no facts. The catalog is only read, for dedup lookups.
func (e *EnhancedImporter) Preview(opts ImportOptions) (*ImportResult, error) {
	if opts.Context == nil {
		opts.Context = context.Background()
	}
	var err error
	if opts.Limits, err = opts.Limits.Resolve(); err != nil {
		return nil, err
	}
	info, err := os.Stat(opts.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory; import its files individually or with a wildcard", opts.SourcePath)
	}
	if compression := DetectCompression(opts.SourcePath); compression != "none" {
		tempDir, err := e.TempDir()
		if err != nil {
			return nil, err
		}
		plainPath, err := DecompressToLimited(opts.Context, opts.SourcePath, tempDir, opts.Limits.DecodedBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress %s: %w", opts.SourcePath, err)
		}
		defer os.Remove(plainPath)
		opts.OriginPath = opts.originPath()
		opts.SourcePath = plainPath
	}
	format, err := e.detectFormat(opts.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to detect format: %w", err)
	}
	origin := opts.Origin
	if origin == "" {
		origin = e.detectOrigin(opts.originPath(), format)
	}
	shape := sourceShape{format: format, origin: origin}
	shape.collection, _ = streamableCollectionFormat(opts.SourcePath, format)
	if shape.collection == "" && shape.format != "json" && len(opts.Set) > 0 {
		return nil, fmt.Errorf("--set applies only to JSON or NDJSON input (got %s); convert the source to JSON", format)
	}

	report := &PreviewReport{Schemas: map[string]int{}}
	result := &ImportResult{SourcePath: opts.originPath(), DetectedFormat: format, DetectedOrigin: origin, DryRun: true, Preview: report}
	if shape.collection != "" {
		result.DetectedFormat = shape.collection
	}
	var tally identityTally
	facts := projection.NewTally()
	intended := intendedSchema(opts)
	traced := opts
	traced.Explain = true // ambiguity is read from the classification trace

	n, err := e.walkRecords(opts, opts.SourcePath, shape, func(index int, record any) error {
		if err := applyAssignments(record, opts.Set); err != nil {
			return fmt.Errorf("record %d: %w", index, err)
		}
		var assigned schemaAssignment
		if shape.collection != "" {
			assigned = e.assignItemSchemaDetailed(record, traced)
		} else {
			var err error
			if assigned, err = e.assignSchema(record, traced, documentHints(shape)); err != nil {
				return err
			}
		}
		schema := schemaname.Normalize(assigned.Schema)
		report.Schemas[schema]++

		paths, err := e.redactor.For(intended, schema)
		if err != nil {
			return fmt.Errorf("record %d: %w", index, err)
		}
		if len(paths) > 0 {
			if shape.collection == "" && shape.format != "json" {
				return fmt.Errorf("schema %s declares sensitive fields (%s), which cannot be redacted in a %s document; convert the source to JSON",
					schema, redact.Describe(paths), format)
			}
			result.Redacted += redact.Apply(record, paths)
			assigned.Validation = stripValidationValues(assigned.Validation)
		}

		if intended != "" && !assigned.Validated {
			report.ValidationFailures++
			if vr := assigned.Validation; vr != nil {
				for _, issue := range vr.GetErrorsForSchema(vr.IntendedSchema) {
					if len(report.ValidationIssues) == maxPreviewIssues {
						break
					}
					report.ValidationIssues = append(report.ValidationIssues, fmt.Sprintf("record %d: %s: %s", index, issue.Path, issue.Message))
				}
			}
		}
		if assigned.Trace != nil && len(assigned.Trace.Alternatives) > 0 {
			report.Ambiguous++
			if report.AmbiguousExample == "" {
				report.AmbiguousExample = fmt.Sprintf("%s also matched %v", schema, assigned.Trace.Alternatives)
			}
		}

		values, idErr := identity.ExtractFieldValues(record, e.getSchemaIdentityFields(schema))
		if idErr != nil {
			tally.record(schema, idErr)
			values = nil
		}
		hash, err := previewContentHash(opts, shape, record, result.Redacted)
		if err != nil {
			return err
		}
		rid := identity.ComputeResourceID(e.identityNamespace(schema), values, hash)
		_, projected := projection.Prepare(e.projector, schema, hash, rid, len(values) > 0, record)
		facts.Note(e.projector, schema, projected)
		{
			existing, err := e.catalogDB.FindByContentHash(hash)
			if err != nil {
				return err
			}
			if existing != nil {
				report.AlreadyCataloged++
				if assigned.Validated && existing.Schema != schema {
					report.WouldReassign++
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result.RecordCount = n
	tally.apply(result)
	applyTally(result, facts)
	return result, nil
}

// previewContentHash is the hash the import would dedup a record on: an item's
// canonical JSON, or a document's stored bytes (the source itself unless the
// pre-pass would rewrite it).
func previewContentHash(opts ImportOptions, shape sourceShape, record any, redacted int) (string, error) {
	if shape.collection == "" && len(opts.Set) == 0 && redacted == 0 {
		f, err := os.Open(opts.SourcePath)
		if err != nil {
			return "", err
		}
		defer f.Close()
		return idgen.ComputeContentIDReader(f)
	}
	canonical, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	return idgen.ComputeContentID(canonical), nil
}
