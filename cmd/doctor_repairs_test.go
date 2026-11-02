package cmd

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/repair"
)

// doctorRepairs runs doctor with args and --json and returns its report and
// the repairs by name. Doctor's exit status also reflects catalog validation,
// so it is not asserted here.
func doctorRepairs(t *testing.T, args ...string) (doctorReport, map[string]doctorRepair) {
	t.Helper()
	r := runCLI(t, append([]string{"doctor", "--json"}, args...)...)
	var report doctorReport
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &report), "stdout=%s stderr=%s", r.Stdout, r.Stderr)
	byName := map[string]doctorRepair{}
	for _, result := range report.Repairs {
		byName[result.Name] = result
	}
	return report, byName
}

// repairWorkspace creates a workspace with a user schema, a payload file and
// a writable catalog.
func repairWorkspace(t *testing.T) (string, *database.CatalogDB, string) {
	t.Helper()
	pudlDir := cliWorkspace(t)
	writeFile(t, filepath.Join(pudlDir, "schema", "user", "demo", "types.cue"), `package demo
#Thing: {
 _pudl: {schema_type: "base", resource_type: "test.thing", identity_fields: ["custom_field"]}
 custom_field: int
}
`)
	payload := filepath.Join(pudlDir, "data", "fixture.json")
	writeFile(t, payload, `{"custom_field":42}`)
	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return pudlDir, db, payload
}

func addEntry(t *testing.T, db *database.CatalogDB, id, schema, payload string, explicit bool) {
	t.Helper()
	entry := database.CatalogEntry{ID: id, StoredPath: payload, Schema: schema, Format: "json", Origin: "custom", ImportTimestamp: time.Now()}
	if explicit {
		kind := "observe"
		entry.EntryType = &kind
	}
	require.NoError(t, db.AddEntry(entry))
}

// addLegacySchemaEntry stores an entry whose schema name predates canonical
// naming; AddEntry would normalize it.
func addLegacySchemaEntry(t *testing.T, db *database.CatalogDB, payload string) {
	t.Helper()
	addEntry(t, db, "1111111100000000", "pudl/core.#Item", payload, false)
	_, err := db.DB().Exec("UPDATE catalog_entries SET schema = ? WHERE id = ?", "core.#Item", "1111111100000000")
	require.NoError(t, err)
}

func TestDoctorReportIsVersionedAndDiagnosesWithoutWriting(t *testing.T) {
	_, db, payload := repairWorkspace(t)
	addLegacySchemaEntry(t, db, payload)

	report, repairs := doctorRepairs(t)
	require.Equal(t, doctorReportVersion, report.ReportVersion)
	require.Equal(t, "diagnose", report.Mode)
	require.Equal(t, repair.Names(), func() []string {
		var names []string
		for _, r := range report.Repairs {
			names = append(names, r.Name)
		}
		return names
	}(), "every repair is diagnosed, in order")
	names := repairs["schema-names"]
	require.Equal(t, repair.StatusNeeded, names.Status)
	require.Equal(t, []string{"pudl", "doctor", "--fix", "--only", "schema-names"}, names.Apply)
	require.Equal(t, repair.StatusNeeded, repairs["identity"].Status, "an entry without resource_id needs identity")
	stored, err := db.GetEntry("1111111100000000")
	require.NoError(t, err)
	require.Equal(t, "core.#Item", stored.Schema, "diagnosis never writes")
	require.Nil(t, stored.ResourceID)
}

func TestDoctorFixFlagValidation(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "doctor", "--dry-run")
	require.ErrorContains(t, r.Err, "requires --fix")
	r = runCLI(t, "doctor", "--fix", "--only", "nonsense")
	require.ErrorContains(t, r.Err, `unknown repair "nonsense"`)
}

// Former `pudl schema migrate`.
func TestDoctorFixNormalizesSchemaNames(t *testing.T) {
	_, db, payload := repairWorkspace(t)
	addLegacySchemaEntry(t, db, payload)

	report, repairs := doctorRepairs(t, "--fix", "--dry-run", "--only", "schema-names")
	require.Equal(t, "dry-run", report.Mode)
	require.Len(t, report.Repairs, 1)
	require.Equal(t, 1, repairs["schema-names"].Pending)
	stored, _ := db.GetEntry("1111111100000000")
	require.Equal(t, "core.#Item", stored.Schema, "dry run writes nothing")

	_, repairs = doctorRepairs(t, "--fix", "--only", "schema-names")
	require.Equal(t, repair.StatusApplied, repairs["schema-names"].Status)
	require.Equal(t, []repair.Change{{Subject: "1111111100000000", From: "core.#Item", To: "pudl/core.#Item"}}, repairs["schema-names"].Changes)
	stored, _ = db.GetEntry("1111111100000000")
	require.Equal(t, "pudl/core.#Item", stored.Schema)
}

// Former `pudl schema reinfer`: drift is repaired, explicit assignments only
// with --include-explicit.
func TestDoctorFixReinfersDriftAndExplicitOnRequest(t *testing.T) {
	_, db, payload := repairWorkspace(t)
	addEntry(t, db, "1111111100000000", "pudl/core.#Item", payload, false)
	addEntry(t, db, "2222222200000000", "pudl/core.#Item", payload, true)

	_, repairs := doctorRepairs(t, "--fix", "--only", "inference")
	require.Equal(t, repair.StatusApplied, repairs["inference"].Status, repairs["inference"].Message)
	inferred, _ := db.GetEntry("1111111100000000")
	require.Equal(t, "user/demo.#Thing", inferred.Schema)
	require.NotNil(t, inferred.ResourceID, "identity is recomputed under the new schema")
	explicit, _ := db.GetEntry("2222222200000000")
	require.Equal(t, "pudl/core.#Item", explicit.Schema, "explicit assignments are not reclassified by default")

	_, repairs = doctorRepairs(t, "--fix", "--only", "inference", "--entry", "2222222200000000", "--include-explicit")
	require.Equal(t, 1, repairs["inference"].Changed)
	explicit, _ = db.GetEntry("2222222200000000")
	require.Equal(t, "user/demo.#Thing", explicit.Schema)
}

// Former `pudl migrate identity`.
func TestDoctorFixBackfillsIdentity(t *testing.T) {
	_, db, payload := repairWorkspace(t)
	addEntry(t, db, "1111111100000000", "user/demo.#Thing", payload, false)

	_, repairs := doctorRepairs(t, "--fix", "--only", "identity")
	require.Equal(t, repair.StatusApplied, repairs["identity"].Status, repairs["identity"].Message)
	stored, _ := db.GetEntry("1111111100000000")
	require.NotNil(t, stored.ResourceID)
	require.Equal(t, `{"custom_field":42}`, *stored.IdentityJSON)

	_, repairs = doctorRepairs(t, "--only", "identity")
	require.Equal(t, repair.StatusOK, repairs["identity"].Status)
}
