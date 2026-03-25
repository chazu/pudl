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
	"pudl/internal/idgen"
)

// ManifestInput represents a mu build manifest.
type ManifestInput struct {
	Timestamp string           `json:"timestamp"`
	Summary   ManifestSummary  `json:"summary"`
	Actions   []ManifestAction `json:"actions"`
}

// ManifestSummary contains aggregate counts from a mu build.
type ManifestSummary struct {
	Total    int `json:"total"`
	Cached   int `json:"cached"`
	Executed int `json:"executed"`
	Failed   int `json:"failed"`
}

// ManifestAction represents a single action from a mu build manifest.
type ManifestAction struct {
	ID       string            `json:"id"`
	Target   string            `json:"target,omitempty"`
	Cached   bool              `json:"cached"`
	ExitCode int               `json:"exit_code"`
	Outputs  map[string]string `json:"outputs,omitempty"`
}

// IngestManifestResult contains summary information about the ingestion.
type IngestManifestResult struct {
	RunID   string
	Total   int
	Cached  int
	Failed  int
	Skipped bool
}

// IngestManifest processes a mu build manifest and stores results in the catalog.
// Returns the run_id assigned to this manifest and any error.
func IngestManifest(db *database.CatalogDB, reader io.Reader, origin string, configDir string) (*IngestManifestResult, error) {
	// Read entire JSON from reader
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest data: %w", err)
	}

	var manifest ManifestInput
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest JSON: %w", err)
	}

	// Generate deterministic content hash from raw data
	hash := sha256.Sum256(data)
	contentHash := fmt.Sprintf("%x", hash)

	// Content-hash dedup: skip if manifest with same content hash already ingested
	existing, err := db.FindByContentHash(contentHash)
	if err != nil {
		return nil, fmt.Errorf("failed to check for duplicate manifest: %w", err)
	}
	if existing != nil {
		runID := ""
		if existing.RunID != nil {
			runID = *existing.RunID
		}
		return &IngestManifestResult{
			RunID:   runID,
			Total:   manifest.Summary.Total,
			Cached:  manifest.Summary.Cached,
			Failed:  manifest.Summary.Failed,
			Skipped: true,
		}, nil
	}

	// Generate run_id from timestamp + content hash (deterministic)
	runIDSource := manifest.Timestamp + ":" + contentHash
	runIDHash := sha256.Sum256([]byte(runIDSource))
	runID := fmt.Sprintf("%x", runIDHash)

	// Store the raw manifest JSON in the data directory
	manifestStoredPath, err := storeRawData(configDir, data, "manifest.json")
	if err != nil {
		return nil, fmt.Errorf("failed to store manifest data: %w", err)
	}

	// Use the content hash as the catalog entry ID
	manifestID := idgen.ComputeContentID(data)

	// Create the manifest catalog entry
	entryType := "manifest"
	format := "json"
	schema := "pudl/mu.#Manifest"
	now := time.Now()

	manifestEntry := database.CatalogEntry{
		ID:              manifestID,
		StoredPath:      manifestStoredPath,
		MetadataPath:    manifestStoredPath + ".meta",
		ImportTimestamp:  now,
		Format:          format,
		Origin:          origin,
		Schema:          schema,
		Confidence:      1.0,
		RecordCount:     1,
		SizeBytes:       int64(len(data)),
		ContentHash:     &contentHash,
		EntryType:       &entryType,
		RunID:           &runID,
	}

	if err := db.AddEntry(manifestEntry); err != nil {
		return nil, fmt.Errorf("failed to add manifest entry: %w", err)
	}

	// Create per-action entries
	actionEntryType := "manifest-action"
	actionSchema := "pudl/mu.#ManifestAction"

	for _, action := range manifest.Actions {
		// Extract definition name from target (strip //)
		defName := targetToDefinition(action.Target)

		// Build tags JSON
		tags, err := json.Marshal(map[string]interface{}{
			"exit_code": action.ExitCode,
			"cached":    action.Cached,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal action tags: %w", err)
		}
		tagsStr := string(tags)

		// Store action JSON in raw data directory
		actionData, err := json.Marshal(action)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal action data: %w", err)
		}

		actionStoredPath, err := storeRawData(configDir, actionData, defName+"_action.json")
		if err != nil {
			return nil, fmt.Errorf("failed to store action data: %w", err)
		}

		// Generate action entry ID from action data content
		actionID := idgen.ComputeContentID(append([]byte(runID+":"), actionData...))
		actionContentHash := idgen.ComputeContentID(actionData)

		actionEntry := database.CatalogEntry{
			ID:              actionID,
			StoredPath:      actionStoredPath,
			MetadataPath:    actionStoredPath + ".meta",
			ImportTimestamp:  now,
			Format:          format,
			Origin:          origin,
			Schema:          actionSchema,
			Confidence:      1.0,
			RecordCount:     1,
			SizeBytes:       int64(len(actionData)),
			ContentHash:     &actionContentHash,
			EntryType:       &actionEntryType,
			Definition:      &defName,
			RunID:           &runID,
			Tags:            &tagsStr,
		}

		if err := db.AddEntry(actionEntry); err != nil {
			return nil, fmt.Errorf("failed to add action entry for %s: %w", defName, err)
		}

		// Update convergence status based on action exit code
		status := "converged"
		if action.ExitCode != 0 {
			status = "failed"
		}
		if err := db.UpdateStatus(defName, status); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update status for %s: %v\n", defName, err)
		}
	}

	return &IngestManifestResult{
		RunID:  runID,
		Total:  manifest.Summary.Total,
		Cached: manifest.Summary.Cached,
		Failed: manifest.Summary.Failed,
	}, nil
}

// targetToDefinition converts a mu target name to a definition name.
// It strips the leading "//" prefix if present.
func targetToDefinition(target string) string {
	return strings.TrimPrefix(target, "//")
}

// storeRawData writes data to the raw data directory using the standard
// YYYY/MM/DD/ date-based path structure.
func storeRawData(configDir string, data []byte, filename string) (string, error) {
	now := time.Now()
	dateDir := filepath.Join(configDir, "data", "raw",
		fmt.Sprintf("%d", now.Year()),
		fmt.Sprintf("%02d", now.Month()),
		fmt.Sprintf("%02d", now.Day()))

	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create raw data directory: %w", err)
	}

	// Use timestamp + filename to avoid collisions
	storedName := fmt.Sprintf("%s_%s", now.Format("20060102_150405"), filename)
	storedPath := filepath.Join(dateDir, storedName)

	if err := os.WriteFile(storedPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write raw data file: %w", err)
	}

	return storedPath, nil
}
