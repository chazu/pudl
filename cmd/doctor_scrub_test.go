package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/repair"
)

const scrubLoginSchema = `package vault

#Login: {
	_pudl: {
		schema_type:   "base"
		resource_type: "test.login"
		identity_fields: ["name"]
		%s
	}
	name: string
	...
}
`

func TestDoctorSensitiveScrubIsOptInAndNeverPrintsValues(t *testing.T) {
	const secret = "hunter2-correct-horse"
	pudlDir := cliWorkspace(t)
	schemaPath := filepath.Join(pudlDir, "schema", "pudl", "vault", "vault.cue")
	writeFile(t, schemaPath, fmt.Sprintf(scrubLoginSchema, ""))
	writeFile(t, "login.json", `{"name":"svc","password":"`+secret+`"}`)
	r := runCLI(t, "import", "login.json", "--schema", "pudl/vault.#Login", "--json")
	require.NoError(t, r.Err, r.Stderr)
	var imported []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &imported), r.Stdout)
	writeFile(t, schemaPath, fmt.Sprintf(scrubLoginSchema, `sensitive_fields: ["password"]`))
	reloadSchemas()

	outputs := []string{}
	report, repairs := doctorRepairs(t)
	scrub := repairs["sensitive-scrub"]
	assert.Equal(t, repair.StatusNeeded, scrub.Status, "%+v", scrub)
	assert.Contains(t, scrub.Message, "rotate every credential")
	assert.Contains(t, scrub.Message, "Fact history and exported bundles keep the plaintext")
	assert.Equal(t, []string{"pudl", "doctor", "--fix", "--only", "sensitive-scrub"}, scrub.Apply)
	outputs = append(outputs, fmt.Sprintf("%+v", report))
	human := runCLI(t, "doctor")
	assert.Contains(t, human.Stdout, "Repair sensitive-scrub: needed")
	outputs = append(outputs, human.Stdout, human.Stderr)

	// Plain --fix never scrubs: naming the repair is the confirmation. Nor
	// does it reclassify this explicit import.
	_, repairs = doctorRepairs(t, "--fix")
	assert.Equal(t, repair.StatusSkipped, repairs["sensitive-scrub"].Status)
	assert.Contains(t, repairs["sensitive-scrub"].Message, "opt-in")
	assert.Equal(t, repair.StatusOK, repairs["inference"].Status, "%+v", repairs["inference"])
	_, repairs = doctorRepairs(t, "--fix", "--dry-run", "--only", "sensitive-scrub")
	assert.Equal(t, repair.StatusNeeded, repairs["sensitive-scrub"].Status)

	applied := runCLI(t, "doctor", "--json", "--fix", "--only", "sensitive-scrub")
	outputs = append(outputs, applied.Stdout, applied.Stderr)
	var appliedReport doctorReport
	require.NoError(t, json.Unmarshal([]byte(applied.Stdout), &appliedReport), applied.Stdout)
	require.Len(t, appliedReport.Repairs, 1)
	assert.Equal(t, repair.StatusApplied, appliedReport.Repairs[0].Status, appliedReport.Repairs[0].Errors)

	// The old proquint still resolves, to the re-derived entry.
	require.Len(t, imported, 1)
	oldID := imported[0].ID
	show := runCLI(t, "show", idgen.HashToProquint(oldID), "--json")
	require.NoError(t, show.Err, show.Stderr)
	assert.NotContains(t, show.Stdout, oldID)
	var shown struct {
		Entry struct {
			StoredPath string `json:"stored_path"`
		} `json:"entry"`
	}
	require.NoError(t, json.Unmarshal([]byte(show.Stdout), &shown), show.Stdout)
	stored, err := os.ReadFile(shown.Entry.StoredPath)
	require.NoError(t, err)
	assert.Equal(t, `{"name":"svc","password":"[REDACTED]"}`, string(stored))
	outputs = append(outputs, show.Stdout)

	_, repairs = doctorRepairs(t)
	assert.Equal(t, repair.StatusOK, repairs["sensitive-scrub"].Status)
	for _, out := range outputs {
		assert.NotContains(t, out, secret)
	}
}
