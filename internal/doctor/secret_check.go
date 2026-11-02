package doctor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/redact"
	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/secrets"
	"github.com/chazu/pudl/internal/secretscan"
)

// MaxSecretFindings bounds the findings a report keeps; Total counts them all.
const MaxSecretFindings = 500

// SecretFinding is one likely secret in stored data: where it is and what it
// looks like, never the value.
type SecretFinding struct {
	Subject  string       `json:"subject"` // "entry" or "fact"
	ID       string       `json:"id"`
	Proquint string       `json:"proquint,omitempty"`
	Schema   string       `json:"schema,omitempty"`
	Relation string       `json:"relation,omitempty"`
	Path     string       `json:"path"`
	Kind     secrets.Kind `json:"kind"`
	Fix      string       `json:"fix"`
}

// SecretReport tallies a scan of stored payloads and current facts.
type SecretReport struct {
	Entries      int             `json:"entries"`       // payloads scanned
	Facts        int             `json:"facts"`         // current facts scanned
	FactsScanned bool            `json:"facts_scanned"` // false when the scan was scoped to catalog entries
	Total        int             `json:"total"`
	Findings     []SecretFinding `json:"findings"`
}

// SecretScan finds likely secrets in stored data outside what schemas cover
// (sensitive_fields and secret_scan_ignore). It only reports.
type SecretScan struct {
	scanner *secretscan.Scanner
	report  SecretReport
}

// NewSecretScan scans against the schemas src loads.
func NewSecretScan(src redact.Source) *SecretScan {
	return &SecretScan{scanner: secretscan.New(src, nil), report: SecretReport{Findings: []SecretFinding{}}}
}

// Entry scans one entry's stored payload, decoded by the caller as JSON when
// possible. Other formats are decoded with the importer. Collection parents
// are skipped: each of their items is an entry of its own.
func (s *SecretScan) Entry(entry database.CatalogEntry, data any) {
	if entry.CollectionType != nil && *entry.CollectionType == "collection" {
		return
	}
	if _, raw := data.(string); raw && entry.Format != "json" {
		if decoded, err := importer.DecodeStoredFile(entry.StoredPath, entry.Format); err == nil {
			data = decoded
		}
	}
	s.report.Entries++
	for _, f := range s.scanner.Scan(data, entry.Schema) {
		s.add(SecretFinding{Subject: "entry", ID: entry.ID, Proquint: idgen.HashToProquint(entry.ID),
			Schema: entry.Schema, Path: f.Path, Kind: f.Kind, Fix: entryFix(entry.Schema, f.Path)})
	}
}

// Facts scans the args of every current fact except projection-owned ones:
// projection copies (already redacted) values from a cataloged payload, which
// Entry scans under the schema's coverage. Scanning the copy would report the
// same value again, under a fact path that secret_scan_ignore cannot reach.
// The reserved catalog_entry relation is a view, never stored as facts.
func (s *SecretScan) Facts(ctx context.Context, db *database.CatalogDB) error {
	s.report.FactsScanned = true
	return db.EachCurrentFactContext(ctx, func(f database.Fact) error {
		if database.IsProjectionSource(f.Source) {
			return nil
		}
		var args any = f.Args // undecodable args are scanned as one string
		_ = json.Unmarshal([]byte(f.Args), &args)
		s.report.Facts++
		for _, finding := range secrets.Scan(args, secrets.Options{}) {
			s.add(SecretFinding{Subject: "fact", ID: f.ID, Relation: f.Relation, Path: finding.Path, Kind: finding.Kind,
				Fix: "Fact args have no schema, so sensitive_fields and secret_scan_ignore do not apply: rotate the credential and retract the fact (pudl facts retract " + f.ID + "); fact history keeps the value"})
		}
		return nil
	})
}

// Report returns the tally so far.
func (s *SecretScan) Report() SecretReport { return s.report }

func (s *SecretScan) add(f SecretFinding) {
	s.report.Total++
	if len(s.report.Findings) < MaxSecretFindings {
		s.report.Findings = append(s.report.Findings, f)
	}
}

func entryFix(schema, path string) string {
	fix := fmt.Sprintf("Declare %s in %s's _pudl.sensitive_fields", path, schema)
	if schemaname.IsFallbackSchema(schema) {
		fix = fmt.Sprintf("Route the record to a schema of its own (import --schema), then declare %s in that schema's _pudl.sensitive_fields", path)
	}
	return fix + " (then pudl doctor --fix --only sensitive-scrub redacts the stored plaintext; rotate the credential), or add it to _pudl.secret_scan_ignore if it is not a secret"
}
