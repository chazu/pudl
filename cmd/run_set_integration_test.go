package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/validator"
	"github.com/chazu/pudl/internal/workspace"
)

// Run-set fixture models observe through #CommandObserve shell runs. The
// producer fails when PUDL_TEST_FAIL_PRODUCER is set. The consumer depends on
// the producer and touches PUDL_TEST_CONSUMER_OUT when its command runs, so a
// test can tell whether a blocked consumer ran.
const runSetFixtureModels = `

#Producer: #SystemModel & {
	name: "producer"
	populate: runs: [{argv: ["sh", "-c", """
		if [ -n "$PUDL_TEST_FAIL_PRODUCER" ]; then echo 'producer observation failed' >&2; exit 1; fi
		echo '[{"_schema":"resources.thing","name":"one","value":"ready"}]'
		"""]}]
}

#Consumer: #SystemModel & {
	name: "consumer"
	depends_on: ["producer"]
	populate: runs: [{argv: ["sh", "-c", "touch \"$PUDL_TEST_CONSUMER_OUT\"; echo '[{\"_schema\":\"resources.consumer\",\"name\":\"consumer\"}]'"]}]
}

#Zeta: #SystemModel & {
	name: "zeta"
	populate: runs: [{argv: ["sh", "-c", "echo '[{\"_schema\":\"resources.consumer\",\"name\":\"zeta\"}]'"]}]
}

#Drifter: #SystemModel & {
	name: "drifter"
	populate: runs: [{argv: ["sh", "-c", "echo '[{\"_schema\":\"resources.thing\",\"name\":\"two\",\"value\":\"live\"}]'"]}]
	desired: [{"_schema": "pudl/resources.#Thing", name: "two", value: "wanted"}]
}
`

func writeRunSetFixtureSchemas(t *testing.T, schemaRoot string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(schemaRoot, "cue.mod"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(schemaRoot, "cue.mod", "module.cue"), []byte("module: \"pudl.schemas\"\nlanguage: version: \"v0.14.0\"\n"), 0o644))
	modelsDir := filepath.Join(schemaRoot, "pudl", "systemmodel")
	require.NoError(t, os.MkdirAll(modelsDir, 0o755))
	models := systemmodel.SchemaCUE() + runSetFixtureModels
	require.NoError(t, os.WriteFile(filepath.Join(modelsDir, "models.cue"), []byte(models), 0o644))
	resourcesDir := filepath.Join(schemaRoot, "pudl", "resources")
	require.NoError(t, os.MkdirAll(resourcesDir, 0o755))
	resources := `package resources

#Thing: {
	_pudl: {
		schema_type: "base"
		resource_type: "resources.thing"
		identity_fields: ["name"]
		tracked_fields: ["value"]
	}
	name: string
	value: string
	...
}
`
	require.NoError(t, os.WriteFile(filepath.Join(resourcesDir, "resources.cue"), []byte(resources), 0o644))
}

// runSetHarness drives run sets.
type runSetHarness struct {
	consumerOut string
}

func (h *runSetHarness) run(models ...string) error {
	_, err := executeRunSet(context.Background(), models)
	return err
}

func setupRunSetFixture(t *testing.T) (*runSetHarness, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	pudlDir := config.GetPudlDir()
	writeRunSetFixtureSchemas(t, filepath.Join(pudlDir, "schema"))
	validator.ResetSharedLoaders()
	inference.ResetShared()
	t.Cleanup(func() {
		validator.ResetSharedLoaders()
		inference.ResetShared()
	})
	policy, err := workspace.Resolve(t.TempDir(), pudlDir)
	require.NoError(t, err)
	previousPolicy := wsPolicy
	wsPolicy = policy
	t.Cleanup(func() { wsPolicy = previousPolicy })

	consumerOut := filepath.Join(t.TempDir(), "consumer-input")
	t.Setenv("PUDL_TEST_CONSUMER_OUT", consumerOut)
	previousJSON := jsonOutput
	jsonOutput = true
	t.Cleanup(func() { jsonOutput = previousJSON })
	return &runSetHarness{consumerOut: consumerOut}, pudlDir
}

func latestRunSetReportForTest(t *testing.T, pudlDir string) acute.RunSetReport {
	t.Helper()
	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	defer db.Close()
	record, err := db.LatestRunSetReport()
	require.NoError(t, err)
	require.NotNil(t, record)
	var report acute.RunSetReport
	require.NoError(t, json.Unmarshal(record.Report, &report))
	return report
}

func memberRunReportForTest(t *testing.T, pudlDir, runID string) RunReport {
	t.Helper()
	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	defer db.Close()
	record, err := db.GetRunReport(runID)
	require.NoError(t, err)
	require.NotNil(t, record)
	var report RunReport
	require.NoError(t, json.Unmarshal(record.Report, &report))
	return report
}

func TestRunSetRunsProducersFirstAndPersistsLinkedReports(t *testing.T) {
	h, pudlDir := setupRunSetFixture(t)

	require.NoError(t, h.run("consumer", "producer"))
	_, err := os.Stat(h.consumerOut)
	require.NoError(t, err, "the consumer ran")

	setReport := latestRunSetReportForTest(t, pudlDir)
	assert.Equal(t, database.RunStatusSucceeded, setReport.Status)
	assert.Equal(t, []string{"producer", "consumer"}, setReport.Ordered)
	require.Len(t, setReport.Members, 2)

	consumerReport := memberRunReportForTest(t, pudlDir, setReport.Members[1].RunID)
	assert.Equal(t, setReport.RunSetID, consumerReport.RunSetID)
	assert.Equal(t, database.RunStatusSucceeded, consumerReport.CompletionStatus)
}

func TestRunSetBlocksConsumerAndContinuesIndependentBranchAfterProducerFailure(t *testing.T) {
	h, pudlDir := setupRunSetFixture(t)
	t.Setenv("PUDL_TEST_FAIL_PRODUCER", "1")

	err := h.run("consumer", "producer", "zeta")
	require.ErrorContains(t, err, "run set")
	_, statErr := os.Stat(h.consumerOut)
	assert.True(t, os.IsNotExist(statErr), "a blocked consumer never runs its command")

	report := latestRunSetReportForTest(t, pudlDir)
	assert.Equal(t, database.RunStatusFailed, report.Status)
	results := map[string]string{}
	for _, member := range report.Members {
		results[member.Model] = member.Result
		memberReport := memberRunReportForTest(t, pudlDir, member.RunID)
		assert.Equal(t, member.Result, memberReport.CompletionStatus)
		assert.Equal(t, report.RunSetID, memberReport.RunSetID)
	}
	assert.Equal(t, database.RunStatusFailed, results["producer"])
	assert.Equal(t, database.RunStatusBlocked, results["consumer"])
	assert.Equal(t, database.RunStatusSucceeded, results["zeta"])
}

func TestRunSetReportCommandReadsPersistedReportByID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	db, err := database.NewCatalogDB(config.GetPudlDir())
	require.NoError(t, err)
	payload, err := json.Marshal(acute.RunSetReport{
		ReportVersion: 1, RunSetID: "set_1", Mode: "observe-only",
		Status: database.RunStatusSucceeded,
	})
	require.NoError(t, err)
	require.NoError(t, db.SaveRunSetReport("set_1", payload))
	require.NoError(t, db.Close())

	previousJSON := jsonOutput
	jsonOutput = true
	t.Cleanup(func() { jsonOutput = previousJSON })
	require.NoError(t, runReportCmd.RunE(runReportCmd, []string{"set_1"}))
	require.ErrorContains(t, runReportCmd.RunE(runReportCmd, []string{"missing"}), "not found")
}
