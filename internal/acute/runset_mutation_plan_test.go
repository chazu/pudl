package acute

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/wiring"
)

func mutationTestModel() *systemmodel.SystemModel {
	return &systemmodel.SystemModel{
		Name:     "app",
		Plugins:  []systemmodel.PluginDef{{Name: "fake", Digest: "sha256:plugin"}},
		Populate: systemmodel.Populate{Plugin: "fake"},
		Desired:  []map[string]any{{"_schema": "resources.#App", "name": "app", "subnet": "subnet-1"}},
		Converge: &systemmodel.PluginPlan{
			Plugin: "fake",
			Input:  map[string]any{"region": "east"},
			SealedInputs: map[string]systemmodel.SealedInput{
				"TOKEN": {Ref: "pass:apps/token", DeliveryMode: "env"},
			},
			SealedOutputs: map[string]systemmodel.SealedOutput{
				"GENERATED": {Ref: "pass:apps/generated", StoreMode: "create_if_absent"},
			},
		},
	}
}

func mutationTestEvidence(t *testing.T) []wiring.BindingEvidence {
	t.Helper()
	maxAge := time.Hour
	return []wiring.BindingEvidence{{
		Input: "subnet", ProducerModel: "network", ProducerRunID: "run_network",
		SnapshotID: "snap_network", Workspace: "repo", Schema: "resources.#Subnet",
		Identity: map[string]any{"name": "private"}, Path: "/id", Selection: "current-run",
		ObservedAt: time.Unix(100, 0), EvaluatedAt: time.Unix(200, 0), Age: 100 * time.Second,
		MaxAge: &maxAge, Value: json.RawMessage(`"subnet-1"`), ValueType: "string",
		ScalarSHA256: "scalar", ResolutionCode: "resolved",
	}}
}

func TestRunSetMutationPlanDigestCommitsExecutableInputsWithoutReferencePaths(t *testing.T) {
	member, err := NewRunSetMutationMemberPlan(mutationTestModel(), "run_app", "snap_app", mutationTestEvidence(t), []byte("action app"), true)
	require.NoError(t, err)
	plan := RunSetMutationPlan{
		PlanVersion: 1, RunSetID: "set_1", Mode: "converge",
		Options: RunSetMutationOptions{MaxObservationAge: "1h0m0s", MaxIterations: 5, MaxApplies: 20},
		Edges:   []RunSetEdge{{From: "app", To: "network", Sources: []string{"binding"}}},
		Ordered: []string{"network", "app"}, Members: []RunSetMutationMemberPlan{member},
		WritableRefsConfigured:  true,
		WritableRefFingerprints: FingerprintWritableRefs([]string{"pass:apps/*"}),
	}
	digest, err := plan.CanonicalDigest()
	require.NoError(t, err)
	assert.Len(t, digest, 64)

	encoded, err := json.Marshal(plan)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "pass:apps/token")
	assert.NotContains(t, string(encoded), "pass:apps/generated")
	assert.NotContains(t, string(encoded), "pass:apps/*")
	assert.Contains(t, string(encoded), `"subnet-1"`)

	changed := plan
	changed.Members = append([]RunSetMutationMemberPlan(nil), plan.Members...)
	changed.Members[0].MuPlanSHA256 = "different"
	changedDigest, err := changed.CanonicalDigest()
	require.NoError(t, err)
	assert.NotEqual(t, digest, changedDigest)

	// Wall-clock evaluation evidence is intentionally non-semantic. The stable
	// commitment contains the pinned snapshot/value and age policy instead.
	updatedEvidence := mutationTestEvidence(t)
	updatedEvidence[0].EvaluatedAt = time.Unix(999, 0)
	updatedMember, err := NewRunSetMutationMemberPlan(mutationTestModel(), "run_app", "snap_app", updatedEvidence, []byte("action app"), true)
	require.NoError(t, err)
	assert.Equal(t, member, updatedMember)
}

func TestRunSetMutationPlanRejectsUnresolvedSealedSource(t *testing.T) {
	model := mutationTestModel()
	model.Converge.SealedInputs["TOKEN"] = systemmodel.SealedInput{
		Source: &systemmodel.SealedSource{Model: "secrets", Output: "TOKEN"}, DeliveryMode: "env",
	}
	_, err := NewRunSetMutationMemberPlan(model, "run_app", "snap_app", nil, []byte("action app"), true)
	require.ErrorContains(t, err, "unresolved producer source")
}
