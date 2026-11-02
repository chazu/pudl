package systemmodel

import (
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadModel compiles a model source (CUE referencing #SystemModel) together with
// the embedded schema, looks up the named instance, and decodes it via the
// production DecodeValue path. Test-only harness; the live resolver loads
// instances from the schema repo (cmd/run_resolve.go).
func loadModel(src []byte, name string) (*SystemModel, error) {
	ctx := cuecontext.New()

	combined := schemaCUE + "\n" + string(src)
	v := ctx.CompileString(combined, cue.Filename("model.cue"))
	if v.Err() != nil {
		return nil, fmt.Errorf("compile model: %w", v.Err())
	}

	inst := v.LookupPath(cue.ParsePath(name))
	if !inst.Exists() {
		return nil, fmt.Errorf("model instance %q not found in source", name)
	}
	m, err := DecodeValue(inst)
	if err != nil {
		return nil, fmt.Errorf("model %q: %w", name, err)
	}
	return m, nil
}

// observe-only command model with a check.
const commandObserveModel = `
pods: #SystemModel & {
	name: "pods"
	populate: {
		timeout: "2m"
		runs: [{argv: ["kubectl", "get", "pods", "-o", "json"], set: cluster: "prod"}]
	}
	checks: [{
		name:     "pdb_present"
		query:    "missing_pdb"
		expect:   "empty"
		severity: "warn"
		message:  "workload without PodDisruptionBudget"
	}]
}
`

func TestLoadModel_ObserveOnly(t *testing.T) {
	m, err := loadModel([]byte(commandObserveModel), "pods")
	require.NoError(t, err)

	assert.Equal(t, "pods", m.Name)
	assert.Equal(t, KindCommand, m.Populate.Kind())
	assert.Equal(t, "2m", m.Populate.Timeout)
	require.Len(t, m.Populate.Runs, 1)
	assert.Equal(t, "prod", m.Populate.Runs[0].Set["cluster"])

	require.Len(t, m.Checks, 1)
	assert.Equal(t, "pdb_present", m.Checks[0].Name)
	assert.Equal(t, "missing_pdb", m.Checks[0].Query)
	assert.Equal(t, "empty", m.Checks[0].Expect)
	assert.Equal(t, "warn", m.Checks[0].Severity)
}

func TestLoadModel_MissingInstance(t *testing.T) {
	_, err := loadModel([]byte(commandObserveModel), "nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestLoadModel_RejectsInvalid(t *testing.T) {
	// Without populate a model is checks-only, so it may not declare desired
	// state (there is nothing to observe it against).
	const bad = `bad: #SystemModel & {name: "x", desired: [{"_schema": "a.#B", name: "n"}]}`
	_, err := loadModel([]byte(bad), "bad")
	require.Error(t, err)
}

func TestLoadModel_ChecksOnlyAndCommandArms(t *testing.T) {
	m, err := loadModel([]byte(`m: #SystemModel & {name: "posture", checks: [{name: "c", query: "q", expect: "empty", severity: "fail", message: "m"}]}`), "m")
	require.NoError(t, err)
	require.Equal(t, KindNone, m.Populate.Kind())

	m, err = loadModel([]byte(`m: #SystemModel & {name: "fw", populate: {schema: "pudl/gcp.#Firewall", runs: [{argv: ["gcloud", "list"], set: project: "p"}]}}`), "m")
	require.NoError(t, err)
	require.Equal(t, KindCommand, m.Populate.Kind())
	require.Equal(t, []string{"gcloud", "list"}, m.Populate.Runs[0].Argv)
	require.Equal(t, "p", m.Populate.Runs[0].Set["project"])

	_, err = loadModel([]byte(`m: #SystemModel & {name: "fw", populate: {runs: []}}`), "m")
	require.Error(t, err, "a command arm needs at least one run")
}
