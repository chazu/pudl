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

	"pudl/internal/database"
	"pudl/internal/identity"
)

// ObserveResult matches mu's coordinator.ObserveResult exactly.
// mu observe --json emits a JSON array of these.
type ObserveResult struct {
	Target  string         `json:"target"`
	Current map[string]any `json:"current,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// IngestObserveResults processes mu observe --json output and stores results
// in the catalog. The input is a JSON array of ObserveResult objects.
//
// Creates an ObserveSnapshot collection entry for the run, then stores each
// record from current.records as an individual observe entry linked to the
// snapshot. Records with a _schema field are routed to their specific schema.
//
// Returns the number of records ingested and any error.
func IngestObserveResults(db *database.CatalogDB, reader io.Reader, origin string, dataDir string) (int, error) {
	if origin == "" {
		origin = "mu-observe"
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return 0, fmt.Errorf("failed to read input: %w", err)
	}

	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 {
		return 0, nil
	}

	var results []ObserveResult
	if err := json.Unmarshal(data, &results); err != nil {
		return 0, fmt.Errorf("failed to parse observe results (expected JSON array from mu observe --json): %w", err)
	}

	now := time.Now()
	rawDir := filepath.Join(dataDir, "raw", now.Format("2006"), now.Format("01"), now.Format("02"))
	if err := os.MkdirAll(rawDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create raw directory: %w", err)
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
			if s, ok := rec["_schema"].(string); ok {
				schemaCounts[resourceTypeToSchema(s)]++
			} else {
				schemaCounts["pudl/mu.#ObserveResult"]++
			}
		}
	}

	if len(allRecords) == 0 {
		return 0, nil
	}

	// Create the snapshot collection entry.
	snapshotID := fmt.Sprintf("observe_%s", now.Format("20060102_150405"))
	snapshotCollectionID, err := createObserveSnapshot(db, snapshotID, now, origin, targets, len(allRecords), schemaCounts, errors, rawDir)
	if err != nil {
		return 0, err
	}

	// Ingest each record as a member of the snapshot.
	ingested := 0
	for i, tr := range allRecords {
		n, err := ingestObserveRecord(db, tr.record, tr.target, origin, rawDir, now, i, snapshotCollectionID)
		if err != nil {
			return ingested, err
		}
		ingested += n
	}

	return ingested, nil
}

// createObserveSnapshot creates the collection entry for an observe run.
func createObserveSnapshot(
	db *database.CatalogDB,
	snapshotID string,
	now time.Time,
	origin string,
	targets []string,
	recordCount int,
	schemaCounts map[string]int,
	errors []map[string]string,
	rawDir string,
) (string, error) {
	// Build schema summary.
	var schemaSummary []map[string]any
	for schema, count := range schemaCounts {
		schemaSummary = append(schemaSummary, map[string]any{
			"schema": schema,
			"count":  count,
		})
	}

	snapshot := map[string]any{
		"snapshot_id":    snapshotID,
		"timestamp":      now.Format(time.RFC3339),
		"origin":         origin,
		"targets":        targets,
		"record_count":   recordCount,
		"schema_summary": schemaSummary,
	}
	if len(errors) > 0 {
		snapshot["errors"] = errors
	}

	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return "", fmt.Errorf("failed to marshal snapshot: %w", err)
	}
	hash := sha256.Sum256(snapshotJSON)
	contentHash := fmt.Sprintf("%x", hash)

	// Store the snapshot JSON.
	filename := fmt.Sprintf("%s_snapshot.json", snapshotID)
	storedPath := filepath.Join(rawDir, filename)
	if err := os.WriteFile(storedPath, snapshotJSON, 0644); err != nil {
		return "", fmt.Errorf("failed to write snapshot: %w", err)
	}

	// Dedup: if a snapshot with this content hash already exists, return it.
	existingSnapshot, err := db.GetEntry(contentHash)
	if err == nil && existingSnapshot != nil {
		return contentHash, nil
	}

	schema := "pudl/mu.#ObserveSnapshot"
	resourceID := identity.ComputeResourceID(schema, map[string]any{"snapshot_id": snapshotID}, contentHash)
	entryType := "observe"
	collectionType := "collection"

	entry := database.CatalogEntry{
		ID:             contentHash,
		StoredPath:     storedPath,
		ImportTimestamp: now,
		Format:         "json",
		Origin:         origin,
		Schema:         schema,
		Confidence:     1.0,
		RecordCount:    recordCount,
		SizeBytes:      int64(len(snapshotJSON)),
		EntryType:      &entryType,
		ResourceID:     &resourceID,
		ContentHash:    &contentHash,
		CollectionType: &collectionType,
	}

	if err := db.AddEntry(entry); err != nil {
		return "", fmt.Errorf("failed to add snapshot entry: %w", err)
	}

	return contentHash, nil
}

// ingestObserveRecord stores a single observe record in the catalog.
// Returns 1 if ingested, 0 if deduplicated, or an error.
func ingestObserveRecord(
	db *database.CatalogDB,
	record map[string]any,
	target string,
	origin string,
	rawDir string,
	now time.Time,
	index int,
	collectionID string,
) (int, error) {
	// Determine schema from _schema field, falling back to generic observe result.
	schema := "pudl/mu.#ObserveResult"
	if declaredSchema, ok := record["_schema"].(string); ok && declaredSchema != "" {
		schema = resourceTypeToSchema(declaredSchema)
	}

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
		return 0, nil
	}

	// Store raw JSON.
	safeTarget := strings.ReplaceAll(target, "/", "--")
	filename := fmt.Sprintf("%s_observe_%s_%d.json", now.Format("20060102_150405"), safeTarget, index)
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
	resourceID := identity.ComputeResourceID(schema, identityValues, contentHash)

	entryType := "observe"
	collectionType := "item"
	itemID := fmt.Sprintf("%s_item_%d", safeTarget, index)
	entry := database.CatalogEntry{
		ID:              contentHash,
		StoredPath:      storedPath,
		ImportTimestamp:  now,
		Format:          "json",
		Origin:          origin,
		Schema:          schema,
		Confidence:      1.0,
		RecordCount:     1,
		SizeBytes:       int64(len(recordJSON)),
		EntryType:       &entryType,
		Definition:      &target,
		ResourceID:      &resourceID,
		ContentHash:     &contentHash,
		CollectionID:    &collectionID,
		CollectionType:  &collectionType,
		ItemIndex:       &index,
		ItemID:          &itemID,
	}

	if err := db.AddEntry(entry); err != nil {
		return 0, fmt.Errorf("failed to add observe entry: %w", err)
	}

	return 1, nil
}

// resourceTypeToSchema converts a _schema resource type like "linux.host" to a
// pudl schema path like "pudl/linux.#Host".
func resourceTypeToSchema(resourceType string) string {
	parts := strings.SplitN(resourceType, ".", 2)
	if len(parts) != 2 {
		return "pudl/mu.#ObserveResult"
	}
	pkg := parts[0]
	name := parts[1]
	if len(name) > 0 {
		name = strings.ToUpper(name[:1]) + name[1:]
	}
	// Handle underscored names: "network_interface" -> "NetworkInterface"
	for {
		idx := strings.Index(name, "_")
		if idx < 0 || idx >= len(name)-1 {
			break
		}
		name = name[:idx] + strings.ToUpper(name[idx+1:idx+2]) + name[idx+2:]
	}
	return fmt.Sprintf("pudl/%s.#%s", pkg, name)
}
