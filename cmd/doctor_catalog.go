package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/validator"
)

type catalogDiagnostic struct {
	ID               string `json:"id"`
	Proquint         string `json:"proquint"`
	Schema           string `json:"schema"`
	Status           string `json:"status"`
	Error            string `json:"error,omitempty"`
	InferredSchema   string `json:"inferred_schema,omitempty"`
	InferenceChecked bool   `json:"inference_checked"`
}

func checkCatalogEntries(entryID string) ([]catalogDiagnostic, error) {
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return nil, err
	}
	catalogRoot := filepath.Dir(cfg.DataPath)
	if _, err := os.Stat(filepath.Join(catalogRoot, "data", "sqlite", "catalog.db")); os.IsNotExist(err) && entryID == "" {
		return []catalogDiagnostic{}, nil
	}
	db, err := database.OpenCatalogDBReadOnly(catalogRoot)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	paths := effectiveSchemaPaths(cfg)
	validation, err := validator.NewValidationService(paths...)
	if err != nil {
		return nil, err
	}
	inferrer, err := inference.Shared(paths...)
	if err != nil {
		return nil, err
	}
	var entries []database.CatalogEntry
	if entryID != "" {
		entry, lookupErr := db.GetEntryByProquint(entryID)
		if lookupErr != nil {
			entry, lookupErr = db.GetEntry(entryID)
		}
		if lookupErr != nil {
			return nil, fmt.Errorf("entry %q not found: %w", entryID, lookupErr)
		}
		entries = append(entries, *entry)
	} else {
		result, queryErr := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{Limit: 0, SortBy: "timestamp"})
		if queryErr != nil {
			return nil, queryErr
		}
		entries = result.Entries
	}
	out := make([]catalogDiagnostic, 0, len(entries))
	for _, entry := range entries {
		diagnostic := catalogDiagnostic{ID: entry.ID, Proquint: idgen.HashToProquint(entry.ID), Schema: entry.Schema, Status: "ok"}
		data, loadErr := loadVerifyData(entry.StoredPath)
		if loadErr != nil {
			diagnostic.Status, diagnostic.Error = "error", loadErr.Error()
		} else if result := validation.ValidateDataAgainstSchema(data, entry.Schema); !result.Valid {
			diagnostic.Status, diagnostic.Error = "invalid", result.ErrorMessage
			if diagnostic.Error == "" {
				diagnostic.Error = strings.Join(result.Errors, "; ")
			}
		} else if shouldVerifyInference(data, entry) {
			diagnostic.InferenceChecked = true
			collectionType := ""
			if entry.CollectionType != nil {
				collectionType = *entry.CollectionType
			}
			result, inferErr := inferrer.Infer(data, inference.InferenceHints{Origin: entry.Origin, Format: entry.Format, CollectionType: collectionType})
			if inferErr != nil {
				diagnostic.Status, diagnostic.Error = "error", inferErr.Error()
			} else if !schemaname.IsEquivalent(result.Schema, entry.Schema) {
				diagnostic.Status, diagnostic.InferredSchema = "mismatch", result.Schema
			}
		}
		out = append(out, diagnostic)
	}
	return out, nil
}

// Explicit assignments are validated, never heuristically reclassified.
func shouldVerifyInference(data interface{}, entry database.CatalogEntry) bool {
	if entry.EntryType != nil || entry.CollectionType != nil {
		return false
	}
	if record, ok := data.(map[string]interface{}); ok {
		if declared, ok := record["_schema"].(string); ok && strings.TrimSpace(declared) != "" {
			return false
		}
	}
	return true
}

func loadVerifyData(path string) (interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var value interface{}
	if err := json.Unmarshal(data, &value); err != nil {
		return string(data), nil
	}
	return value, nil
}
