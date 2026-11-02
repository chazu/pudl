package importer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/chazu/pudl/internal/artifacts"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/idgen"
)

// An entry's schema is explicit when a producer or the user chose it, and
// inferred otherwise. Inference repairs re-check only inferred entries.
//
// For imports the record is the entry's .meta: an import with --schema stores
// the requested schema as schema_info.intended_schema, and how the record
// validated against it as schema_info.validation_status. Imports made before
// intended_schema was recorded are recognised by their validation status.

// explicitStatuses are the validation statuses only a --schema import records.
var explicitStatuses = map[string]bool{"validated": true, "fallback": true, "unverified": true}

// ExplicitAssignment reports whether entry's schema was assigned explicitly
// rather than inferred: a producer entry (any entry_type), a record that
// declares its own _schema, or an import whose .meta records a requested
// --schema. data is the entry's decoded payload.
func ExplicitAssignment(entry database.CatalogEntry, data any) bool {
	if entry.EntryType != nil {
		return true
	}
	if record, ok := data.(map[string]any); ok {
		if declared, ok := record["_schema"].(string); ok && strings.TrimSpace(declared) != "" {
			return true
		}
	}
	info := readSchemaInfo(entry.MetadataPath)
	return info.IntendedSchema != "" || explicitStatuses[info.ValidationStatus]
}

// readSchemaInfo returns the schema_info of a .meta file, or the zero value
// when there is none.
func readSchemaInfo(path string) SchemaInfo {
	var meta struct {
		SchemaInfo SchemaInfo `json:"schema_info"`
	}
	if path == "" {
		return meta.SchemaInfo
	}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &meta)
	}
	return meta.SchemaInfo
}

// reassignedMeta is an existing entry's .meta to rewrite after a re-import
// moved the entry to an explicit schema.
type reassignedMeta struct {
	path string
	info SchemaInfo
}

// recordReassignment replaces the schema_info of an existing entry's .meta
// with the explicit assignment a re-import moved it to, so the move is
// recorded as explicit like a first import with --schema. Other sections are
// kept. Call after the catalog transaction commits.
func recordReassignment(r reassignedMeta) error {
	if r.path == "" {
		return nil
	}
	raw, err := os.ReadFile(r.path)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	decoded, err := idgen.DecodeJSONExact(raw)
	if err != nil {
		return err
	}
	meta, ok := decoded.(map[string]any)
	if !ok {
		return nil
	}
	meta["schema_info"] = r.info
	encoded, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(r.path), "."+filepath.Base(r.path)+".tmp")
	if err := artifacts.WriteFilePrivate(tmp, encoded); err != nil {
		return err
	}
	if err := os.Rename(tmp, r.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
