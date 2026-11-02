package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An inferred entry whose schema was removed (a retired built-in such as
// pudl/infra.#Service) is drift, not invalid: doctor re-infers it and the
// inference repair reassigns it.
func TestDoctorReinfersEntriesWhoseSchemaWasRemoved(t *testing.T) {
	_, db, payload := repairWorkspace(t)
	addEntry(t, db, "2222222200000000", "pudl/infra.#Service", payload, false)

	report, repairs := doctorRepairs(t)
	require.Len(t, report.Catalog, 1)
	assert.Equal(t, "mismatch", report.Catalog[0].Status, report.Catalog[0].Error)
	assert.Equal(t, "user/demo.#Thing", report.Catalog[0].InferredSchema)
	assert.Equal(t, 1, repairs["inference"].Pending)

	_, repairs = doctorRepairs(t, "--fix", "--only", "inference")
	assert.Equal(t, "applied", string(repairs["inference"].Status))
	entry, err := db.GetEntry("2222222200000000")
	require.NoError(t, err)
	assert.Equal(t, "user/demo.#Thing", entry.Schema)
}
