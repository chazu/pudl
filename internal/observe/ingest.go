package observe

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/artifacts"
	"github.com/chazu/pudl/internal/ingestprep"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/identity"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/policyfacts"
	"github.com/chazu/pudl/internal/projection"
	"github.com/chazu/pudl/internal/redact"
	"github.com/chazu/pudl/internal/secretscan"
	"github.com/chazu/pudl/internal/validator"
)

// identityNamespace returns the schema used to namespace resource identity:
// the root of the assigned schema's inheritance family. A nil graph (e.g. in
// tests) falls back to the schema itself.
func identityNamespace(graph *inference.InheritanceGraph, schema string) string {
	if graph == nil {
		return schema
	}
	return graph.IdentityRoot(schema)
}

// ObserveResult is one observed target and its current records. A populate
// phase wraps what its commands printed as a JSON array of these. The shape is
// the historical mu `observe --json` element, so stored observations keep it.
type ObserveResult struct {
	Target  string         `json:"target"`
	Current map[string]any `json:"current,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// ObserveIngest is one observation to record: the wrapped observe results,
// where to stage them, and the provenance that makes the resulting snapshot a
// first-class object rather than a timestamped convention.
type ObserveIngest struct {
	Scope    string
	Complete bool
	Schemas  []string
	Context  context.Context
	Limits   ingestprep.Limits
	Reader   io.Reader
	DataDir  string
	Graph    *inference.InheritanceGraph
	// Inferrer is used when a record's declared _schema resource type cannot be
	// mapped to a loaded schema name. The declared resource type remains a
	// strong hint, but inference is the safe fallback instead of persisting a
	// dangling ref.
	Inferrer *inference.SchemaInferrer

	// SnapshotID is allocated by the run before it observes, so a failed ingest
	// can still be named. Generated here when empty.
	SnapshotID string
	RunID      string
	// Model is the #SystemModel this observation was taken for, empty for a
	// standalone ingest.
	Model string
	// Workspace is where the run stood: the repo workspace name, or "global".
	Workspace string
	// Origin is the catalog ingest origin; defaults to "mu-observe" (the
	// historical value, kept so stored origins stay comparable).
	Origin string
	// Source is how the observation was produced; defaults to "ingest-observe".
	Source string

	// ManualSchema requires every record to satisfy this schema. The command
	// arm may explicitly enable AllowSchemaFallback for exploratory use. Empty
	// keeps the _schema-based routing.
	AllowSchemaFallback bool
	ManualSchema        string
	Chain               *validator.ChainValidator
	// Redactor supplies sensitive-field paths; nil builds one from Inferrer.
	Redactor *redact.Registry
	// Projection computes schema-declared facts; nil builds one from Inferrer.
	Projection *projection.Registry
}

// ObserveIngestResult is what an ingest recorded.
type ObserveIngestResult struct {
	SchemaMismatches int
	Records          int
	SnapshotID       string
	// Redacted counts sensitive values replaced before storage.
	Redacted int
	// Facts counts projected facts per relation; FactWarnings explains gaps.
	Facts        map[string]int
	FactWarnings []string
	// SecretWarnings lists likely secrets found outside schema sensitive-field
	// coverage, by record index, schema, path and kind (never the value),
	// capped at secretscan.MaxWarnings; SecretWarningsTotal counts them all.
	// Stored data is unchanged.
	SecretWarnings      []secretscan.Warning
	SecretWarningsTotal int
	// PolicyChange counts the policy results the ingest wrote or closed.
	PolicyChange policyfacts.Change
}

// NewSnapshotID allocates a snapshot identifier. Callers that own a run should
// allocate one up front and pass it to IngestObserve, so the run can name the
// snapshot a failed ingest would have produced.
func NewSnapshotID() string {
	return "snap_" + idgen.GenerateRandomProquint()
}

// IngestObserve stores wrapped observe results in the catalog. The input is a
// JSON array of ObserveResult objects.
//
// It creates one observe snapshot — a contract row plus the collection entry
// holding the records — and stores each record from current.records as an
// individual observe entry linked to it. Records with a _schema field are routed
// to their specific schema.
func IngestObserve(db *database.CatalogDB, in ObserveIngest) (ObserveIngestResult, error) {
	prepared, err := PrepareObservation(in)
	if err != nil {
		return ObserveIngestResult{}, err
	}
	defer prepared.Close()
	if prepared.empty {
		return ObserveIngestResult{}, nil
	}
	return prepared.Commit(db)
}

// observeSnapshotEntry is what createObserveSnapshot needs to stage the
// snapshot's own evidence file and its catalog collection entry.
type observeSnapshotEntry struct {
	snapshotID   string
	now          time.Time
	origin       string
	targets      []string
	recordCount  int
	schemaCounts map[string]int
	errors       []map[string]string
	rawDir       string
	maxBytes     int64
	runID        string
}

// createObserveSnapshot writes the snapshot's evidence file and the catalog
// collection entry that holds its records.
//
// The entry is keyed on the *pre-allocated* snapshot ID, not on the hash of its
// own payload. There used to be two identifiers for one snapshot — a readable
// `observe_<timestamp>` inside the payload that nothing used, and the content
// hash that everything actually used — and the run could not name the snapshot
// it was about to create. One identifier, allocated before the observation, is
// what lets a failed ingest still be named.
//
// The content hash is retained in content_hash, where it belongs.
func prepareObserveSnapshot(in observeSnapshotEntry) (database.CatalogEntry, error) {
	// Build schema summary.
	var schemaSummary []map[string]any
	for schema, count := range in.schemaCounts {
		schemaSummary = append(schemaSummary, map[string]any{
			"schema": schema,
			"count":  count,
		})
	}

	snapshot := map[string]any{
		"snapshot_id":    in.snapshotID,
		"timestamp":      in.now.Format(time.RFC3339),
		"origin":         in.origin,
		"targets":        in.targets,
		"record_count":   in.recordCount,
		"schema_summary": schemaSummary,
	}
	if len(in.errors) > 0 {
		snapshot["errors"] = in.errors
	}
	if in.runID != "" {
		snapshot["run_id"] = in.runID
	}

	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return database.CatalogEntry{}, fmt.Errorf("failed to marshal snapshot: %w", err)
	}
	if int64(len(snapshotJSON)) > in.maxBytes {
		return database.CatalogEntry{}, fmt.Errorf("observation exceeds staging byte limit")
	}
	hash := sha256.Sum256(snapshotJSON)
	contentHash := fmt.Sprintf("%x", hash)

	// Store the snapshot JSON.
	filename := fmt.Sprintf("%s_snapshot.json", in.snapshotID)
	storedPath := filepath.Join(in.rawDir, filename)
	if err := artifacts.WriteFilePrivate(storedPath, snapshotJSON); err != nil {
		return database.CatalogEntry{}, fmt.Errorf("failed to write snapshot: %w", err)
	}

	// No snapshot-level dedup. A snapshot is the record of *one* observation by
	// *one* run, which is what invariant 3 requires and what `--catalog-scope`
	// selects; collapsing two observations into a shared row would leave the
	// second run with no snapshot of its own. Idempotency (invariant 9) is served
	// at the record level instead — see the content-hash dedup in
	// ingestObserveRecord, which reuses the record entry and adds a membership
	// rather than duplicating it.

	// ObserveSnapshot is a family root, so it is its own identity namespace.
	schema := "pudl/mu.#ObserveSnapshot"
	resourceID := identity.ComputeResourceID(schema, map[string]any{"snapshot_id": in.snapshotID}, contentHash)
	entryType := "observe"
	collectionType := "collection"
	var runIDPtr *string
	if in.runID != "" {
		runIDPtr = &in.runID
	}

	entry := database.CatalogEntry{
		ID:              in.snapshotID,
		StoredPath:      storedPath,
		ImportTimestamp: in.now,
		Format:          "json",
		Origin:          in.origin,
		Schema:          schema,
		Confidence:      1.0,
		RecordCount:     in.recordCount,
		SizeBytes:       int64(len(snapshotJSON)),
		EntryType:       &entryType,
		ResourceID:      &resourceID,
		ContentHash:     &contentHash,
		CollectionType:  &collectionType,
		RunID:           runIDPtr,
	}

	return entry, nil
}

// ingestObserveRecord stores a single observe record in the catalog.
// Returns 1 if ingested, 0 if deduplicated, or an error.
func prepareObserveRecord(
	record map[string]any,
	target string,
	origin string,
	rawDir string,
	now time.Time,
	index int,
	collectionID string,
	graph *inference.InheritanceGraph,
	inferrer *inference.SchemaInferrer,
	runID string,
	route observeRoute,
) (database.CatalogEntry, []byte, error) {
	// Determine schema from _schema field, falling back to generic observe result.
	schema, err := route.resolve(record, graph, inferrer)
	if err != nil {
		return database.CatalogEntry{}, nil, err
	}
	// Redact before hashing, identity and storage: the secret must reach
	// neither the record file nor the content hash.
	if err := route.redact(record, schema); err != nil {
		return database.CatalogEntry{}, nil, err
	}
	route.scanSecrets(record, index, schema)

	// Compute content hash from the canonical JSON of the record.
	recordJSON, err := json.Marshal(record)
	if err != nil {
		return database.CatalogEntry{}, nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	hash := sha256.Sum256(recordJSON)
	contentHash := fmt.Sprintf("%x", hash)

	// Binding selectors use the assigned schema's declared identity fields, not
	// target/name heuristics. Persist the canonical schema-relative identity on
	// every typed observation so a later resolver can match it exactly without
	// reopening or re-inferring the raw record.
	var identityValues map[string]any
	if inferrer != nil {
		if meta, ok := inferrer.GetSchemaMetadata(schema); ok {
			identityValues, err = identity.ExtractFieldValues(record, meta.IdentityFields)
			if err != nil {
				identityValues = nil
			}
		}
	}
	resourceID := identity.ComputeResourceID(identityNamespace(graph, schema), identityValues, contentHash)
	identityJSON := ""
	if len(identityValues) > 0 {
		identityJSON, err = identity.CanonicalIdentityJSON(identityValues)
		if err != nil {
			return database.CatalogEntry{}, nil, fmt.Errorf("canonicalize observe identity: %w", err)
		}
	}

	// Store raw JSON.
	//
	// The filename carries the content hash, not the record's index. Indexing it
	// made the name a function of (second, target, position), so two observations
	// of the same target within one second — a converge loop re-observing, or two
	// models watching one host — wrote to the same path and the later one
	// silently overwrote the earlier record's evidence. The entries stayed
	// distinct, so the first snapshot went on pointing at a file that now held
	// somebody else's record, and a set-diff against it could report clean off
	// data it never observed.
	//
	// The hash is already this entry's ID, so file and entry are now one-to-one.
	// Two writes can only collide when the content is identical, in which case
	// the bytes are too — and the dedup above means that path is not reached
	// anyway.
	safeTarget := strings.ReplaceAll(target, "/", "--")
	filename := fmt.Sprintf("%s_observe_%s_%s.json", now.Format("20060102_150405"), safeTarget, contentHash[:16])
	storedPath := filepath.Join(rawDir, filename)
	entryType := "observe"
	collectionType := "item"
	itemID := fmt.Sprintf("%s_item_%d", safeTarget, index)
	var runIDPtr *string
	if runID != "" {
		runIDPtr = &runID
	}
	var identityJSONPtr *string
	if identityJSON != "" {
		identityJSONPtr = &identityJSON
	}
	entry := database.CatalogEntry{
		ID:              contentHash,
		StoredPath:      storedPath,
		ImportTimestamp: now,
		Format:          "json",
		Origin:          origin,
		Schema:          schema,
		Confidence:      1.0,
		RecordCount:     1,
		SizeBytes:       int64(len(recordJSON)),
		EntryType:       &entryType,
		Target:          &target,
		ResourceID:      &resourceID,
		IdentityJSON:    identityJSONPtr,
		ContentHash:     &contentHash,
		CollectionID:    &collectionID,
		CollectionType:  &collectionType,
		ItemIndex:       &index,
		ItemID:          &itemID,
		RunID:           runIDPtr,
	}

	return entry, recordJSON, nil
}

const genericObserveSchema = "pudl/mu.#ObserveResult"

// resolveObserveSchema turns a record's declared resource type into a schema
// reference only when that reference exists in the current schema namespace.
// If the naming convention does not identify a loaded schema, the inferrer gets
// one chance to resolve the declared resource type by _pudl.resource_type;
// otherwise the record is safely classified as the generic observe result.
func resolveObserveSchema(record map[string]any, graph *inference.InheritanceGraph, inferrer *inference.SchemaInferrer) string {
	declared, ok := record["_schema"].(string)
	if !ok || declared == "" {
		return genericObserveSchema
	}

	candidate := resourceTypeToSchema(declared)
	// A nil graph means there is no loaded schema namespace to validate against.
	// Accepting the naming-convention candidate in that case would recreate the
	// original dangling-reference bug for library callers that omit schema
	// context. The CLI always supplies the graph built from its active schema
	// repository.
	if graph != nil && graph.HasSchema(candidate) {
		return candidate
	}

	if inferrer != nil {
		result, err := inferrer.Infer(record, inference.InferenceHints{
			DeclaredSchema: declared,
			CollectionType: "item",
		})
		if err == nil && result != nil && result.Schema != "" {
			return result.Schema
		}
	}
	return genericObserveSchema
}

// resourceTypeToSchema converts a _schema resource type like "linux.host" to
// a pudl schema path like "pudl/linux.#Host". The final resource component is
// used because provider namespaces may contain dots (for example
// "aws.ec2.vpc"). Known initialisms preserve the spelling used by CUE
// definitions such as #VPC, #EC2Instance, and #NATGateway.
func resourceTypeToSchema(resourceType string) string {
	parts := strings.Split(resourceType, ".")
	if len(parts) < 2 {
		return genericObserveSchema
	}
	pkg := parts[0]
	name := pascalResourceName(parts[len(parts)-1])
	if name == "" {
		return genericObserveSchema
	}
	return fmt.Sprintf("pudl/%s.#%s", pkg, name)
}

var resourceInitialisms = map[string]string{
	"acl": "ACL", "api": "API", "arn": "ARN", "aws": "AWS",
	"cidr": "CIDR", "dns": "DNS", "ec2": "EC2", "gcp": "GCP",
	"http": "HTTP", "https": "HTTPS", "iam": "IAM", "id": "ID",
	"ip": "IP", "json": "JSON", "k8s": "K8s", "nat": "NAT",
	"rds": "RDS", "ssh": "SSH", "tcp": "TCP", "tls": "TLS",
	"udp": "UDP", "uid": "UID", "uri": "URI", "url": "URL",
	"vpc": "VPC", "yaml": "YAML",
}

func pascalResourceName(raw string) string {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == '_' || r == '-' })
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		if initialism, ok := resourceInitialisms[strings.ToLower(part)]; ok {
			b.WriteString(initialism)
			continue
		}
		runes := []rune(part)
		b.WriteString(strings.ToUpper(string(runes[0])))
		b.WriteString(string(runes[1:]))
	}
	return b.String()
}
