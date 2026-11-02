package importer

import (
	"fmt"

	"github.com/chazu/pudl/internal/secretscan"
)

// maxSecretWarnings caps the likely-secret findings an import result lists;
// SecretWarningsTotal still counts every one.
const maxSecretWarnings = secretscan.MaxWarnings

// SecretWarning is a likely secret found in an imported record outside its
// schema's sensitive-field coverage. It names the record, schema, path and
// kind — never the value.
type SecretWarning = secretscan.Warning

// secretTally accumulates findings across an import's records.
type secretTally struct{ secretscan.Tally }

// apply copies the tally onto result.
func (t *secretTally) apply(result *ImportResult) {
	if result == nil || t.Total == 0 {
		return
	}
	result.SecretWarnings = t.Warnings
	result.SecretWarningsTotal = t.Total
}

// scanSecrets reports likely secrets in record outside what the intended and
// assigned schemas cover. Read-only: the record, its hash and storage are
// untouched.
func (e *EnhancedImporter) scanSecrets(tally *secretTally, record any, index int, intended, assigned string) {
	if e.secretScan == nil {
		return
	}
	tally.Note(index, assigned, e.secretScan.Scan(record, intended, assigned))
}

// scanCatalogedDocument scans a document whose content is already cataloged.
// The dedup skip happens before decoding, so without this extra decode of the
// staged bytes a re-import would never warn. Collections need no such step:
// their items are decoded and scanned while staging, before the dedup check.
func (e *EnhancedImporter) scanCatalogedDocument(result *ImportResult, path, format, intended string) error {
	data, _, err := e.analyzeData(path, format)
	if err != nil {
		return fmt.Errorf("failed to analyze data: %w", err)
	}
	var tally secretTally
	e.scanSecrets(&tally, data, 0, intended, result.AssignedSchema)
	tally.apply(result)
	return nil
}
