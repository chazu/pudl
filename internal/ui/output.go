package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// OutputFormat represents the output format type
type OutputFormat string

const (
	OutputFormatText OutputFormat = "text"
	OutputFormatJSON OutputFormat = "json"
)

// OutputWriter handles structured output in different formats
type OutputWriter struct {
	Format OutputFormat
	Writer io.Writer
	Pretty bool
}

// NewOutputWriter creates a new output writer
func NewOutputWriter(format OutputFormat, pretty bool) *OutputWriter {
	return &OutputWriter{
		Format: format,
		Writer: os.Stdout,
		Pretty: pretty,
	}
}

// WriteJSON writes data as JSON to the output
func (w *OutputWriter) WriteJSON(data interface{}) error {
	encoder := json.NewEncoder(w.Writer)
	if w.Pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(data)
}

// WriteText writes a formatted string to the output
func (w *OutputWriter) WriteText(format string, args ...interface{}) {
	fmt.Fprintf(w.Writer, format, args...)
}

// WriteLine writes a line to the output
func (w *OutputWriter) WriteLine(s string) {
	fmt.Fprintln(w.Writer, s)
}

// Write writes data based on the current format
// For JSON format, it writes the jsonData
// For text format, it calls the textFunc
func (w *OutputWriter) Write(jsonData interface{}, textFunc func()) error {
	if w.Format == OutputFormatJSON {
		return w.WriteJSON(jsonData)
	}
	textFunc()
	return nil
}

// ListOutput represents structured output for list command
type ListOutput struct {
	Entries      []EntryOutput `json:"entries"`
	TotalEntries int           `json:"total_entries"`
	TotalPages   int           `json:"total_pages"`
	CurrentPage  int           `json:"current_page"`
	Summary      *ListSummary  `json:"summary,omitempty"`
}

// ListSummary contains aggregate statistics
type ListSummary struct {
	TotalSize     int64    `json:"total_size_bytes"`
	TotalRecords  int      `json:"total_records"`
	UniqueSchemas []string `json:"unique_schemas,omitempty"`
	UniqueOrigins []string `json:"unique_origins,omitempty"`
	UniqueFormats []string `json:"unique_formats,omitempty"`
}

// EntryOutput represents a single entry in JSON output
type EntryOutput struct {
	ID              string   `json:"id"`
	Proquint        string   `json:"proquint"`
	Schema          string   `json:"schema"`
	Origin          string   `json:"origin"`
	Format          string   `json:"format"`
	SizeBytes       int64    `json:"size_bytes"`
	RecordCount     int      `json:"record_count"`
	ImportTimestamp string   `json:"import_timestamp"`
	StoredPath      string   `json:"stored_path"`
	MetadataPath    string   `json:"metadata_path"`
	Confidence      float64  `json:"confidence"`
	CollectionType  *string  `json:"collection_type,omitempty"`
	CollectionID    *string  `json:"collection_id,omitempty"`
	ItemID          *string  `json:"item_id,omitempty"`
	ItemIndex       *int     `json:"item_index,omitempty"`
}

// ImportOutput represents structured output for import command
type ImportOutput struct {
	Success     bool              `json:"success"`
	ID          string            `json:"id"`
	Proquint    string            `json:"proquint,omitempty"`
	Schema      string            `json:"schema"`
	Origin      string            `json:"origin"`
	Format      string            `json:"format"`
	RecordCount int               `json:"record_count"`
	SizeBytes   int64             `json:"size_bytes"`
	StoredPath  string            `json:"stored_path"`
	Validation  *ValidationOutput `json:"validation,omitempty"`
}

// ValidationOutput represents validation results in JSON
type ValidationOutput struct {
	IsCompliant    bool     `json:"is_compliant"`
	AssignedSchema string   `json:"assigned_schema"`
	Errors         []string `json:"errors,omitempty"`
}

// ShowOutput represents structured output for show command
type ShowOutput struct {
	Entry    EntryOutput     `json:"entry"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// DoctorOutput represents structured output for doctor command
type DoctorOutput struct {
	Healthy bool                `json:"healthy"`
	Checks  []DoctorCheckOutput `json:"checks"`
}

// DoctorCheckOutput represents a single health check result
type DoctorCheckOutput struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
	Fix     string `json:"fix,omitempty"`
}

// SchemaNewOutput represents structured output for schema new command
type SchemaNewOutput struct {
	Success                bool                      `json:"success"`
	FilePath               string                    `json:"file_path"`
	PackageName            string                    `json:"package_name"`
	DefinitionName         string                    `json:"definition_name"`
	FieldCount             int                       `json:"field_count"`
	InferredIdentityFields []string                  `json:"inferred_identity_fields,omitempty"`
	IsCollection           bool                      `json:"is_collection"`
	NewItemSchemas         []SchemaNewItemOutput     `json:"new_item_schemas,omitempty"`
	ExistingSchemaRefs     []string                  `json:"existing_schema_refs,omitempty"`
}

// SchemaNewItemOutput represents a generated item schema in collection generation
type SchemaNewItemOutput struct {
	FilePath       string `json:"file_path"`
	DefinitionName string `json:"definition_name"`
	FieldCount     int    `json:"field_count"`
}
