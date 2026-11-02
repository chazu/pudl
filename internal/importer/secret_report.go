package importer

import "github.com/chazu/pudl/internal/secrets"

// maxSecretWarnings caps the likely-secret findings an import result lists;
// SecretWarningsTotal still counts every one.
const maxSecretWarnings = 50

// SecretWarning is a likely secret found in an imported record outside its
// schema's sensitive-field coverage. It names the record, schema, path and
// kind — never the value.
type SecretWarning struct {
	// Record is the record's index in a collection (0 for a document).
	Record int    `json:"record"`
	Schema string `json:"schema"`
	secrets.Finding
}

// secretTally accumulates findings across an import's records.
type secretTally struct {
	warnings []SecretWarning
	total    int
}

// note records the findings for one record.
func (t *secretTally) note(record int, schema string, findings []secrets.Finding) {
	for _, f := range findings {
		t.total++
		if len(t.warnings) < maxSecretWarnings {
			t.warnings = append(t.warnings, SecretWarning{Record: record, Schema: schema, Finding: f})
		}
	}
}

// apply copies the tally onto result.
func (t *secretTally) apply(result *ImportResult) {
	if result == nil || t.total == 0 {
		return
	}
	result.SecretWarnings = t.warnings
	result.SecretWarningsTotal = t.total
}

// scanSecrets reports likely secrets in record outside what the intended and
// assigned schemas cover. Read-only: the record, its hash and storage are
// untouched.
func (e *EnhancedImporter) scanSecrets(tally *secretTally, record any, index int, intended, assigned string) {
	if e.secretScan == nil {
		return
	}
	tally.note(index, assigned, e.secretScan.Scan(record, intended, assigned))
}
