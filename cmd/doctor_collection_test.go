package cmd

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A JSON array import stores a pudl/core.#Collection container whose payload
// is the array itself. Doctor must not validate that payload against the
// container schema, and a failing doctor must not print usage.
func TestDoctorAcceptsJSONArrayCollections(t *testing.T) {
	pudlDir := cliWorkspace(t)
	payload := filepath.Join(t.TempDir(), "fw.json")
	writeFile(t, payload, `[{"name":"a","x":1},{"name":"b","x":2}]`)
	imported := runCLI(t, "import", "--path", payload)
	require.NoError(t, imported.Err, "stdout=%s stderr=%s", imported.Stdout, imported.Stderr)

	report, _ := doctorRepairs(t)
	require.Len(t, report.Catalog, 3, "the collection and its two items")
	for _, entry := range report.Catalog {
		assert.Equal(t, "ok", entry.Status, "%s [%s]: %s", entry.Proquint, entry.Schema, entry.Error)
	}
	assert.True(t, report.OK)

	fixed := runCLI(t, "doctor", "--fix", "--dry-run")
	assert.NoError(t, fixed.Err, "stdout=%s", fixed.Stdout)

	// Break the workspace so doctor fails, and check usage is not printed.
	writeFile(t, filepath.Join(pudlDir, "schema", "user", "broken", "broken.cue"), "package broken\n#X: {\n")
	failed := runCLI(t, "doctor", "--health-only")
	require.Error(t, failed.Err)
	assert.NotContains(t, failed.Stdout+failed.Stderr, "Usage:")
}
