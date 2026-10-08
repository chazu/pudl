package importer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/ingestprep"
	"github.com/chazu/pudl/internal/redact"
	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/validator"
)

// The transform pre-pass rewrites a source before staging when an import must
// change its records: `--set` assignments, and redaction of sensitive fields.
// Rewriting first keeps every downstream invariant — the collection or document
// hash is the hash of the stored bytes, items hash their canonical record —
// with no special cases in dedup, staging or VerifyPayloads.

// sourceShape says how a source decodes into records.
type sourceShape struct {
	collection string // "ndjson" | "json-array" for collections, "" for a document
	format     string // the detected format
	origin     string // the detected origin, an inference hint for documents
}

// documentHints are the inference hints a single-document import uses.
func documentHints(shape sourceShape) inference.InferenceHints {
	return inference.InferenceHints{Origin: shape.origin, Format: shape.format}
}

// walkRecords decodes the source and calls fn once per record: each element of
// a collection, or the one decoded document. Collections stream under the
// import limits.
func (e *EnhancedImporter) walkRecords(opts ImportOptions, sourcePath string, shape sourceShape, fn func(index int, record any) error) (int, error) {
	if shape.collection == "" {
		if shape.format == "json" {
			raw, err := readBounded(sourcePath, opts.Limits.RecordBytes)
			if err != nil {
				return 0, err
			}
			record, err := idgen.DecodeJSONExact(raw)
			if err != nil {
				return 0, fmt.Errorf("decode %s: %w", opts.originPath(), err)
			}
			return 1, fn(0, record)
		}
		data, _, err := e.analyzeData(sourcePath, shape.format)
		if err != nil {
			return 0, err
		}
		return 1, fn(0, data)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return 0, err
	}
	defer source.Close()
	reader := &ingestprep.Reader{Context: opts.Context, Source: source, Remaining: opts.Limits.DecodedBytes}
	sink := func(index int, raw json.RawMessage) error {
		record, err := idgen.DecodeJSONExact(raw)
		if err != nil {
			return fmt.Errorf("decode record %d: %w", index, err)
		}
		return fn(index, record)
	}
	if shape.collection == "ndjson" {
		return ingestprep.NDJSON(reader, opts.Limits.RecordBytes, sink)
	}
	return ingestprep.Array(reader, opts.Limits.RecordBytes, sink)
}

// readBounded reads a whole file, refusing one larger than limit.
func readBounded(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("document exceeds record byte limit (%d)", limit)
	}
	return os.ReadFile(path)
}

// transformResult is the outcome of the pre-pass.
type transformResult struct {
	// path is the rewritten source, or "" when no record changed and the
	// original file must be imported unchanged (so its hash, and dedup, hold).
	path        string
	size        int64
	assignments string // spooled per-record schema assignments, or ""
	redacted    int
}

func (t *transformResult) cleanup() {
	if t == nil {
		return
	}
	if t.path != "" {
		_ = os.Remove(t.path)
	}
	if t.assignments != "" {
		_ = os.Remove(t.assignments)
	}
}

// needsTransform reports whether the pre-pass must run.
func (e *EnhancedImporter) needsTransform(opts ImportOptions) bool {
	return len(opts.Set) > 0 || e.redactor.Any()
}

// intendedSchema is the schema the caller routed records to, if any.
func intendedSchema(opts ImportOptions) string {
	if opts.ManualSchema == "" {
		return ""
	}
	return schemaname.Normalize(opts.ManualSchema)
}

// transformSource runs the pre-pass over sourcePath.
func (e *EnhancedImporter) transformSource(opts ImportOptions, sourcePath string, shape sourceShape) (result *transformResult, resultErr error) {
	if shape.collection == "" && shape.format != "json" {
		return e.checkUnstructuredDocument(opts, sourcePath, shape)
	}
	tmp, err := e.TempDir()
	if err != nil {
		return nil, err
	}
	result = &transformResult{}
	defer func() {
		if resultErr != nil {
			result.cleanup()
		}
	}()

	// Keep the source's extension: format detection is extension-first, and
	// the stored file is named after it.
	out, err := os.CreateTemp(tmp, "transform-*"+filepath.Ext(sourcePath))
	if err != nil {
		return nil, err
	}
	result.path = out.Name()
	defer out.Close()
	w := bufio.NewWriter(out)

	var spool *json.Encoder
	var spoolFile *os.File
	redacting := e.redactor.Any()
	if redacting {
		spoolFile, err = os.CreateTemp(tmp, "assignments-*.ndjson")
		if err != nil {
			return nil, err
		}
		result.assignments = spoolFile.Name()
		defer spoolFile.Close()
		spoolWriter := bufio.NewWriter(spoolFile)
		defer spoolWriter.Flush()
		spool = json.NewEncoder(spoolWriter)
	}

	if shape.collection == "json-array" {
		if _, err := w.WriteString("["); err != nil {
			return nil, err
		}
	}
	_, err = e.walkRecords(opts, sourcePath, shape, func(index int, record any) error {
		if err := applyAssignments(record, opts.Set); err != nil {
			return fmt.Errorf("record %d: %w", index, err)
		}
		if redacting {
			assigned, err := e.assignForTransform(record, opts, shape, index)
			if err != nil {
				return err
			}
			paths, err := e.redactor.For(intendedSchema(opts), assigned.Schema)
			if err != nil {
				return fmt.Errorf("record %d: %w", index, err)
			}
			if len(paths) > 0 {
				result.redacted += redact.Apply(record, paths)
				assigned.Validation = stripValidationValues(assigned.Validation)
			}
			if err := spool.Encode(assigned); err != nil {
				return err
			}
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("encode record %d: %w", index, err)
		}
		switch shape.collection {
		case "ndjson":
			encoded = append(encoded, '\n')
		case "json-array":
			if index > 0 {
				encoded = append([]byte(","), encoded...)
			}
		}
		_, err = w.Write(encoded)
		return err
	})
	if err != nil {
		return nil, err
	}
	if shape.collection == "json-array" {
		if _, err := w.WriteString("]"); err != nil {
			return nil, err
		}
	}
	if err := w.Flush(); err != nil {
		return nil, err
	}
	if info, err := out.Stat(); err == nil {
		result.size = info.Size()
	}
	if len(opts.Set) == 0 && result.redacted == 0 {
		// Nothing changed: import the original bytes so unrelated imports keep
		// their content hash. The spooled assignments still describe the same
		// records in the same order, so the main pass reuses them.
		_ = os.Remove(result.path)
		result.path = ""
	}
	return result, nil
}

// checkUnstructuredDocument handles YAML/CSV and other non-JSON documents,
// which the pre-pass cannot rewrite: --set is refused, and a document routed
// to a schema with sensitive fields fails closed rather than being stored.
func (e *EnhancedImporter) checkUnstructuredDocument(opts ImportOptions, sourcePath string, shape sourceShape) (*transformResult, error) {
	if len(opts.Set) > 0 {
		return nil, fmt.Errorf("--set applies only to JSON or NDJSON input (got %s); convert the source to JSON", shape.format)
	}
	_, err := e.walkRecords(opts, sourcePath, shape, func(_ int, record any) error {
		assigned, err := e.assignSchema(record, opts, documentHints(shape))
		if err != nil {
			return err
		}
		paths, err := e.redactor.For(intendedSchema(opts), assigned.Schema)
		if err != nil {
			return err
		}
		if len(paths) > 0 {
			return fmt.Errorf("schema %s declares sensitive fields (%s), which cannot be redacted in a %s document; convert the source to JSON",
				assigned.Schema, redact.Describe(paths), shape.format)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &transformResult{}, nil
}

// assignForTransform assigns the schema the main pass will reuse for record.
func (e *EnhancedImporter) assignForTransform(record any, opts ImportOptions, shape sourceShape, index int) (spooledAssignment, error) {
	var assigned schemaAssignment
	if shape.collection != "" {
		itemOpts := opts
		itemOpts.Explain = opts.Explain && index < maxItemExplanations
		assigned = e.assignItemSchemaDetailed(record, itemOpts)
	} else {
		var err error
		assigned, err = e.assignSchema(record, opts, documentHints(shape))
		if err != nil {
			return spooledAssignment{}, err
		}
	}
	spooled := spooledAssignment{
		Schema: assigned.Schema, Confidence: assigned.Confidence, Reason: assigned.Reason,
		SourcePath: assigned.SourcePath, Trace: assigned.Trace, Validated: assigned.Validated,
	}
	if shape.collection == "" {
		// Only a document reports its validation result.
		spooled.Validation = assigned.Validation
	}
	return spooled, nil
}

// stripValidationValues returns a copy of vr whose errors no longer quote data:
// values are dropped and messages (CUE messages quote conflicting values) are
// replaced. Paths are kept; they name fields, not data. Used only for schemas
// with sensitive fields.
func stripValidationValues(vr *validator.ValidationResult) *validator.ValidationResult {
	if vr == nil {
		return nil
	}
	strip := func(errs []validator.ValidationError) []validator.ValidationError {
		out := make([]validator.ValidationError, len(errs))
		for i, e := range errs {
			e.Value = nil
			e.Message = "value rejected (details withheld: schema declares sensitive fields)"
			out[i] = e
		}
		return out
	}
	out := *vr
	out.ValidationErrors = strip(vr.ValidationErrors)
	out.ChainAttempts = make([]validator.ChainAttempt, len(vr.ChainAttempts))
	for i, a := range vr.ChainAttempts {
		a.Errors = strip(a.Errors)
		out.ChainAttempts[i] = a
	}
	return &out
}
