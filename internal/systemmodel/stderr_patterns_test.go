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

func TestLoadModel_FailWhen(t *testing.T) {
	m, err := loadModel([]byte(`
fw: #SystemModel & {
	name: "fw"
	populate: {
		fail_when: ["Some requests did not succeed"]
		runs: [{argv: ["gcloud", "compute", "addresses", "list", "--project=proj-a"]}]
	}
}
`), "fw")
	require.NoError(t, err)
	assert.Equal(t, []string{"Some requests did not succeed"}, m.Populate.FailWhen)
	assert.False(t, m.Populate.IsZero())
}

// A bad or empty fail_when pattern fails the load, also in a fallback.
func TestLoadModel_FailWhenRejectsBadPatterns(t *testing.T) {
	_, err := loadModel([]byte(`
fw: #SystemModel & {
	name: "fw"
	populate: {
		fail_when: ["(unclosed"]
		runs: [{argv: ["list"]}]
	}
}
`), "fw")
	require.ErrorContains(t, err, `populate.fail_when[0] "(unclosed" is not a valid regular expression`)

	_, err = loadModel([]byte(`
fw: #SystemModel & {
	name: "fw"
	populate: {
		runs: [{argv: ["search"]}]
		fallback: {
			when: ["PERMISSION_DENIED"]
			observe: {fail_when: ["(unclosed"], runs: [{argv: ["list"]}]}
		}
	}
}
`), "fw")
	require.ErrorContains(t, err, `populate.fallback.observe: populate.fail_when[0]`)

	_, err = Populate{FailWhen: []string{""}}.FailPatterns()
	require.ErrorContains(t, err, "populate.fail_when[0] is empty, so it would match every run")
}
