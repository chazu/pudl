package cmd

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/chazu/pudl/internal/systemmodel"
)

func TestRedactSealedErrorRemovesOperationalProviderPaths(t *testing.T) {
	model := &systemmodel.SystemModel{
		Populate: systemmodel.Populate{SealedInputs: map[string]systemmodel.SealedInput{
			"TOKEN": {Ref: "vault:production/team/token"},
		}},
		Converge: &systemmodel.PluginPlan{SealedOutputs: map[string]systemmodel.SealedOutput{
			"ROTATED": {Ref: "pass:apps/rotated-token"},
		}},
	}

	err := redactSealedError(errors.New("resolve vault:production/team/token then store pass:apps/rotated-token"), model)

	assert.NotContains(t, err.Error(), "production/team/token")
	assert.NotContains(t, err.Error(), "apps/rotated-token")
	assert.Contains(t, err.Error(), "<sealed-ref:vault:")
	assert.Contains(t, err.Error(), "<sealed-ref:pass:")
}

func TestRedactSealedErrorPreservesCause(t *testing.T) {
	cause := errors.New("provider failed")
	assert.ErrorIs(t, redactSealedError(cause, &systemmodel.SystemModel{}), cause)
}
