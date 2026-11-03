package schemagen

import (
	"encoding/json"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/graphspec"
)

func TestGeneratedGraphBlockIsValid(t *testing.T) {
	g := NewGenerator(t.TempDir())
	data := map[string]any{"GroupId": "sg-1", "GroupName": "web", "AccountId": "123", "Region": "us-east-1"}
	res, err := g.Generate(data, GenerateOptions{PackagePath: "acme/net", DefinitionName: "SecurityGroup"})
	require.NoError(t, err)
	require.Contains(t, res.Content, "pudl graph vocab")

	v := cuecontext.New().CompileString(res.Content)
	require.NoError(t, v.Err(), res.Content)
	var graph cue.Value
	iter, err := v.LookupPath(cue.ParsePath("#SecurityGroup")).Fields(cue.Hidden(true))
	require.NoError(t, err)
	for iter.Next() {
		if iter.Selector().String() == "_pudl" {
			graph = iter.Value().LookupPath(cue.ParsePath("graph"))
		}
	}
	require.True(t, graph.Exists(), res.Content)
	raw, err := graph.MarshalJSON()
	require.NoError(t, err)
	spec, err := graphspec.Parse(json.RawMessage(raw))
	require.NoError(t, err, string(raw))
	require.Equal(t, &graphspec.Display{Code: "SG", Label: "Security group", Class: "security"}, spec.Display)
	require.Equal(t, "AccountId", spec.Account.Path.String())
	require.Equal(t, "Region", spec.Region.Path.String())
	require.Nil(t, spec.Name, "no name-like field: left as a TODO")
	require.True(t, strings.Contains(res.Content, `// name: "..." // TODO`))
}

func TestGraphDisplayGuesses(t *testing.T) {
	for def, want := range map[string][3]string{
		"Instance":         {"INST", "Instance", "compute"},
		"SQLInstance":      {"SQLI", "SQL instance", "data"},
		"PubSubTopic":      {"PST", "Pub sub topic", "messaging"},
		"VeryLongTypeName": {"VLTN", "Very long type name", "compute"},
		"ABCDEFGH":         {"ABCDE", "ABCDEFGH", "compute"},
	} {
		require.Equal(t, want, [3]string{graphCode(def), graphLabel(def), graphClass(def)}, def)
	}
}
