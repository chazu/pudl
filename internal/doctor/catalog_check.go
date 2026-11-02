package doctor

import (
	"context"
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

// CatalogDiagnostic is one catalog entry's validation result: "ok",
// "invalid" against its schema, "mismatch" with what inference now selects,
// or "error" when it could not be checked.
type CatalogDiagnostic struct {
	ID               string `json:"id"`
	Proquint         string `json:"proquint"`
	Schema           string `json:"schema"`
	Status           string `json:"status"`
	Error            string `json:"error,omitempty"`
	InferredSchema   string `json:"inferred_schema,omitempty"`
	InferenceChecked bool   `json:"inference_checked"`
}

// CatalogScan selects what CheckCatalogEntries checks: the catalog beside
// DataPath, validated against the schemas in SchemaPaths, either one entry
// (EntryID, by proquint or full ID) or every entry Filter selects.
type CatalogScan struct {
	DataPath    string
	SchemaPaths []string
	EntryID     string
	Filter      database.FilterOptions
}

// CheckCatalogEntries validates the entries in scope, streaming them, and
// scans each loaded payload for likely secrets. Current facts are scanned too
// unless an entry, schema or origin scopes the check to catalog entries. A
// missing catalog has nothing to check, and is not created.
func CheckCatalogEntries(ctx context.Context, scan CatalogScan) ([]CatalogDiagnostic, *SecretReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	entryID, filter := scan.EntryID, scan.Filter
	catalogRoot := filepath.Dir(scan.DataPath)
	if _, err := os.Stat(filepath.Join(catalogRoot, "data", "sqlite", "catalog.db")); os.IsNotExist(err) && entryID == "" {
		return []CatalogDiagnostic{}, &SecretReport{FactsScanned: true, Findings: []SecretFinding{}}, nil
	}
	db, err := database.OpenCatalogDBReadOnly(catalogRoot)
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()
	paths := scan.SchemaPaths
	validation, err := validator.NewValidationService(paths...)
	if err != nil {
		return nil, nil, err
	}
	inferrer, err := inference.Shared(paths...)
	if err != nil {
		return nil, nil, err
	}
	secretScan := NewSecretScan(inferrer)
	out := []CatalogDiagnostic{}
	check := func(entry database.CatalogEntry) error {
		out = append(out, checkCatalogEntry(entry, validation, inferrer, secretScan))
		return nil
	}
	if entryID != "" {
		entry, lookupErr := db.GetEntryByProquint(entryID)
		if lookupErr != nil {
			entry, lookupErr = db.GetEntry(entryID)
		}
		if lookupErr != nil {
			return nil, nil, fmt.Errorf("entry %q not found: %w", entryID, lookupErr)
		}
		_ = check(*entry)
	} else if err := db.EachEntryContext(ctx, filter, check); err != nil {
		return nil, nil, err
	}
	if entryID == "" && filter.Schema == "" && filter.Origin == "" {
		if err := secretScan.Facts(ctx, db); err != nil {
			return nil, nil, err
		}
	}
	report := secretScan.Report()
	return out, &report, nil
}

// checkCatalogEntry validates one entry, re-checks its inference, and scans
// its payload for likely secrets.
func checkCatalogEntry(entry database.CatalogEntry, validation *validator.ValidationService, inferrer *inference.SchemaInferrer, secretScan *SecretScan) CatalogDiagnostic {
	diagnostic := CatalogDiagnostic{ID: entry.ID, Proquint: idgen.HashToProquint(entry.ID), Schema: entry.Schema, Status: "ok"}
	data, loadErr := loadVerifyData(entry.StoredPath)
	if loadErr == nil {
		secretScan.Entry(entry, data)
	}
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
	return diagnostic
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

// DriftEntryIDs lists the entries whose schema inference now selects another
// schema: what the inference repair re-infers after a scan.
func DriftEntryIDs(catalog []CatalogDiagnostic) []string {
	var ids []string
	for _, entry := range catalog {
		if entry.Status == "mismatch" {
			ids = append(ids, entry.ID)
		}
	}
	return ids
}
