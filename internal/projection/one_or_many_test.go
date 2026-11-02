package projection

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// trustRows projects an AWS-style trust document and returns "effect principal" rows.
func trustRows(t *testing.T, oneOrMany bool, doc string) []string {
	t.Helper()
	rels, err := ParseRelations(json.RawMessage(`{"trust": {"each": "Doc.Statement[*]", "one_or_many": ` + map[bool]string{true: "true", false: "false"}[oneOrMany] + `,
		"args": {"effect": "Effect", "principal": "Principal.AWS[*]"}}}`))
	require.NoError(t, err)
	var data any
	require.NoError(t, json.Unmarshal([]byte(doc), &data))
	res := Compute(&SchemaSpec{Schema: "x.#Role", Relations: rels}, "e", "r", data)
	var out []string
	for _, f := range res.Facts {
		var args map[string]any
		require.NoError(t, json.Unmarshal([]byte(f.Args), &args))
		out = append(out, args["effect"].(string)+" "+args["principal"].(string))
	}
	sort.Strings(out)
	return out
}

func TestOneOrManyTreatsSingleValuesAsOneElement(t *testing.T) {
	single := `{"Doc": {"Statement": {"Effect": "Allow", "Principal": {"AWS": "arn:a"}}}}`
	list := `{"Doc": {"Statement": [{"Effect": "Allow", "Principal": {"AWS": ["arn:a", "*"]}}, {"Effect": "Deny", "Principal": {"Service": "s"}}]}}`

	require.Equal(t, []string{"Allow arn:a"}, trustRows(t, true, single), "a lone statement and a lone principal are one element each")
	require.Equal(t, []string{"Allow *", "Allow arn:a"}, trustRows(t, true, list), "arrays expand as before")
	require.Empty(t, trustRows(t, false, single), "without one_or_many a non-array wildcard yields nothing (unchanged behaviour)")
}

func TestOneOrManyChangesTheFingerprintOnlyWhenSet(t *testing.T) {
	plain, err := ParseRelations(json.RawMessage(`{"r": {"each": "a[*]", "args": {"x": "x"}}}`))
	require.NoError(t, err)
	loose, err := ParseRelations(json.RawMessage(`{"r": {"each": "a[*]", "one_or_many": true, "args": {"x": "x"}}}`))
	require.NoError(t, err)
	require.NotEqual(t, fingerprint(&SchemaSpec{Relations: plain}), fingerprint(&SchemaSpec{Relations: loose}),
		"turning one_or_many on re-projects the schema's facts")
}
