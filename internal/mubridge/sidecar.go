package mubridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// SchemaSidecar is the on-disk JSON shape that mu (or a human/agent)
// writes alongside a data file to declare the data's CUE schema.
//
// The wire format is intentionally compatible with the SchemaRef shape
// emitted by mu plugins via their discover response — same fields,
// same JSON keys.
//
// File naming convention: for a data file at "<path>", the sidecar
// lives at "<path>.schema.json".
//
// Optionally, the sidecar may carry the schema's CUE definition files
// inline so pudl can auto-register an unknown ref on first sight (the
// brainstorm's "schema travels with the data" tier). Each Definition
// is a slash-separated relative path inside the (module, version)
// directory plus the file's bytes; pudl writes them into its schema
// cache under <pudlDir>/schemas/<module>/<version>/<rel_path>.
type SchemaSidecar struct {
	Module      string                  `json:"module"`
	Version     string                  `json:"version"`
	Definition  string                  `json:"definition,omitempty"` // optional, e.g. "#EC2Instance"
	Source      string                  `json:"source,omitempty"`     // advisory: "vendored" | "remote"
	Definitions []SidecarDefinitionFile `json:"definitions,omitempty"`
}

// SidecarDefinitionFile is one CUE source file carried inline with the
// sidecar. The Path is slash-separated, relative to the module's
// version directory, e.g. "ec2.cue" or "vpc/vpc.cue".
type SidecarDefinitionFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// CanonicalRef returns the schema_ref string used in the
// item_schemas catalog table: "<module>@<version>" or
// "<module>@<version>#<definition>".
func (s SchemaSidecar) CanonicalRef() string {
	if s.Module == "" || s.Version == "" {
		return ""
	}
	ref := s.Module + "@" + s.Version
	if s.Definition != "" {
		ref += s.Definition
	}
	return ref
}

// SidecarPath returns the conventional path for a sidecar file given
// the path of its data file.
func SidecarPath(dataPath string) string { return dataPath + ".schema.json" }

// ReadSidecar reads and parses the sidecar at dataPath+".schema.json".
// Returns (nil, nil) if no sidecar exists. Returns an error if the
// file exists but is malformed or missing required fields.
func ReadSidecar(dataPath string) (*SchemaSidecar, error) {
	p := SidecarPath(dataPath)
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sidecar %s: %w", p, err)
	}
	var s SchemaSidecar
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse sidecar %s: %w", p, err)
	}
	if s.Module == "" || s.Version == "" {
		return nil, fmt.Errorf("sidecar %s: module and version are required", p)
	}
	return &s, nil
}
