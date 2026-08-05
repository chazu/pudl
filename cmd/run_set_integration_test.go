package cmd

import (
	"encoding/json"
	"fmt"
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
	"github.com/chazu/pudl/internal/wiring"
	"github.com/chazu/pudl/internal/workspace"
)

type runSetMu struct {
	configs         map[string]string
	failProducer    bool
	producerEmpty   bool
	applied         map[string]int
	operations      []string
	failApply       string
	invalidManifest string
	planSuffix      string
}

func (m *runSetMu) Observe(configPath, target string) ([]byte, error) {
	root := filepath.Dir(configPath)
	generated, _ := filepath.Glob(filepath.Join(root, "pudl_run_*", "mu.cue"))
	if len(generated) > 0 {
		data, _ := os.ReadFile(generated[len(generated)-1])
		m.configs[target] = string(data)
	}
	if m.failProducer && strings.Contains(target, "producer") {
		return nil, fmt.Errorf("producer observation failed")
	}
	if strings.Contains(target, "mutator") {
		m.operations = append(m.operations, "observe "+target)
		exists := m.applied[target] > 0
		return []byte(fmt.Sprintf(`[{"target":%q,"current":{"resources":[{"resource":"Config/%s","exists":%t,"matches":%t}]}}]`, target, strings.TrimPrefix(target, "//models/"), exists, exists)), nil
	}
	switch {
	case strings.Contains(target, "producer"):
		if m.producerEmpty {
			return []byte(fmt.Sprintf(`[{"target":%q,"current":{"records":[]}}]`, target)), nil
		}
		return []byte(`[{
			"target":"//models/producer:populate",
			"current":{"records":[{"_schema":"resources.thing","name":"one","value":"ready"}]}
		}]`), nil
	case strings.Contains(target, "consumer"):
		return []byte(`[{
			"target":"//models/consumer:populate",
			"current":{"records":[{"_schema":"resources.consumer","name":"consumer"}]}
		}]`), nil
	case strings.Contains(target, "zeta"):
		return []byte(`[{
			"target":"//models/zeta:populate",
			"current":{"records":[{"_schema":"resources.consumer","name":"zeta"}]}
		}]`), nil
	default:
		return nil, fmt.Errorf("unexpected target %q", target)
	}
}

func (m *runSetMu) Build(configPath, target string, flags ...string) ([]byte, error) {
	for _, flag := range flags {
		if flag == "--plan" {
			m.operations = append(m.operations, "plan "+target)
			if strings.Contains(target, "mutator-secret-producer") || strings.Contains(target, "sealed-mutator") {
				return []byte(fmt.Sprintf(`{"version":2,"targets":[%q],"actions":[{"id":%q,"action_key":%q,"sealed_outputs":{"TOKEN":"pass:apps/token"},"sealed_output_modes":{"TOKEN":"create_if_absent"}}]}`, target, target+":write-token", "sha256:sealed"+m.planSuffix)), nil
			}
			if strings.Contains(target, "mutator-secret-consumer") {
				return []byte(fmt.Sprintf(`{"version":2,"targets":[%q],"actions":[{"id":%q,"action_key":%q,"sealed_inputs":{"TOKEN":"pass:apps/token"},"sealed_input_modes":{"TOKEN":"env"}}]}`, target, target+":read-token", "sha256:sealed-input"+m.planSuffix)), nil
			}
			return []byte(fmt.Sprintf(`{"version":2,"targets":[%q],"actions":[{"id":%q,"command":["apply"],"env":{"PLAN_SUFFIX":%q}}]}`, target, target+":apply", m.planSuffix)), nil
		}
		if flag == "--emit-manifest" {
			m.operations = append(m.operations, "apply "+target)
			if m.failApply != "" && strings.Contains(target, m.failApply) {
				if m.failApply == "sealed-mutator" {
					return nil, fmt.Errorf("provider pass:apps/token rejected the write for %s", target)
				}
				return nil, fmt.Errorf("scripted apply failure for %s", target)
			}
			m.applied[target]++
			if m.invalidManifest != "" && strings.Contains(target, m.invalidManifest) {
				return []byte(`not a manifest`), nil
			}
			return []byte(`{"actions":[]}`), nil
		}
	}
	return nil, fmt.Errorf("unexpected build flags %v for %q", flags, target)
}

func writeRunSetFixtureSchemas(t *testing.T, schemaRoot string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(schemaRoot, "cue.mod"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(schemaRoot, "cue.mod", "module.cue"), []byte("module: \"pudl.schemas\"\nlanguage: version: \"v0.14.0\"\n"), 0o644))
	modelsDir := filepath.Join(schemaRoot, "pudl", "systemmodel")
	require.NoError(t, os.MkdirAll(modelsDir, 0o755))
	models := systemmodel.SchemaCUE() + `

#Producer: #SystemModel & {
	name: "producer"
	plugins: [{name: "fake", command: ["true"]}]
	populate: #PluginObserve & {plugin: "fake", input: {}, differential: false}
}

#Consumer: #SystemModel & {
	name: "consumer"
	plugins: [{name: "fake", command: ["true"]}]
	inputs: value: string @pudl(binding=plain)
	bindings: value: {
		source: {model: "producer", schema: "pudl/resources.#Thing", identity: {name: "one"}}
		path: "/value"
	}
	populate: #PluginObserve & {plugin: "fake", input: {bound: inputs.value}, differential: false}
}

#Zeta: #SystemModel & {
	name: "zeta"
	plugins: [{name: "fake", command: ["true"]}]
	populate: #PluginObserve & {plugin: "fake", input: {}, differential: false}
}

#SealedMutator: #SystemModel & {
	name: "sealed-mutator"
	plugins: [{name: "fake", command: ["true"]}]
	populate: #PluginObserve & {plugin: "fake", input: {}, differential: true}
	desired: [{_schema: "pudl/resources.#Consumer", name: "sealed-mutator"}]
	converge: #PluginPlan & {
		plugin: "fake"
		input: {}
		sealed_outputs: TOKEN: {
			ref: "pass:apps/token"
			store_mode: "create_if_absent"
		} @pudl(binding=sealed)
	}
}

#SecretProducer: #SystemModel & {
	name: "mutator-secret-producer"
	plugins: [{name: "fake", command: ["true"]}]
	populate: #PluginObserve & {plugin: "fake", input: {}, differential: true}
	desired: [{_schema: "pudl/resources.#Consumer", name: "mutator-secret-producer"}]
	converge: #PluginPlan & {
		plugin: "fake"
		input: {}
		sealed_outputs: TOKEN: {
			ref: "pass:apps/token"
			store_mode: "create_if_absent"
		} @pudl(binding=sealed)
	}
}

#SecretConsumer: #SystemModel & {
	name: "mutator-secret-consumer"
	plugins: [{name: "fake", command: ["true"]}]
	populate: #PluginObserve & {plugin: "fake", input: {}, differential: true}
	desired: [{_schema: "pudl/resources.#Consumer", name: "mutator-secret-consumer"}]
	converge: #PluginPlan & {
		plugin: "fake"
		input: {}
		sealed_inputs: TOKEN: {
			source: {model: "mutator-secret-producer", output: "TOKEN"}
			delivery_mode: "env"
		} @pudl(binding=sealed)
	}
}

#MutatorA: #SystemModel & {
	name: "mutator-a"
	plugins: [{name: "fake", command: ["true"]}]
	populate: #PluginObserve & {plugin: "fake", input: {}, differential: true}
	desired: [{_schema: "pudl/resources.#Consumer", name: "mutator-a"}]
	converge: #PluginPlan & {plugin: "fake", input: {}}
}

#MutatorB: #SystemModel & {
	name: "mutator-b"
	plugins: [{name: "fake", command: ["true"]}]
	populate: #PluginObserve & {plugin: "fake", input: {}, differential: true}
	desired: [{_schema: "pudl/resources.#Consumer", name: "mutator-b"}]
	converge: #PluginPlan & {plugin: "fake", input: {}}
}

#MutatorDependent: #SystemModel & {
	name: "mutator-dependent"
	depends_on: ["mutator-a"]
	plugins: [{name: "fake", command: ["true"]}]
	populate: #PluginObserve & {plugin: "fake", input: {}, differential: true}
	desired: [{_schema: "pudl/resources.#Consumer", name: "mutator-dependent"}]
	converge: #PluginPlan & {plugin: "fake", input: {}}
}
`
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

func TestRunSetUsesProducerCurrentRunSnapshotAndPersistsLinkedReports(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pudlDir := config.GetPudlDir()
	schemaRoot := filepath.Join(pudlDir, "schema")
	writeRunSetFixtureSchemas(t, schemaRoot)
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

	muRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(muRoot, "mu.cue"), []byte("package mu\n"), 0o644))
	previousMuRoot := runSetMuRoot
	runSetMuRoot = muRoot
	t.Cleanup(func() { runSetMuRoot = previousMuRoot })
	runner := &runSetMu{configs: map[string]string{}}
	previousFactory := runMuRunnerFactory
	runMuRunnerFactory = func() muRunner { return runner }
	t.Cleanup(func() { runMuRunnerFactory = previousFactory })
	previousJSON := jsonOutput
	jsonOutput = true
	t.Cleanup(func() { jsonOutput = previousJSON })
	runSetCmd.Flags().Lookup("max-observation-age").Changed = false

	err = runObserveSet(runSetCmd, []string{"consumer", "producer"})
	require.NoError(t, err)
	consumerConfig := runner.configs["//models/consumer:populate"]
	assert.Contains(t, consumerConfig, `"bound":"ready"`)
	assert.NotContains(t, consumerConfig, `"bindings"`)
	assert.NotContains(t, consumerConfig, `"inputs"`)

	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	defer db.Close()
	record, err := db.LatestRunSetReport()
	require.NoError(t, err)
	require.NotNil(t, record)
	var setReport acute.RunSetReport
	require.NoError(t, json.Unmarshal(record.Report, &setReport))
	assert.Equal(t, database.RunStatusSucceeded, setReport.Status)
	assert.Equal(t, []string{"producer", "consumer"}, setReport.Ordered)
	require.Len(t, setReport.Members, 2)

	consumerRecord, err := db.GetRunReport(setReport.Members[1].RunID)
	require.NoError(t, err)
	require.NotNil(t, consumerRecord)
	var consumerReport RunReport
	require.NoError(t, json.Unmarshal(consumerRecord.Report, &consumerReport))
	assert.Equal(t, setReport.RunSetID, consumerReport.RunSetID)
	assert.Equal(t, database.RunStatusSucceeded, consumerReport.CompletionStatus)
	require.Len(t, consumerReport.Bindings, 1)
	assert.Equal(t, "current-run", consumerReport.Bindings[0].Selection)
	assert.Equal(t, setReport.Members[0].RunID, consumerReport.Bindings[0].ProducerRunID)
}

func TestRunSetBlocksConsumerAndContinuesIndependentBranchAfterProducerFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
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

	muRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(muRoot, "mu.cue"), []byte("package mu\n"), 0o644))
	previousMuRoot := runSetMuRoot
	runSetMuRoot = muRoot
	t.Cleanup(func() { runSetMuRoot = previousMuRoot })
	runner := &runSetMu{configs: map[string]string{}, failProducer: true}
	previousFactory := runMuRunnerFactory
	runMuRunnerFactory = func() muRunner { return runner }
	t.Cleanup(func() { runMuRunnerFactory = previousFactory })
	previousJSON := jsonOutput
	jsonOutput = true
	t.Cleanup(func() { jsonOutput = previousJSON })
	runSetCmd.Flags().Lookup("max-observation-age").Changed = false

	err = runObserveSet(runSetCmd, []string{"consumer", "producer", "zeta"})
	require.ErrorContains(t, err, "run set")
	assert.NotContains(t, runner.configs, "//models/consumer:populate")
	assert.Contains(t, runner.configs, "//models/zeta:populate")

	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	defer db.Close()
	record, err := db.LatestRunSetReport()
	require.NoError(t, err)
	require.NotNil(t, record)
	var report acute.RunSetReport
	require.NoError(t, json.Unmarshal(record.Report, &report))
	assert.Equal(t, database.RunStatusFailed, report.Status)
	results := map[string]string{}
	for _, member := range report.Members {
		results[member.Model] = member.Result
		memberRecord, reportErr := db.GetRunReport(member.RunID)
		require.NoError(t, reportErr)
		require.NotNil(t, memberRecord)
		var memberReport RunReport
		require.NoError(t, json.Unmarshal(memberRecord.Report, &memberReport))
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
	require.NoError(t, runSetReportCmd.RunE(runSetReportCmd, []string{"set_1"}))
	require.ErrorContains(t, runSetReportCmd.RunE(runSetReportCmd, []string{"missing"}), "not found")
}

func TestRunSetPersistsStructuredUnresolvedBindingWhenCurrentSnapshotLacksSource(t *testing.T) {
	runner, pudlDir := setupMutatingRunSetFixture(t, false)
	runner.producerEmpty = true
	runSetConverge = false

	err := runObserveSet(runSetCmd, []string{"consumer", "producer"})
	require.ErrorContains(t, err, "run set")
	report := latestRunSetReportForTest(t, pudlDir)
	require.Len(t, report.Members, 2)
	consumer := report.Members[1]
	assert.Equal(t, "consumer", consumer.Model)
	assert.Equal(t, database.RunStatusFailed, consumer.Result)

	db, openErr := database.NewCatalogDB(pudlDir)
	require.NoError(t, openErr)
	defer db.Close()
	record, reportErr := db.GetRunReport(consumer.RunID)
	require.NoError(t, reportErr)
	require.NotNil(t, record)
	var memberReport RunReport
	require.NoError(t, json.Unmarshal(record.Report, &memberReport))
	require.Len(t, memberReport.BindingIssues, 1)
	issue := memberReport.BindingIssues[0]
	assert.Equal(t, "value", issue.Input)
	assert.Equal(t, "producer", issue.ProducerModel)
	assert.Equal(t, "source-absent", issue.Code)
	assert.Equal(t, "/value", issue.Path)
	assert.Contains(t, issue.Message, "no matching")
}

func TestObserveOnlyRunSetDoesNotExecuteDormantConvergeSealedOutput(t *testing.T) {
	runner, pudlDir := setupMutatingRunSetFixture(t, false)
	runSetConverge = false

	require.NoError(t, runObserveSet(runSetCmd, []string{"sealed-mutator"}))
	assert.Equal(t, []string{"observe //models/sealed-mutator:drift"}, runner.operations)
	assert.Empty(t, runner.applied)
	report := latestRunSetReportForTest(t, pudlDir)
	assert.Equal(t, "observe-only", report.Mode)
	assert.Equal(t, database.RunStatusSucceeded, report.Status)
}

func TestMutatingRunSetPlansAllMembersBeforeApplyingAndPersistsReceipts(t *testing.T) {
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

	muRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(muRoot, "mu.cue"), []byte("package mu\n"), 0o644))
	previousMuRoot := runSetMuRoot
	runSetMuRoot = muRoot
	t.Cleanup(func() { runSetMuRoot = previousMuRoot })
	runner := &runSetMu{configs: map[string]string{}, applied: map[string]int{}}
	previousFactory := runMuRunnerFactory
	runMuRunnerFactory = func() muRunner { return runner }
	t.Cleanup(func() { runMuRunnerFactory = previousFactory })
	previousJSON := jsonOutput
	jsonOutput = true
	t.Cleanup(func() { jsonOutput = previousJSON })
	previousConverge, previousApproval := runSetConverge, runSetRequireApproval
	previousIters, previousApplies := runSetMaxIters, runSetMaxApplies
	runSetConverge, runSetRequireApproval = true, false
	runSetMaxIters, runSetMaxApplies = 3, 10
	t.Cleanup(func() {
		runSetConverge, runSetRequireApproval = previousConverge, previousApproval
		runSetMaxIters, runSetMaxApplies = previousIters, previousApplies
	})
	runSetCmd.Flags().Lookup("max-observation-age").Changed = false

	require.NoError(t, runObserveSet(runSetCmd, []string{"mutator-a"}))
	assert.Equal(t, []string{
		"observe //models/mutator-a:drift",
		"plan //models/mutator-a:drift",
		"observe //models/mutator-a:drift",
		"apply //models/mutator-a:drift",
		"observe //models/mutator-a:drift",
	}, runner.operations)

	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	defer db.Close()
	setRecord, err := db.LatestRunSetReport()
	require.NoError(t, err)
	require.NotNil(t, setRecord)
	var setReport acute.RunSetReport
	require.NoError(t, json.Unmarshal(setRecord.Report, &setReport))
	assert.Equal(t, database.RunStatusSucceeded, setReport.Status)
	assert.Equal(t, "converge", setReport.Mode)
	assert.Len(t, setReport.PlanDigest, 64)
	require.Len(t, setReport.Members, 1)
	assert.True(t, setReport.Members[0].MutationRequired)

	memberRecord, err := db.GetRunReport(setReport.Members[0].RunID)
	require.NoError(t, err)
	require.NotNil(t, memberRecord)
	var memberReport RunReport
	require.NoError(t, json.Unmarshal(memberRecord.Report, &memberReport))
	assert.Equal(t, database.RunStatusSucceeded, memberReport.CompletionStatus)
	require.NotNil(t, memberReport.Converge)
	require.Len(t, memberReport.Converge.MutationReceipts, 1)
	assert.Equal(t, "completed", memberReport.Converge.MutationReceipts[0].Status)
}

func TestMutatingRunSetFailureBlocksDependentsAndCancelsIndependentMutations(t *testing.T) {
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

	muRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(muRoot, "mu.cue"), []byte("package mu\n"), 0o644))
	previousMuRoot := runSetMuRoot
	runSetMuRoot = muRoot
	t.Cleanup(func() { runSetMuRoot = previousMuRoot })
	runner := &runSetMu{configs: map[string]string{}, applied: map[string]int{}, failApply: "mutator-a"}
	previousFactory := runMuRunnerFactory
	runMuRunnerFactory = func() muRunner { return runner }
	t.Cleanup(func() { runMuRunnerFactory = previousFactory })
	previousJSON := jsonOutput
	jsonOutput = true
	t.Cleanup(func() { jsonOutput = previousJSON })
	previousConverge, previousApproval := runSetConverge, runSetRequireApproval
	previousIters, previousApplies := runSetMaxIters, runSetMaxApplies
	runSetConverge, runSetRequireApproval = true, false
	runSetMaxIters, runSetMaxApplies = 3, 10
	t.Cleanup(func() {
		runSetConverge, runSetRequireApproval = previousConverge, previousApproval
		runSetMaxIters, runSetMaxApplies = previousIters, previousApplies
	})
	runSetCmd.Flags().Lookup("max-observation-age").Changed = false

	err = runObserveSet(runSetCmd, []string{"mutator-dependent", "mutator-b", "mutator-a"})
	require.ErrorContains(t, err, "run set")
	planA := indexOfString(runner.operations, "plan //models/mutator-a:drift")
	planB := indexOfString(runner.operations, "plan //models/mutator-b:drift")
	planDependent := indexOfString(runner.operations, "plan //models/mutator-dependent:drift")
	applyA := indexOfString(runner.operations, "apply //models/mutator-a:drift")
	assert.Greater(t, applyA, planA)
	assert.Greater(t, applyA, planB)
	assert.Greater(t, applyA, planDependent, "every read-only plan must finish before the first mutation")
	assert.Equal(t, -1, indexOfString(runner.operations, "apply //models/mutator-b:drift"))
	assert.Equal(t, -1, indexOfString(runner.operations, "apply //models/mutator-dependent:drift"))

	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	defer db.Close()
	record, err := db.LatestRunSetReport()
	require.NoError(t, err)
	require.NotNil(t, record)
	var setReport acute.RunSetReport
	require.NoError(t, json.Unmarshal(record.Report, &setReport))
	assert.Equal(t, database.RunStatusFailed, setReport.Status)
	results := map[string]string{}
	for _, member := range setReport.Members {
		results[member.Model] = member.Result
	}
	assert.Equal(t, database.RunStatusFailed, results["mutator-a"])
	assert.Equal(t, database.RunStatusCancelled, results["mutator-b"])
	assert.Equal(t, database.RunStatusBlocked, results["mutator-dependent"])
}

func TestMutatingRunSetLostReceiptNeedsVerificationAndStopsLaterMutations(t *testing.T) {
	runner, pudlDir := setupMutatingRunSetFixture(t, false)
	runner.invalidManifest = "mutator-a"

	err := runObserveSet(runSetCmd, []string{"mutator-a", "mutator-b"})
	require.ErrorContains(t, err, "run set")
	assert.NotEqual(t, -1, indexOfString(runner.operations, "apply //models/mutator-a:drift"))
	assert.Equal(t, -1, indexOfString(runner.operations, "apply //models/mutator-b:drift"))

	report := latestRunSetReportForTest(t, pudlDir)
	assert.Equal(t, database.RunStatusFailed, report.Status)
	results := map[string]string{}
	for _, member := range report.Members {
		results[member.Model] = member.Result
	}
	assert.Equal(t, database.RunStatusFailed, results["mutator-a"])
	assert.Equal(t, database.RunStatusCancelled, results["mutator-b"])

	db, openErr := database.NewCatalogDB(pudlDir)
	require.NoError(t, openErr)
	defer db.Close()
	record, reportErr := db.GetRunReport(report.Members[0].RunID)
	require.NoError(t, reportErr)
	require.NotNil(t, record)
	var memberReport RunReport
	require.NoError(t, json.Unmarshal(record.Report, &memberReport))
	require.NotNil(t, memberReport.Converge)
	assert.True(t, memberReport.Converge.NeedsVerification)
	assert.Equal(t, string(outcomeNeedsVerification), memberReport.Converge.Outcome)
	assert.Empty(t, memberReport.Converge.MutationReceipts)
	assert.Equal(t, "unknown", runVerdict(&memberReport, runFlags{converge: true}))
}

func indexOfString(values []string, wanted string) int {
	for index, value := range values {
		if value == wanted {
			return index
		}
	}
	return -1
}

func setupMutatingRunSetFixture(t *testing.T, requireApproval bool) (*runSetMu, string) {
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

	muRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(muRoot, "mu.cue"), []byte("package mu\n"), 0o644))
	previousMuRoot := runSetMuRoot
	runSetMuRoot = muRoot
	t.Cleanup(func() { runSetMuRoot = previousMuRoot })
	runner := &runSetMu{configs: map[string]string{}, applied: map[string]int{}}
	previousFactory := runMuRunnerFactory
	runMuRunnerFactory = func() muRunner { return runner }
	t.Cleanup(func() { runMuRunnerFactory = previousFactory })
	previousJSON := jsonOutput
	jsonOutput = true
	t.Cleanup(func() { jsonOutput = previousJSON })
	previousConverge, previousApproval := runSetConverge, runSetRequireApproval
	previousIters, previousApplies := runSetMaxIters, runSetMaxApplies
	runSetConverge, runSetRequireApproval = true, requireApproval
	runSetMaxIters, runSetMaxApplies = 3, 10
	t.Cleanup(func() {
		runSetConverge, runSetRequireApproval = previousConverge, previousApproval
		runSetMaxIters, runSetMaxApplies = previousIters, previousApplies
	})
	runSetCmd.Flags().Lookup("max-observation-age").Changed = false
	return runner, pudlDir
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

func TestMutatingRunSetExactApprovalRevalidatesBeforeApplying(t *testing.T) {
	runner, pudlDir := setupMutatingRunSetFixture(t, true)
	require.NoError(t, runObserveSet(runSetCmd, []string{"mutator-a"}))
	assert.Equal(t, -1, indexOfString(runner.operations, "apply //models/mutator-a:drift"))
	pending := latestRunSetReportForTest(t, pudlDir)
	assert.Equal(t, "pending-approval", pending.Status)
	assert.Equal(t, "pending", pending.ApprovalStatus)
	require.Len(t, pending.Members, 1)
	assert.Equal(t, database.RunStatusRunning, pending.Members[0].Result)

	require.NoError(t, resumeRunSet(pending.RunSetID))
	assert.NotEqual(t, -1, indexOfString(runner.operations, "apply //models/mutator-a:drift"))
	completed := latestRunSetReportForTest(t, pudlDir)
	assert.Equal(t, database.RunStatusSucceeded, completed.Status)
	assert.Equal(t, "approved", completed.ApprovalStatus)

	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	defer db.Close()
	approval, err := db.GetRunSetApproval(pending.RunSetID)
	require.NoError(t, err)
	require.NotNil(t, approval)
	assert.Equal(t, "approved", approval.Status)
}

func TestMutatingRunSetChangedPlanInvalidatesApprovalWithoutApplying(t *testing.T) {
	runner, pudlDir := setupMutatingRunSetFixture(t, true)
	require.NoError(t, runObserveSet(runSetCmd, []string{"mutator-a"}))
	pending := latestRunSetReportForTest(t, pudlDir)
	runner.planSuffix = " changed"

	err := resumeRunSet(pending.RunSetID)
	require.ErrorContains(t, err, "approval is stale")
	assert.Equal(t, -1, indexOfString(runner.operations, "apply //models/mutator-a:drift"))
	stale := latestRunSetReportForTest(t, pudlDir)
	assert.Equal(t, database.RunStatusFailed, stale.Status)
	assert.Equal(t, "stale", stale.ApprovalStatus)
	assert.Equal(t, database.RunStatusCancelled, stale.Members[0].Result)
}

func TestMutatingRunSetRejectionPerformsNoMutation(t *testing.T) {
	runner, pudlDir := setupMutatingRunSetFixture(t, true)
	require.NoError(t, runObserveSet(runSetCmd, []string{"mutator-a"}))
	pending := latestRunSetReportForTest(t, pudlDir)

	require.NoError(t, rejectRunSet(pending.RunSetID))
	assert.Equal(t, -1, indexOfString(runner.operations, "apply //models/mutator-a:drift"))
	rejected := latestRunSetReportForTest(t, pudlDir)
	assert.Equal(t, database.RunStatusFailed, rejected.Status)
	assert.Equal(t, "rejected", rejected.ApprovalStatus)
	assert.Equal(t, database.RunStatusCancelled, rejected.Members[0].Result)
}

func TestSealedOutputForcesExactApprovalAndRecordsStrictActionRouting(t *testing.T) {
	runner, pudlDir := setupMutatingRunSetFixture(t, false)
	wsPolicy.Workspace = &workspace.Workspace{PudlDir: pudlDir,
		SecretsWritableRefs: []string{"pass:apps/*"}, SecretsWritableConfigured: true}

	require.NoError(t, runObserveSet(runSetCmd, []string{"sealed-mutator"}))
	pending := latestRunSetReportForTest(t, pudlDir)
	assert.Equal(t, "pending-approval", pending.Status)
	assert.Equal(t, "pending", pending.ApprovalStatus)
	assert.Equal(t, -1, indexOfString(runner.operations, "apply //models/sealed-mutator:drift"))

	generated := runner.configs["//models/sealed-mutator:drift"]
	assert.Contains(t, generated, `sealed_routing: "strict"`)
	assert.Contains(t, generated, `"TOKEN":"pass:apps/token"`)
	assert.Contains(t, generated, `writable_refs: ["pass:apps/*"]`)

	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	approval, err := db.GetRunSetApproval(pending.RunSetID)
	require.NoError(t, err)
	require.NotNil(t, approval)
	assert.NotContains(t, string(approval.Plan), "pass:apps/token", "durable exact plan must redact provider paths")
	require.NoError(t, db.Close())

	require.NoError(t, resumeRunSet(pending.RunSetID))
	completed := latestRunSetReportForTest(t, pudlDir)
	assert.Equal(t, database.RunStatusSucceeded, completed.Status)
	require.Len(t, completed.Members, 1)
	db, err = database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	defer db.Close()
	record, err := db.GetRunReport(completed.Members[0].RunID)
	require.NoError(t, err)
	require.NotNil(t, record)
	var member RunReport
	require.NoError(t, json.Unmarshal(record.Report, &member))
	require.Len(t, member.SealedBindings, 1)
	assert.Equal(t, "//models/sealed-mutator:drift:write-token", member.SealedBindings[0].ProducingActionID)
	assert.Equal(t, "stored", member.SealedBindings[0].LifecycleStatus)
	assert.NotContains(t, string(record.Report), "pass:apps/token")
}

func TestSealedMutationFailureRedactsProviderPathFromReportAndError(t *testing.T) {
	runner, pudlDir := setupMutatingRunSetFixture(t, false)
	wsPolicy.Workspace = &workspace.Workspace{PudlDir: pudlDir,
		SecretsWritableRefs: []string{"pass:apps/*"}, SecretsWritableConfigured: true}

	require.NoError(t, runObserveSet(runSetCmd, []string{"sealed-mutator"}))
	pending := latestRunSetReportForTest(t, pudlDir)
	runner.failApply = "sealed-mutator"
	err := resumeRunSet(pending.RunSetID)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "apps/token")

	completed := latestRunSetReportForTest(t, pudlDir)
	require.Len(t, completed.Members, 1)
	db, openErr := database.NewCatalogDB(pudlDir)
	require.NoError(t, openErr)
	defer db.Close()
	record, reportErr := db.GetRunReport(completed.Members[0].RunID)
	require.NoError(t, reportErr)
	require.NotNil(t, record)
	assert.NotContains(t, string(record.Report), "apps/token")
	assert.Contains(t, string(record.Report), "sealed-ref:pass")
}

func TestCrossModelSealedReferencePlansAndExecutesWithoutPUDLValueAccess(t *testing.T) {
	runner, pudlDir := setupMutatingRunSetFixture(t, false)
	wsPolicy.Workspace = &workspace.Workspace{PudlDir: pudlDir,
		SecretsWritableRefs: []string{"pass:apps/*"}, SecretsWritableConfigured: true}

	require.NoError(t, runObserveSet(runSetCmd, []string{"mutator-secret-consumer", "mutator-secret-producer"}))
	pending := latestRunSetReportForTest(t, pudlDir)
	assert.Equal(t, []string{"mutator-secret-producer", "mutator-secret-consumer"}, pending.Ordered)
	assert.Equal(t, "pending-approval", pending.Status)
	assert.Contains(t, runner.configs["//models/mutator-secret-consumer:drift"], `"TOKEN":"pass:apps/token"`)
	assert.Contains(t, runner.configs["//models/mutator-secret-consumer:drift"], `sealed_routing: "strict"`)

	require.NoError(t, resumeRunSet(pending.RunSetID))
	producerApply := indexOfString(runner.operations, "apply //models/mutator-secret-producer:drift")
	consumerApply := indexOfString(runner.operations, "apply //models/mutator-secret-consumer:drift")
	assert.Greater(t, producerApply, -1)
	assert.Greater(t, consumerApply, producerApply)

	completed := latestRunSetReportForTest(t, pudlDir)
	db, openErr := database.NewCatalogDB(pudlDir)
	require.NoError(t, openErr)
	defer db.Close()
	for _, member := range completed.Members {
		record, reportErr := db.GetRunReport(member.RunID)
		require.NoError(t, reportErr)
		require.NotNil(t, record)
		assert.NotContains(t, string(record.Report), "pass:apps/token")
		var memberReport RunReport
		require.NoError(t, json.Unmarshal(record.Report, &memberReport))
		require.Len(t, memberReport.SealedBindings, 1)
		if member.Model == "mutator-secret-consumer" {
			assert.Equal(t, "delivered", memberReport.SealedBindings[0].LifecycleStatus)
			assert.Equal(t, "mutator-secret-producer", memberReport.SealedBindings[0].ProducerModel)
			assert.Equal(t, []string{"//models/mutator-secret-consumer:drift:read-token"}, memberReport.SealedBindings[0].ClaimingActionIDs)
		} else {
			assert.Equal(t, "stored", memberReport.SealedBindings[0].LifecycleStatus)
		}
	}
}

func TestCompletedSealedWriteRemainsRecordedWhenLaterConsumerFails(t *testing.T) {
	runner, pudlDir := setupMutatingRunSetFixture(t, false)
	wsPolicy.Workspace = &workspace.Workspace{PudlDir: pudlDir,
		SecretsWritableRefs: []string{"pass:apps/*"}, SecretsWritableConfigured: true}

	require.NoError(t, runObserveSet(runSetCmd, []string{"mutator-secret-consumer", "mutator-secret-producer"}))
	pending := latestRunSetReportForTest(t, pudlDir)
	runner.failApply = "mutator-secret-consumer"
	err := resumeRunSet(pending.RunSetID)
	require.ErrorContains(t, err, "run set")

	completed := latestRunSetReportForTest(t, pudlDir)
	assert.Equal(t, database.RunStatusFailed, completed.Status)
	db, openErr := database.NewCatalogDB(pudlDir)
	require.NoError(t, openErr)
	defer db.Close()
	record, reportErr := db.GetRunReport(completed.Members[0].RunID)
	require.NoError(t, reportErr)
	require.NotNil(t, record)
	var producer RunReport
	require.NoError(t, json.Unmarshal(record.Report, &producer))
	require.NotNil(t, producer.Converge)
	require.Len(t, producer.Converge.MutationReceipts, 1)
	assert.Equal(t, "completed", producer.Converge.MutationReceipts[0].Status)
	require.Len(t, producer.SealedBindings, 1)
	assert.Equal(t, "stored", producer.SealedBindings[0].LifecycleStatus)
	assert.Equal(t, database.RunStatusSucceeded, completed.Members[0].Result)
	assert.Equal(t, database.RunStatusFailed, completed.Members[1].Result)
}

func TestAnnotateSealedActionClaimsRecordsExactRouting(t *testing.T) {
	report := RunReport{SealedBindings: []wiring.SealedBindingEvidence{
		{Direction: "input", ConsumerPhase: "converge", Input: "TOKEN"},
		{Direction: "output", ProducerPhase: "converge", Output: "RESULT"},
		{Direction: "input", ConsumerPhase: "populate", ProducerPhase: "converge", Input: "POPULATE_TOKEN"},
	}}
	plan := []byte(`{
		"actions": [
			{"id":"//models/app:converge:write","sealed_inputs":{"TOKEN":"env:TOKEN"},"sealed_outputs":{"RESULT":"pass:result"}},
			{"id":"//models/app:converge:read","sealed_inputs":{"TOKEN":"env:TOKEN"}}
		]
	}`)
	require.NoError(t, annotateSealedActionClaims(&report, plan))
	assert.Equal(t, []string{"//models/app:converge:read", "//models/app:converge:write"}, report.SealedBindings[0].ClaimingActionIDs)
	assert.Equal(t, "//models/app:converge:write", report.SealedBindings[1].ProducingActionID)
	assert.Empty(t, report.SealedBindings[2].ClaimingActionIDs, "populate input is not claimed by the converge plan")
}

func TestAnnotateSealedActionClaimsRejectsMissingAndAmbiguousRouting(t *testing.T) {
	input := RunReport{SealedBindings: []wiring.SealedBindingEvidence{{
		Direction: "input", ConsumerPhase: "converge", Input: "TOKEN",
	}}}
	assert.ErrorContains(t, annotateSealedActionClaims(&input, []byte(`{"actions":[]}`)), "no claiming action")

	output := RunReport{SealedBindings: []wiring.SealedBindingEvidence{{
		Direction: "output", ProducerPhase: "converge", Output: "RESULT",
	}}}
	plan := []byte(`{"actions":[
		{"id":"a","sealed_outputs":{"RESULT":"pass:result"}},
		{"id":"b","sealed_outputs":{"RESULT":"pass:result"}}
	]}`)
	assert.ErrorContains(t, annotateSealedActionClaims(&output, plan), "multiple producing actions")
}

func TestCanonicalMutationPlanNormalizesOnlyEphemeralWorkspaceIdentity(t *testing.T) {
	left, err := canonicalMutationPlan([]byte(`{
		"version":2,
		"actions":[{"id":"apply","action_key":"sha256:left","work_dir":"/tmp/left","inputs":{"/tmp/left/desired.json":"sha256:content"},"sealed_output_modes":{"TOKEN":"create"}}]
	}`), "/tmp/left")
	require.NoError(t, err)
	right, err := canonicalMutationPlan([]byte(`{
		"actions":[{"sealed_output_modes":{"TOKEN":"create"},"inputs":{"/tmp/right/desired.json":"sha256:content"},"work_dir":"/tmp/right","action_key":"sha256:right","id":"apply"}],
		"version":2
	}`), "/tmp/right")
	require.NoError(t, err)
	assert.Equal(t, string(left), string(right))
	assert.NotContains(t, string(left), "action_key")
	assert.Contains(t, string(left), "pudl-reconcile-workspace")

	changed, err := canonicalMutationPlan([]byte(`{
		"version":2,
		"actions":[{"id":"apply","work_dir":"/tmp/changed","inputs":{"/tmp/changed/desired.json":"sha256:content"},"sealed_output_modes":{"TOKEN":"overwrite"}}]
	}`), "/tmp/changed")
	require.NoError(t, err)
	assert.NotEqual(t, string(left), string(changed), "store mode remains exact plan identity")
}
