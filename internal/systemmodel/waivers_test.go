package systemmodel

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// waiverModel is a checks-only model with one check and the given waivers.
func waiverModel(waivers string) []byte {
	return []byte(`
app: #SystemModel & {
	name: "app"
	checks: [{name: "open", query: "open_port", expect: "empty", severity: "fail", message: "m"},
		{name: "present", query: "thing", expect: "nonempty", severity: "fail", message: "m"}]
	waivers: [` + waivers + `]
}`)
}

func TestWaiversDecodeBothTargets(t *testing.T) {
	m, err := loadModel(waiverModel(`
		{check: "open", witness: {port: 22, host: "a"}, owner: "sre", reason: "bastion", created: "2026-11-01", expires: "2026-12-31"},
		{policy: "gcp.#BucketBestPractice", rule: "gcp-bucket-uniform-access", resource: "abc", owner: "data", reason: "legacy ACLs", created: "2026-11-01", expires: "2027-01-31"}`), "app")
	require.NoError(t, err)
	require.Len(t, m.Waivers, 2)
	require.Equal(t, "open", m.Waivers[0].Check)
	require.Equal(t, "a", m.Waivers[0].Witness["host"])
	require.True(t, m.Waivers[1].IsPolicy())
	require.Equal(t, "2027-01-31", m.Waivers[1].Expires)
}

// Waivers are validated strictly at load: CUE rejects a missing required
// field or a malformed date, and the loader rejects an impossible date or a
// target the model does not have.
func TestWaiversRejectedAtLoad(t *testing.T) {
	cases := map[string]struct{ waiver, want string }{
		"missing owner":   {`{check: "open", witness: {port: 22}, reason: "r", created: "2026-11-01", expires: "2026-12-31"}`, "owner"},
		"missing expires": {`{check: "open", witness: {port: 22}, owner: "o", reason: "r", created: "2026-11-01"}`, "expires"},
		"malformed date":  {`{check: "open", witness: {port: 22}, owner: "o", reason: "r", created: "2026-11-01", expires: "31/12/2026"}`, "expires"},
		"impossible date": {`{check: "open", witness: {port: 22}, owner: "o", reason: "r", created: "2026-11-01", expires: "2026-02-30"}`, `expires "2026-02-30" is not a YYYY-MM-DD date`},
		"expires first":   {`{check: "open", witness: {port: 22}, owner: "o", reason: "r", created: "2026-11-01", expires: "2026-10-31"}`, "before created"},
		"no target":       {`{owner: "o", reason: "r", created: "2026-11-01", expires: "2026-12-31"}`, "requires a target"},
		"both targets":    {`{check: "open", witness: {port: 22}, policy: "p", rule: "r", resource: "x", owner: "o", reason: "r", created: "2026-11-01", expires: "2026-12-31"}`, "not both"},
		"empty selector":  {`{check: "open", witness: {}, owner: "o", reason: "r", created: "2026-11-01", expires: "2026-12-31"}`, "non-empty witness"},
		"partial policy":  {`{policy: "p", rule: "r", owner: "o", reason: "r", created: "2026-11-01", expires: "2026-12-31"}`, "requires policy, rule and resource"},
		"unknown check":   {`{check: "closed", witness: {port: 22}, owner: "o", reason: "r", created: "2026-11-01", expires: "2026-12-31"}`, `check "closed" is not a check of this model`},
		"nonempty check":  {`{check: "present", witness: {port: 22}, owner: "o", reason: "r", created: "2026-11-01", expires: "2026-12-31"}`, "only expect"},
		"unknown field":   {`{check: "open", witness: {port: 22}, owner: "o", reason: "r", created: "2026-11-01", expires: "2026-12-31", until: "x"}`, "until"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := loadModel(waiverModel(tc.waiver), "app")
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestWaiversRequireChecks(t *testing.T) {
	_, err := loadModel([]byte(`
app: #SystemModel & {
	name: "app"
	waivers: [{policy: "p", rule: "r", resource: "x", owner: "o", reason: "r", created: "2026-11-01", expires: "2026-12-31"}]
}`), "app")
	require.ErrorContains(t, err, "declares no checks to waive")
}
