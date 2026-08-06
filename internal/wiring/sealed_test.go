package wiring

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/systemmodel"
)

func TestResolveSealedSourcesUsesProducerOwnedReferenceAndRedactsEvidence(t *testing.T) {
	producer := &systemmodel.SystemModel{
		Name: "secrets",
		Converge: &systemmodel.PluginPlan{SealedOutputs: map[string]systemmodel.SealedOutput{
			"TOKEN": {Ref: "pass:apps/token", StoreMode: "create_if_absent"},
		}},
	}
	consumer := &systemmodel.SystemModel{
		Name: "app",
		Populate: systemmodel.Populate{SealedInputs: map[string]systemmodel.SealedInput{
			"API_TOKEN": {
				Source:       &systemmodel.SealedSource{Model: "SecretsDef", Output: "TOKEN"},
				DeliveryMode: "file",
			},
		}},
	}

	resolved, err := ResolveSealedSources([]SealedMember{
		{Model: consumer, RunID: "run_app"},
		{Model: producer, Aliases: []string{"SecretsDef"}, RunID: "run_secrets"},
	}, SealedPolicy{WritableConfigured: true, WritableRefs: []string{"pass:apps/*"}})
	require.NoError(t, err)
	require.Len(t, resolved, 2)
	app := resolved[0]
	assert.Equal(t, "app", app.Model.Name)
	assert.Equal(t, "pass:apps/token", app.Model.Populate.SealedInputs["API_TOKEN"].Ref)
	assert.Empty(t, consumer.Populate.SealedInputs["API_TOKEN"].Ref, "resolution must not mutate the authored runtime projection")
	require.Len(t, app.Evidence, 1)
	assert.Equal(t, "producer-output", app.Evidence[0].SourceKind)
	assert.Equal(t, "secrets", app.Evidence[0].ProducerModel)
	assert.Equal(t, "create_if_absent", app.Evidence[0].StoreMode)
	assert.Equal(t, "pass", app.Evidence[0].ProviderScheme)

	encoded, err := MarshalSealedEvidence(app.Evidence)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "apps/token")
	assert.NotContains(t, string(encoded), "pass:apps/token")
	assert.Contains(t, string(encoded), "reference_sha256")

	producerEvidence, err := MarshalSealedEvidence(resolved[1].Evidence)
	require.NoError(t, err)
	assert.NotContains(t, string(producerEvidence), "pass:apps/*", "writable policy paths are durable only as fingerprints")
	assert.Contains(t, string(producerEvidence), "matched_writable_pattern_sha256")
}

func TestResolveSealedSourcesEnforcesWritablePolicyAndExactSet(t *testing.T) {
	producer := &systemmodel.SystemModel{
		Name: "secrets",
		Converge: &systemmodel.PluginPlan{SealedOutputs: map[string]systemmodel.SealedOutput{
			"TOKEN": {Ref: "pass:apps/token", StoreMode: "overwrite"},
		}},
	}
	_, err := ResolveSealedSources([]SealedMember{{Model: producer, RunID: "run_secrets"}}, SealedPolicy{WritableConfigured: true})
	require.ErrorContains(t, err, "not allowed")

	consumer := &systemmodel.SystemModel{
		Name: "app",
		Populate: systemmodel.Populate{SealedInputs: map[string]systemmodel.SealedInput{
			"TOKEN": {Source: &systemmodel.SealedSource{Model: "missing", Output: "TOKEN"}, DeliveryMode: "env"},
		}},
	}
	_, err = ResolveSealedSources([]SealedMember{{Model: consumer, RunID: "run_app"}}, SealedPolicy{})
	require.ErrorContains(t, err, "outside the exact run set")
}

func TestResolveSealedSourcesValidatesDirectReferencesAndPatterns(t *testing.T) {
	consumer := &systemmodel.SystemModel{
		Name: "app",
		Populate: systemmodel.Populate{SealedInputs: map[string]systemmodel.SealedInput{
			"TOKEN": {Ref: "env:TOKEN", DeliveryMode: "env"},
		}},
	}
	resolved, err := ResolveSealedSources([]SealedMember{{Model: consumer, RunID: "run_app"}}, SealedPolicy{})
	require.NoError(t, err)
	require.Len(t, resolved[0].Evidence, 1)
	assert.Equal(t, "direct-ref", resolved[0].Evidence[0].SourceKind)

	_, err = ResolveSealedSources([]SealedMember{{Model: consumer, RunID: "run_app"}}, SealedPolicy{
		WritableConfigured: true, WritableRefs: []string{"pass:[unterminated"},
	})
	require.ErrorContains(t, err, "invalid pattern")

	consumer.Populate.SealedInputs["TOKEN"] = systemmodel.SealedInput{Ref: "not-a-reference", DeliveryMode: "env"}
	_, err = ResolveSealedSources([]SealedMember{{Model: consumer, RunID: "run_app"}}, SealedPolicy{})
	require.ErrorContains(t, err, "scheme:non-empty-path")
}
