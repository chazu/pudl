package artifact

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"pudl/internal/database"
	"pudl/internal/idgen"
)

// StoreOptions configures artifact storage.
type StoreOptions struct {
	Definition string
	Method     string
	Output     interface{}
	Tags       map[string]string
	DataPath   string // Root data directory (e.g. ~/.pudl/data)
}

// StoreResult contains the result of storing an artifact.
type StoreResult struct {
	ID       string
	Proquint string
	Path     string
	Deduped  bool
}

// Store serializes a method output to JSON, content-hashes it, and creates
// a catalog entry. Returns the proquint identifier.
func Store(db *database.CatalogDB, opts StoreOptions) (*StoreResult, error) {
	// Serialize output to JSON
	outputJSON, err := json.MarshalIndent(opts.Output, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize artifact output: %w", err)
	}

	// Content hash
	contentHash := idgen.ComputeContentID(outputJSON)

	// Dedup check
	existing, err := db.FindByContentHash(contentHash)
	if err != nil {
		return nil, fmt.Errorf("failed to check for duplicate: %w", err)
	}
	if existing != nil {
		return &StoreResult{
			ID:       existing.ID,
			Proquint: idgen.HashToProquint(existing.ID),
			Path:     existing.StoredPath,
			Deduped:  true,
		}, nil
	}

	// Build run_id: SHA256 of definition|method|timestamp
	now := time.Now()
	runIDInput := fmt.Sprintf("%s|%s|%s", opts.Definition, opts.Method, now.Format(time.RFC3339Nano))
	runIDHash := sha256.Sum256([]byte(runIDInput))
	runID := fmt.Sprintf("%x", runIDHash)

	// Write artifact file
	artifactDir := filepath.Join(opts.DataPath, "artifacts", opts.Definition, opts.Method)
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create artifact directory: %w", err)
	}

	filename := fmt.Sprintf("%s-%s.json", now.Format("20060102T150405"), contentHash[:16])
	artifactPath := filepath.Join(artifactDir, filename)

	if err := os.WriteFile(artifactPath, outputJSON, 0644); err != nil {
		return nil, fmt.Errorf("failed to write artifact file: %w", err)
	}

	// Write .meta sidecar
	meta := map[string]interface{}{
		"definition": opts.Definition,
		"method":     opts.Method,
		"run_id":     runID,
		"timestamp":  now.Format(time.RFC3339),
		"tags":       opts.Tags,
	}
	metaJSON, _ := json.MarshalIndent(meta, "", "  ")
	metaPath := artifactPath + ".meta"
	if err := os.WriteFile(metaPath, metaJSON, 0644); err != nil {
		return nil, fmt.Errorf("failed to write artifact metadata: %w", err)
	}

	// Serialize tags for DB storage
	var tagsStr *string
	if len(opts.Tags) > 0 {
		tj, _ := json.Marshal(opts.Tags)
		s := string(tj)
		tagsStr = &s
	}

	entryType := "artifact"
	entry := database.CatalogEntry{
		ID:             contentHash,
		StoredPath:     artifactPath,
		MetadataPath:   metaPath,
		ImportTimestamp: now,
		Format:         "json",
		Origin:         fmt.Sprintf("method:%s.%s", opts.Definition, opts.Method),
		Schema:         "pudl/artifact",
		Confidence:     1.0,
		RecordCount:    1,
		SizeBytes:      int64(len(outputJSON)),
		ContentHash:    &contentHash,
		EntryType:      &entryType,
		Definition:     &opts.Definition,
		Method:         &opts.Method,
		RunID:          &runID,
		Tags:           tagsStr,
	}

	if err := db.AddEntry(entry); err != nil {
		return nil, fmt.Errorf("failed to add artifact catalog entry: %w", err)
	}

	return &StoreResult{
		ID:       contentHash,
		Proquint: idgen.HashToProquint(contentHash),
		Path:     artifactPath,
	}, nil
}
