package systemmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadModel_EmptyWhen(t *testing.T) {
	m, err := loadModel([]byte(`
fw: #SystemModel & {
	name: "fw"
	populate: {
		empty_when: ["SERVICE_DISABLED", "API .* is disabled"]
		runs: [{argv: ["gcloud", "compute", "firewall-rules", "list", "--project=proj-a"]}]
	}
}
`), "fw")
	require.NoError(t, err)
	assert.Equal(t, []string{"SERVICE_DISABLED", "API .* is disabled"}, m.Populate.EmptyWhen)
	patterns, err := m.Populate.EmptyPatterns()
	require.NoError(t, err)
	assert.Len(t, patterns, 2)
	assert.False(t, m.Populate.IsZero())
}

// A pattern that is not a valid RE2 expression fails the load, naming the
// field.
func TestLoadModel_EmptyWhenRejectsABadPattern(t *testing.T) {
	_, err := loadModel([]byte(`
fw: #SystemModel & {
	name: "fw"
	populate: {
		empty_when: ["SERVICE_DISABLED", "(unclosed"]
		runs: [{argv: ["list"]}]
	}
}
`), "fw")
	require.ErrorContains(t, err, `populate.empty_when[1] "(unclosed" is not a valid regular expression`)
}

func TestLoadModel_EmptyWhenRejectsAnEmptyPattern(t *testing.T) {
	_, err := loadModel([]byte(`
fw: #SystemModel & {
	name: "fw"
	populate: {
		empty_when: [""]
		runs: [{argv: ["list"]}]
	}
}
`), "fw")
	require.Error(t, err)

	_, err = Populate{EmptyWhen: []string{""}}.EmptyPatterns()
	require.ErrorContains(t, err, "populate.empty_when[0] is empty")
}
