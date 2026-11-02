package checkreport

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
)

func storedFixture(t *testing.T) *database.CatalogDB {
	t.Helper()
	db, err := database.NewCatalogDB(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestFindAndLatestReportWhatIsMissing(t *testing.T) {
	db := storedFixture(t)
	_, err := Latest(db)
	require.ErrorContains(t, err, "no persisted operation reports")
	_, err = Find(db, "run_missing")
	require.ErrorContains(t, err, `operation report "run_missing" not found`)
}

// A model report reads back as recorded: JSON is the stored payload, and the
// text shows fields current reports no longer carry as legacy fields.
func TestFindModelReportKeepsRecordedFields(t *testing.T) {
	db := storedFixture(t)
	payload := []byte(`{"run_id":"run_a","model":"m","mode":"observe-only","ok":true,"report_version":2,"converge":{"applied":1}}`)
	require.NoError(t, db.SaveRunReport("run_a", "m", payload))

	stored, err := Find(db, "run_a")
	require.NoError(t, err)
	require.Nil(t, stored.Check)
	encoded, err := stored.JSON()
	require.NoError(t, err)
	assert.Equal(t, payload, encoded)

	var text bytes.Buffer
	require.NoError(t, stored.WriteText(&text))
	assert.Contains(t, text.String(), "# model: m")
	assert.Contains(t, text.String(), "## legacy fields")
	assert.Contains(t, text.String(), "### converge")
}

func TestLatestReadsACheckReportWithItsMembers(t *testing.T) {
	db := storedFixture(t)
	require.NoError(t, db.SaveRunReport("run_a", "m", []byte(`{"run_id":"run_a","model":"m","mode":"observe-only","ok":true}`)))
	require.NoError(t, db.SaveRunSetReport("check_a", []byte(`{"run_set_id":"check_a","status":"succeeded","members":[{"model":"m","run_id":"run_a","result":"succeeded","report":{"run_id":"run_a","model":"m","mode":"observe-only","ok":true}}]}`)))

	stored, err := Latest(db)
	require.NoError(t, err)
	require.NotNil(t, stored.Check)
	assert.Equal(t, "check_a", stored.Check.RunSetID)

	var text bytes.Buffer
	require.NoError(t, stored.WriteText(&text))
	assert.Contains(t, text.String(), "# model: m")
	assert.Contains(t, text.String(), "check check_a: succeeded")
	assert.Contains(t, text.String(), "m: succeeded (run_a)")
}
