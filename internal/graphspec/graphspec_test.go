package graphspec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAndEval(t *testing.T) {
	spec, err := Parse(json.RawMessage(`{
		"name": "name",
		"account": "project",
		"region": {"path": "region", "last_segment": true},
		"refs": [{"path": "selfLink", "trim_prefix": "https://x/v1/"}, "id", {"path": "email", "prepend": "serviceAccount:"}]
	}`))
	require.NoError(t, err)
	rec := map[string]any{
		"name":     "net-a",
		"project":  "p1",
		"region":   "https://x/v1/projects/p1/regions/us-central1",
		"selfLink": "https://x/v1/projects/p1/global/networks/net-a",
		"id":       json.Number("42"),
		"email":    "etl@p1.iam.gserviceaccount.com",
	}
	require.Equal(t, "net-a", Eval(spec.Name, rec))
	require.Equal(t, "p1", Eval(spec.Account, rec))
	require.Equal(t, "us-central1", Eval(spec.Region, rec))
	require.Equal(t, "", Eval(spec.Zone, rec))
	require.Equal(t, []string{"projects/p1/global/networks/net-a", "42", "serviceAccount:etl@p1.iam.gserviceaccount.com"}, spec.RefValues(rec))
}

func TestParseRejectsMistakes(t *testing.T) {
	for name, raw := range map[string]string{
		"unknown key":       `{"nmae": "name"}`,
		"unknown value key": `{"name": {"path": "name", "trim": "x"}}`,
		"wildcard":          `{"refs": ["items[*].id"]}`,
		"empty path":        `{"name": {"path": ""}}`,
		"not a path":        `{"name": 3}`,
	} {
		_, err := Parse(json.RawMessage(raw))
		require.Error(t, err, name)
	}
}

func TestEvalIgnoresNonScalars(t *testing.T) {
	spec, err := Parse(json.RawMessage(`{"name": "tags"}`))
	require.NoError(t, err)
	require.Equal(t, "", Eval(spec.Name, map[string]any{"tags": map[string]any{"a": "b"}}))
	var nilSpec *Spec
	require.Nil(t, nilSpec.RefValues(map[string]any{}))
}

func TestParseDisplayAndRoles(t *testing.T) {
	spec, err := Parse(json.RawMessage(`{
		"name": "name",
		"display": {"code": "SUBN", "label": "Subnetwork", "class": "network"},
		"admin": {"parent": ["in_folder"]},
		"network": {"group": true, "via": ["in_network"], "region": true},
		"port_of": "attached_to",
		"security": {"control": true},
		"inventory": {"collapsed": true},
		"application": {"group": true, "via": ["part_of"]}
	}`))
	require.NoError(t, err)
	require.Equal(t, &Display{Code: "SUBN", Label: "Subnetwork", Class: "network"}, spec.Display)
	require.Equal(t, &Admin{Parent: []string{"in_folder"}}, spec.Roles.Admin)
	require.Equal(t, &NetworkRole{Group: true, Via: []string{"in_network"}, Region: true}, spec.Roles.Network)
	require.Equal(t, "attached_to", spec.Roles.PortOf)
	require.True(t, spec.Roles.Security.Control)
	require.True(t, spec.Roles.Inventory.Collapsed)
	require.Equal(t, []string{"in_folder", "in_network", "attached_to", "part_of"}, spec.Roles.EdgeTypes())

	bare, err := Parse(json.RawMessage(`{"name": "name"}`))
	require.NoError(t, err)
	require.Nil(t, bare.Display)
	require.Equal(t, Roles{}, bare.Roles)
	require.False(t, bare.MemberOnly)

	member, err := Parse(json.RawMessage(`{"member_only": true}`))
	require.NoError(t, err)
	require.True(t, member.MemberOnly)
	_, err = Parse(json.RawMessage(`{"member_only": "yes"}`))
	require.Error(t, err)
}

func TestParseRejectsBadDisplayAndRoles(t *testing.T) {
	for name, raw := range map[string]string{
		"display missing class": `{"display": {"code": "X", "label": "X"}}`,
		"display bad class":     `{"display": {"code": "X", "label": "X", "class": "netwrok"}}`,
		"display long code":     `{"display": {"code": "TOOLONG", "label": "X", "class": "data"}}`,
		"display unknown key":   `{"display": {"code": "X", "label": "X", "class": "data", "colour": "red"}}`,
		"network unknown key":   `{"network": {"grp": true}}`,
		"empty port_of":         `{"port_of": ""}`,
		"empty via":             `{"network": {"via": [""]}}`,
		"admin parent not list": `{"admin": {"parent": "in_folder"}}`,
		"admin owner in schema": `{"admin": {"owner": true}}`,
	} {
		_, err := Parse(json.RawMessage(raw))
		require.Error(t, err, name)
	}
}
