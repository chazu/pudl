package mubridge

import (
	"bufio"
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

// ObserveInput represents a single observe result from mu.
type ObserveInput struct {
	Target string                 `json:"target"`
	State  map[string]interface{} `json:"state"`
	Diff   map[string]interface{} `json:"diff,omitempty"`
}

// IngestObserveResults processes mu observe output and stores results in the catalog.
// Returns the number of results ingested and any error.
func IngestObserveResults(db *database.CatalogDB, reader io.Reader, origin string, dataDir string) (int, error) {
	if origin == "" {
		origin = "mu-observe"
	}

	scanner := bufio.NewScanner(reader)
	// Allow lines up to 10MB
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	ingested := 0
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var obs ObserveInput
		if err := json.Unmarshal([]byte(line), &obs); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping malformed JSON on line %d: %v\n", lineNum, err)
			continue
		}

		if obs.Target == "" {
			fmt.Fprintf(os.Stderr, "Warning: skipping line %d: missing target field\n", lineNum)
			continue
		}

		// Extract target name, strip leading "//" if present
		defName := obs.Target
		defName = strings.TrimPrefix(defName, "//")

		// Serialize state to JSON for hashing and storage
		stateJSON, err := json.Marshal(obs.State)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping line %d: failed to marshal state: %v\n", lineNum, err)
			continue
		}

		// Compute content hash from state JSON
		hash := sha256.Sum256(stateJSON)
		contentHash := fmt.Sprintf("%x", hash)

		// Check for dedup: if content hash matches latest observe entry for this definition, skip
		existing, err := db.GetLatestObserveByContentHash(defName, contentHash)
		if err != nil {
			return ingested, fmt.Errorf("dedup check failed for %s: %w", defName, err)
		}
		if existing != nil {
			continue // Duplicate, skip
		}

		// Store raw JSON in data/raw/YYYY/MM/DD/
		now := time.Now()
		rawDir := filepath.Join(dataDir, "raw", now.Format("2006"), now.Format("01"), now.Format("02"))
		if err := os.MkdirAll(rawDir, 0755); err != nil {
			return ingested, fmt.Errorf("failed to create raw directory: %w", err)
		}

		// Write full observe result (not just state) to raw storage
		fullJSON, err := json.Marshal(obs)
		if err != nil {
			return ingested, fmt.Errorf("failed to marshal observe result: %w", err)
		}

		filename := fmt.Sprintf("%s_observe_%s.json", now.Format("20060102_150405"), defName)
		storedPath := filepath.Join(rawDir, filename)
		if err := os.WriteFile(storedPath, fullJSON, 0644); err != nil {
			return ingested, fmt.Errorf("failed to write observe result: %w", err)
		}

		// Compute resource ID from target name + schema
		schema := "pudl/mu.#ObserveResult"
		identityValues := map[string]interface{}{"target": defName}
		resourceID := identity.ComputeResourceID(schema, identityValues, contentHash)

		// Create catalog entry
		entryType := "observe"
		entry := database.CatalogEntry{
			ID:             contentHash, // Use content hash as the entry ID
			StoredPath:     storedPath,
			MetadataPath:   "",
			ImportTimestamp: now,
			Format:         "json",
			Origin:         origin,
			Schema:         schema,
			Confidence:     1.0,
			RecordCount:    1,
			SizeBytes:      int64(len(fullJSON)),
			EntryType:      &entryType,
			Definition:     &defName,
			ResourceID:     &resourceID,
			ContentHash:    &contentHash,
		}

		if err := db.AddEntry(entry); err != nil {
			return ingested, fmt.Errorf("failed to add observe entry for %s: %w", defName, err)
		}

		ingested++
	}

	if err := scanner.Err(); err != nil {
		return ingested, fmt.Errorf("error reading input: %w", err)
	}

	return ingested, nil
}
