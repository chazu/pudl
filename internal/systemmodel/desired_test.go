package systemmodel

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeValuePreservesDesiredFieldNames(t *testing.T) {
	model, err := loadModel([]byte(`
inventory: #SystemModel & {
	name: "git-inventory"
	populate: {plugin: "git-inventory", differential: false, input: {}}
	desired: [{
		"_schema": "git.repository"
		name: "local/demo"
		default_branch: "main"
		"display name": "Demo"
	}]
}`), "inventory")
	require.NoError(t, err)
	require.Equal(t, []map[string]any{{
		"_schema": "git.repository", "name": "local/demo",
		"default_branch": "main", "display name": "Demo",
	}}, model.Desired)
}
