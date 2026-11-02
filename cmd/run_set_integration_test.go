package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
// producer honours two switches so one fixture covers failure and empty
// observations: PUDL_TEST_FAIL_PRODUCER and PUDL_TEST_EMPTY_PRODUCER. The
// consumer writes its bound input to PUDL_TEST_CONSUMER_OUT, proving the
// producer's value reached the consumer's command.
const runSetFixtureModels = `

#Producer: #SystemModel & {
	name: "producer"
	populate: runs: [{argv: ["sh", "-c", """
		if [ -n "$PUDL_TEST_FAIL_PRODUCER" ]; then echo 'producer observation failed' >&2; exit 1; fi
		if [ -n "$PUDL_TEST_EMPTY_PRODUCER" ]; then echo '[]'; exit 0; fi
		echo '[{"_schema":"resources.thing","name":"one","value":"ready"}]'
		"""]}]
}

#Consumer: #SystemModel & {
	name: "consumer"
	inputs: value: string @pudl(binding=plain)
	bindings: value: {
		source: {model: "producer", schema: "pudl/resources.#Thing", identity: {name: "one"}}
		path: "/value"
	}
	populate: runs: [{argv: ["sh", "-c", "printf '%s' \"$1\" > \"$PUDL_TEST_CONSUMER_OUT\"; echo '[{\"_schema\":\"resources.consumer\",\"name\":\"consumer\"}]'", "consumer", inputs.value]}]
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
	value: string @pudl(binding=plain)
	...
}
`
	require.NoError(t, os.WriteFile(filepath.Join(resourcesDir, "resources.cue"), []byte(resources), 0o644))
}

// runSetHarness drives run sets with explicit options.
type runSetHarness struct {
	opts        runSetOptions
	deps        runDeps
	consumerOut string
}

func (h *runSetHarness) run(models ...string) error {
	return runSetForTest(models, h.opts, h.deps)
}

func runSetForTest(models []string, opts runSetOptions, deps runDeps) error {
	_, err := executeRunSet(context.Background(), models, opts, deps)
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
	deps := runDeps{mu: newExecMu(context.Background(), 0)}
	return &runSetHarness{deps: deps, consumerOut: consumerOut}, pudlDir
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

func TestRunSetUsesProducerCurrentRunSnapshotAndPersistsLinkedReports(t *testing.T) {
	h, pudlDir := setupRunSetFixture(t)

	require.NoError(t, h.run("consumer", "producer"))
	bound, err := os.ReadFile(h.consumerOut)
	require.NoError(t, err)
	assert.Equal(t, "ready", string(bound), "the producer's value reached the consumer's command")

	setReport := latestRunSetReportForTest(t, pudlDir)
	assert.Equal(t, database.RunStatusSucceeded, setReport.Status)
	assert.Equal(t, []string{"producer", "consumer"}, setReport.Ordered)
	require.Len(t, setReport.Members, 2)

	consumerReport := memberRunReportForTest(t, pudlDir, setReport.Members[1].RunID)
	assert.Equal(t, setReport.RunSetID, consumerReport.RunSetID)
	assert.Equal(t, database.RunStatusSucceeded, consumerReport.CompletionStatus)
	require.Len(t, consumerReport.Bindings, 1)
	assert.Equal(t, "current-run", consumerReport.Bindings[0].Selection)
	assert.Equal(t, setReport.Members[0].RunID, consumerReport.Bindings[0].ProducerRunID)
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
		if member.Model == "consumer" {
			require.Len(t, memberReport.BindingIssues, 1)
			issue := memberReport.BindingIssues[0]
			assert.Equal(t, "value", issue.Input)
			assert.Equal(t, "producer", issue.ProducerModel)
			assert.Equal(t, "pudl/resources.#Thing", issue.Schema)
			assert.Equal(t, "/value", issue.Path)
			assert.Equal(t, "producer-unsuccessful", issue.Code)
			assert.Contains(t, issue.Message, "historical fallback is forbidden")
		}
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

func TestRunSetPersistsStructuredUnresolvedBindingWhenCurrentSnapshotLacksSource(t *testing.T) {
	h, pudlDir := setupRunSetFixture(t)
	t.Setenv("PUDL_TEST_EMPTY_PRODUCER", "1")

	err := h.run("consumer", "producer")
	require.ErrorContains(t, err, "run set")
	report := latestRunSetReportForTest(t, pudlDir)
	require.Len(t, report.Members, 2)
	consumer := report.Members[1]
	assert.Equal(t, "consumer", consumer.Model)
	assert.Equal(t, database.RunStatusFailed, consumer.Result)

	memberReport := memberRunReportForTest(t, pudlDir, consumer.RunID)
	require.Len(t, memberReport.BindingIssues, 1)
	issue := memberReport.BindingIssues[0]
	assert.Equal(t, "value", issue.Input)
	assert.Equal(t, "producer", issue.ProducerModel)
	assert.Equal(t, "source-absent", issue.Code)
	assert.Equal(t, "/value", issue.Path)
	assert.True(t, strings.Contains(issue.Message, "no matching"), issue.Message)
}
