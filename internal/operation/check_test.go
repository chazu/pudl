package operation

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
	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/validator"
)

// Check fixture models observe through #CommandObserve shell runs. The
// producer fails when PUDL_TEST_FAIL_PRODUCER is set. The consumer depends on
// the producer and touches PUDL_TEST_CONSUMER_OUT when its command runs, so a
// test can tell whether a blocked consumer ran.
const checkFixtureModels = `

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

func writeCheckFixtureSchemas(t *testing.T, schemaRoot string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(schemaRoot, "cue.mod"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(schemaRoot, "cue.mod", "module.cue"), []byte("module: \"pudl.schemas\"\nlanguage: version: \"v0.14.0\"\n"), 0o644))
	modelsDir := filepath.Join(schemaRoot, "pudl", "systemmodel")
	require.NoError(t, os.MkdirAll(modelsDir, 0o755))
	models := systemmodel.SchemaCUE() + checkFixtureModels
	require.NoError(t, os.WriteFile(filepath.Join(modelsDir, "models.cue"), []byte(models), 0o644))
	resourcesDir := filepath.Join(schemaRoot, "pudl", "resources")
	require.NoError(t, os.MkdirAll(resourcesDir, 0o755))
	resources := `package resources

#Thing: {
	_pudl: {
		schema_type: "base"
		resource_type: "resources.thing"
		identity_fields: ["name"]
	}
	name: string
	value: string
	...
}
`
	require.NoError(t, os.WriteFile(filepath.Join(resourcesDir, "resources.cue"), []byte(resources), 0o644))
}

// checkHarness drives checks in a fixture workspace.
type checkHarness struct {
	ws          Workspace
	consumerOut string
}

func (h *checkHarness) check(models ...string) (*Result, error) {
	return Check(context.Background(), h.ws, quietOutput(), Options{Models: models})
}

func (h *checkHarness) run(models ...string) error {
	_, err := h.check(models...)
	return err
}

func setupCheckFixture(t *testing.T) (*checkHarness, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	pudlDir := config.GetPudlDir()
	writeCheckFixtureSchemas(t, filepath.Join(pudlDir, "schema"))
	validator.ResetSharedLoaders()
	inference.ResetShared()
	t.Cleanup(func() {
		validator.ResetSharedLoaders()
		inference.ResetShared()
	})
	consumerOut := filepath.Join(t.TempDir(), "consumer-input")
	t.Setenv("PUDL_TEST_CONSUMER_OUT", consumerOut)
	return &checkHarness{ws: testWorkspace(pudlDir), consumerOut: consumerOut}, pudlDir
}

func latestCheckReportForTest(t *testing.T, pudlDir string) acute.RunSetReport {
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

func memberRunReportForTest(t *testing.T, pudlDir, runID string) checkreport.ModelReport {
	t.Helper()
	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	defer db.Close()
	record, err := db.GetRunReport(runID)
	require.NoError(t, err)
	require.NotNil(t, record)
	var report checkreport.ModelReport
	require.NoError(t, json.Unmarshal(record.Report, &report))
	return report
}

func TestCheckRunsProducersFirstAndPersistsLinkedReports(t *testing.T) {
	h, pudlDir := setupCheckFixture(t)

	require.NoError(t, h.run("consumer", "producer"))
	_, err := os.Stat(h.consumerOut)
	require.NoError(t, err, "the consumer ran")

	setReport := latestCheckReportForTest(t, pudlDir)
	assert.Equal(t, database.RunStatusSucceeded, setReport.Status)
	assert.Equal(t, []string{"producer", "consumer"}, setReport.Ordered)
	require.Len(t, setReport.Members, 2)

	consumerReport := memberRunReportForTest(t, pudlDir, setReport.Members[1].RunID)
	assert.Equal(t, setReport.RunSetID, consumerReport.RunSetID)
	assert.Equal(t, database.RunStatusSucceeded, consumerReport.CompletionStatus)
}

func TestCheckBlocksConsumerAndContinuesIndependentBranchAfterProducerFailure(t *testing.T) {
	h, pudlDir := setupCheckFixture(t)
	t.Setenv("PUDL_TEST_FAIL_PRODUCER", "1")

	err := h.run("consumer", "producer", "zeta")
	require.ErrorContains(t, err, "producer failed")
	_, statErr := os.Stat(h.consumerOut)
	assert.True(t, os.IsNotExist(statErr), "a blocked consumer never runs its command")

	report := latestCheckReportForTest(t, pudlDir)
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

// One model or several, a check records one report that embeds each model's
// full report, and `pudl report` renders it with model names.
func TestCheckOfOneModelRecordsOneReportWithTheModelEmbedded(t *testing.T) {
	h, pudlDir := setupCheckFixture(t)
	result, err := h.check("drifter")
	require.NoError(t, err)
	require.True(t, result.Findings)

	report := latestCheckReportForTest(t, pudlDir)
	assert.Equal(t, ReportVersion, report.ReportVersion)
	assert.True(t, strings.HasPrefix(report.RunSetID, "check_"), report.RunSetID)
	require.Len(t, report.Members, 1)
	var member checkreport.ModelReport
	require.NoError(t, json.Unmarshal(report.Members[0].Report, &member))
	assert.Equal(t, "drifter", member.Model)
	require.NotNil(t, member.Drift)
	assert.False(t, member.Drift.Clean)

	db, err := database.OpenCatalogDBReadOnly(pudlDir)
	require.NoError(t, err)
	defer db.Close()
	stored, err := checkreport.Find(db, report.RunSetID)
	require.NoError(t, err)
	var text strings.Builder
	require.NoError(t, stored.WriteText(&text))
	human := text.String()
	for _, text := range []string{"# model: drifter", "### drift", "check " + report.RunSetID + ": succeeded", "drifter: succeeded"} {
		assert.Contains(t, human, text)
	}
	assert.NotContains(t, human, "//models/")
}

// A populate-only member finds nothing; observe-only drift is a finding, not
// an error.
func TestCheckReportsFindingsSeparatelyFromErrors(t *testing.T) {
	h, _ := setupCheckFixture(t)

	result, err := h.check("producer")
	require.NoError(t, err)
	assert.False(t, result.Findings)
	assert.False(t, result.OnlyCheckFailures())

	result, err = h.check("drifter")
	require.NoError(t, err, "observe-only drift is a result, not an error")
	assert.True(t, result.Findings)
}

// A check that fails while resolving its models has no report, and says so
// through its progress events.
func TestCheckWithoutAReportReturnsNoResult(t *testing.T) {
	h, _ := setupCheckFixture(t)
	var events []string
	out := quietOutput()
	out.Progress = func(phase, subject, state string) { events = append(events, phase+" "+subject+" "+state) }
	result, err := Check(context.Background(), h.ws, out, Options{Models: []string{"missing"}})
	require.ErrorContains(t, err, `system model "missing" not found`)
	assert.Nil(t, result)
	assert.Equal(t, []string{"check missing started", "check missing failed"}, events)
}

func TestOnlyCheckFailures(t *testing.T) {
	result := func(members ...acute.RunSetMemberReport) *Result {
		return &Result{Report: &acute.RunSetReport{Members: members}}
	}
	checkFailed := acute.RunSetMemberReport{Model: "a", Result: database.RunStatusFailed, Error: ErrFailSeverityChecks.Error()}
	blocked := acute.RunSetMemberReport{Model: "b", Result: database.RunStatusBlocked}
	broken := acute.RunSetMemberReport{Model: "c", Result: database.RunStatusFailed, Error: "populate: exit 1"}

	assert.True(t, result(checkFailed, blocked).OnlyCheckFailures())
	assert.False(t, result(checkFailed, broken).OnlyCheckFailures())
	assert.False(t, result(blocked).OnlyCheckFailures(), "nothing failed")
	assert.False(t, (*Result)(nil).OnlyCheckFailures())
}

func TestReportHasFindings(t *testing.T) {
	assert.False(t, reportHasFindings(nil))
	assert.False(t, reportHasFindings(&checkreport.ModelReport{Drift: &acute.ModelDriftResult{Clean: true}}))
	assert.True(t, reportHasFindings(&checkreport.ModelReport{Drift: &acute.ModelDriftResult{Clean: false}}), "replayed drift is still a finding")
	assert.True(t, reportHasFindings(&checkreport.ModelReport{Checks: []checkreport.CheckResult{{Severity: "fail"}}}))
	assert.False(t, reportHasFindings(&checkreport.ModelReport{Checks: []checkreport.CheckResult{{Severity: "warn"}}}))
}

// Live output prints each model's plan and report as it concludes, passes a
// failing command's stderr through, and hands the final report to Concluded.
func TestCheckLiveOutput(t *testing.T) {
	h, _ := setupCheckFixture(t)
	t.Setenv("PUDL_TEST_FAIL_PRODUCER", "1")
	var stdout, stderr strings.Builder
	var concluded *acute.RunSetReport
	out := Output{Out: &stdout, Err: &stderr, Live: true, Concluded: func(r *acute.RunSetReport) error {
		concluded = r
		return nil
	}}
	_, err := Check(context.Background(), h.ws, out, Options{Models: []string{"producer", "zeta"}})
	require.ErrorContains(t, err, "producer failed")

	assert.Contains(t, stdout.String(), "model:    producer")
	assert.Contains(t, stdout.String(), "# model: zeta")
	assert.Contains(t, stderr.String(), "producer observation failed")
	require.NotNil(t, concluded)
	assert.Equal(t, database.RunStatusFailed, concluded.Status)
	require.Len(t, concluded.Members, 2)
}
