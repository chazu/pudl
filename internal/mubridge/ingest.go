package mubridge

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/identity"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/schemaname"
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

// ObserveResult matches mu's coordinator.ObserveResult exactly.
// mu observe --json emits a JSON array of these.
type ObserveResult struct {
	Target  string         `json:"target"`
	Current map[string]any `json:"current,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// ObserveIngest is one observation to record: the mu observe output, where to
// stage it, and the provenance that makes the resulting snapshot a first-class
// object rather than a timestamped convention.
//
// It replaces the positional IngestObserveResults* family, which had reached six
// parameters and would have taken ten.
type ObserveIngest struct {
	Reader  io.Reader
	DataDir string
	Graph   *inference.InheritanceGraph
	// SchemaMappings maps plugin-declared _schema resource types to validated
	// PUDL semantic schemas. It takes precedence over the legacy naming
	// convention when present.
	SchemaMappings map[string]string
	// Inferrer is used when a plugin-declared resource type cannot be mapped to
	// a loaded schema name. The declared resource type remains a strong hint,
	// but inference is the safe fallback instead of persisting a dangling ref.
	Inferrer *inference.SchemaInferrer

	// SnapshotID is allocated by the run before it observes, so a failed ingest
	// can still be named. Generated here when empty (the standalone
	// `pudl mu ingest-observe` path, which has no run to allocate one).
	SnapshotID string
	RunID      string
	// Model is the #SystemModel this observation was taken for, empty for a
	// standalone ingest.
	Model string
	// Workspace is where the run stood: the repo workspace name, or "global".
	Workspace string
	// Origin is the catalog ingest origin; defaults to "mu-observe".
	Origin string
	// Source is how the observation was produced; defaults to "ingest-observe".
	Source string
}

// ObserveIngestResult is what an ingest recorded.
type ObserveIngestResult struct {
	Records    int
	SnapshotID string
}

// NewSnapshotID allocates a snapshot identifier. Callers that own a run should
// allocate one up front and pass it to IngestObserve, so the run can name the
// snapshot a failed ingest would have produced.
func NewSnapshotID() string {
	return "snap_" + idgen.GenerateRandomProquint()
}

// IngestObserve processes mu observe --json output and stores it in the catalog.
// The input is a JSON array of ObserveResult objects.
//
// It creates one observe snapshot — a contract row plus the collection entry
// holding the records — and stores each record from current.records as an
// individual observe entry linked to it. Records with a _schema field are routed
// to their specific schema.
func IngestObserve(db *database.CatalogDB, in ObserveIngest) (ObserveIngestResult, error) {
	if in.Origin == "" {
		in.Origin = database.SnapshotSourceMuObserve
	}
	if in.Source == "" {
		in.Source = database.SnapshotSourceIngestObserve
	}
	if in.SnapshotID == "" {
		in.SnapshotID = NewSnapshotID()
	}

	data, err := io.ReadAll(in.Reader)
	if err != nil {
		return ObserveIngestResult{}, fmt.Errorf("failed to read input: %w", err)
	}

	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 {
		return ObserveIngestResult{}, nil
	}

	var results []ObserveResult
	if err := json.Unmarshal(data, &results); err != nil {
		return ObserveIngestResult{}, fmt.Errorf("failed to parse observe results (expected JSON array from mu observe --json): %w", err)
	}

	origin := in.Origin
	runID := in.RunID
	now := time.Now()
	rawDir := filepath.Join(in.DataDir, "raw", now.Format("2006"), now.Format("01"), now.Format("02"))
	if err := os.MkdirAll(rawDir, 0755); err != nil {
		return ObserveIngestResult{}, fmt.Errorf("failed to create raw directory: %w", err)
	}

	// Collect all records across targets, tracking metadata for the snapshot.
	type targetRecord struct {
		record map[string]any
		target string
	}
	var allRecords []targetRecord
	var targets []string
	var errors []map[string]string
	schemaCounts := map[string]int{}

	for _, result := range results {
		if result.Target == "" {
			fmt.Fprintf(os.Stderr, "Warning: skipping observe result with empty target\n")
			continue
		}

		target := strings.TrimPrefix(result.Target, "//")

		if result.Error != "" {
			fmt.Fprintf(os.Stderr, "Warning: target %s reported error: %s\n", result.Target, result.Error)
			errors = append(errors, map[string]string{"target": target, "error": result.Error})
			targets = append(targets, target)
			continue
		}
		if result.Current == nil {
			continue
		}

		targets = append(targets, target)

		// Extract records from current.records, or treat current as a single record.
		var records []map[string]any
		if rawRecords, ok := result.Current["records"]; ok {
			if arr, ok := rawRecords.([]any); ok {
				for _, item := range arr {
					if rec, ok := item.(map[string]any); ok {
						records = append(records, rec)
					}
				}
			}
		}
		if len(records) == 0 {
			records = []map[string]any{result.Current}
		}

		for _, rec := range records {
			allRecords = append(allRecords, targetRecord{record: rec, target: target})
			if _, ok := rec["_schema"].(string); ok {
				schemaCounts[resolveObserveSchemaWithMappings(rec, in.Graph, in.Inferrer, in.SchemaMappings)]++
			} else {
				schemaCounts["pudl/mu.#ObserveResult"]++
			}
		}
	}

	// Everything above is parsing; nothing has touched the catalog yet. The
	// writes below are one step and are recorded as one: the snapshot contract,
	// its collection entry and every membership commit together, so a failure
	// part-way through cannot leave a snapshot describing records that were never
	// stored, or records belonging to a snapshot that does not exist. That partial
	// state is exactly what a later run would read as an observation.
	ingested := 0

	err = db.WithCatalogTx(func(tx *database.CatalogTx) error {
		if err := createObserveSnapshot(tx, observeSnapshotEntry{
			snapshotID:   in.SnapshotID,
			now:          now,
			origin:       origin,
			targets:      targets,
			recordCount:  len(allRecords),
			schemaCounts: schemaCounts,
			errors:       errors,
			rawDir:       rawDir,
			runID:        runID,
		}); err != nil {
			return err
		}
		if err := tx.RecordObserveSnapshot(database.ObserveSnapshot{
			SnapshotID:  in.SnapshotID,
			RunID:       runID,
			Model:       in.Model,
			Workspace:   in.Workspace,
			Origin:      origin,
			Source:      in.Source,
			Targets:     targets,
			RecordCount: len(allRecords),
			CreatedAt:   now,
		}); err != nil {
			return err
		}

		ingested = 0
		for i, tr := range allRecords {
			n, err := ingestObserveRecord(tx, tr.record, tr.target, origin, rawDir, now, i, in.SnapshotID, in.Graph, in.Inferrer, in.SchemaMappings, runID)
			if err != nil {
				return err
			}
			ingested += n
		}
		return nil
	})
	if err != nil {
		// Nothing was recorded, so report nothing recorded — the old partial
		// count and snapshot ID described rows that had just been rolled back.
		return ObserveIngestResult{}, err
	}

	return ObserveIngestResult{Records: ingested, SnapshotID: in.SnapshotID}, nil
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
func createObserveSnapshot(db database.CatalogWriter, in observeSnapshotEntry) error {
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
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}
	hash := sha256.Sum256(snapshotJSON)
	contentHash := fmt.Sprintf("%x", hash)

	// Store the snapshot JSON.
	filename := fmt.Sprintf("%s_snapshot.json", in.snapshotID)
	storedPath := filepath.Join(in.rawDir, filename)
	if err := os.WriteFile(storedPath, snapshotJSON, 0644); err != nil {
		return fmt.Errorf("failed to write snapshot: %w", err)
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

	if err := db.AddEntry(entry); err != nil {
		return fmt.Errorf("failed to add snapshot entry: %w", err)
	}
	return nil
}

// ingestObserveRecord stores a single observe record in the catalog.
// Returns 1 if ingested, 0 if deduplicated, or an error.
func ingestObserveRecord(
	db database.CatalogWriter,
	record map[string]any,
	target string,
	origin string,
	rawDir string,
	now time.Time,
	index int,
	collectionID string,
	graph *inference.InheritanceGraph,
	inferrer *inference.SchemaInferrer,
	schemaMappings map[string]string,
	runID string,
) (int, error) {
	// Determine schema from _schema field, falling back to generic observe result.
	schema := resolveObserveSchemaWithMappings(record, graph, inferrer, schemaMappings)

	// Compute content hash from the canonical JSON of the record.
	recordJSON, err := json.Marshal(record)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal record: %w", err)
	}
	hash := sha256.Sum256(recordJSON)
	contentHash := fmt.Sprintf("%x", hash)

	// Dedup: skip if exact same content already exists for this target.
	existing, err := db.GetLatestObserveByContentHash(target, contentHash)
	if err != nil {
		return 0, fmt.Errorf("dedup check failed for %s: %w", target, err)
	}
	if existing != nil {
		// The entry keeps the run that *first* observed it. Rewriting run_id to the
		// current run made the association last-writer-wins: an entry first seen by
		// run A silently moved to run B on the next identical observation, so a
		// query for run A under-reported what run A actually saw. Invariant 3 wants
		// exactly one run per observation, and re-running must not degrade the
		// provenance that replay-by-durable-ID depends on.
		//
		// This run's sighting is not lost — it is recorded as snapshot membership
		// below, which is the relationship that is legitimately many-to-many.
		if err := db.AddCollectionMembership(collectionID, existing.ID, index); err != nil {
			return 0, fmt.Errorf("failed to link existing observe record to snapshot: %w", err)
		}
		return 0, nil
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
	if err := os.WriteFile(storedPath, recordJSON, 0644); err != nil {
		return 0, fmt.Errorf("failed to write observe record: %w", err)
	}

	// Compute resource ID from the record's identity fields.
	identityValues := map[string]any{"target": target}
	if s, ok := record["_schema"].(string); ok {
		identityValues["_schema"] = s
	}
	for _, key := range []string{"hostname", "host", "name", "unit", "mountpoint", "ifname"} {
		if v, ok := record[key]; ok {
			identityValues[key] = v
		}
	}
	resourceID := identity.ComputeResourceID(identityNamespace(graph, schema), identityValues, contentHash)

	entryType := "observe"
	collectionType := "item"
	itemID := fmt.Sprintf("%s_item_%d", safeTarget, index)
	var runIDPtr *string
	if runID != "" {
		runIDPtr = &runID
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
		ContentHash:     &contentHash,
		CollectionID:    &collectionID,
		CollectionType:  &collectionType,
		ItemIndex:       &index,
		ItemID:          &itemID,
		RunID:           runIDPtr,
	}

	if err := db.AddEntry(entry); err != nil {
		return 0, fmt.Errorf("failed to add observe entry: %w", err)
	}

	return 1, nil
}

const genericObserveSchema = "pudl/mu.#ObserveResult"

// resolveObserveSchema turns a plugin-declared resource type into a schema
// reference only when that reference exists in the current schema namespace.
// If the naming convention does not identify a loaded schema, the inferrer gets
// one chance to resolve the declared resource type by _pudl.resource_type;
// otherwise the record is safely classified as the generic observe result.
func resolveObserveSchema(record map[string]any, graph *inference.InheritanceGraph, inferrer *inference.SchemaInferrer) string {
	return resolveObserveSchemaWithMappings(record, graph, inferrer, nil)
}

// resolveObserveSchemaWithMappings prefers an explicit plugin mapping over
// the historical resource-type-to-CUE-name convention. Invalid mappings are
// not persisted: callers validate package metadata before ingest, while this
// function still falls back safely for library callers that bypass validation.
func resolveObserveSchemaWithMappings(record map[string]any, graph *inference.InheritanceGraph, inferrer *inference.SchemaInferrer, schemaMappings map[string]string) string {
	declared, ok := record["_schema"].(string)
	if !ok || declared == "" {
		return genericObserveSchema
	}
	if mapped, exists := schemaMappings[declared]; exists && graph != nil {
		mapped = schemaname.Normalize(mapped)
		if graph.HasSchema(mapped) {
			return mapped
		}
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
