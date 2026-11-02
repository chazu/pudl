package secretscan

import "github.com/chazu/pudl/internal/secrets"

// MaxWarnings caps the findings a Tally lists; Total still counts every one.
const MaxWarnings = 50

// Warning is a likely secret found in an ingested record outside its schema's
// sensitive-field coverage. It names the record, schema, path and kind —
// never the value.
type Warning struct {
	// Record is the record's index in its collection or observation (0 for a
	// document).
	Record int    `json:"record"`
	Schema string `json:"schema"`
	secrets.Finding
}

// Tally accumulates findings across an ingest's records.
type Tally struct {
	Warnings []Warning
	Total    int
}

// Note records the findings for one record.
func (t *Tally) Note(record int, schema string, findings []secrets.Finding) {
	for _, f := range findings {
		t.Total++
		if len(t.Warnings) < MaxWarnings {
			t.Warnings = append(t.Warnings, Warning{Record: record, Schema: schema, Finding: f})
		}
	}
}
