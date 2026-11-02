package datalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCatalogEntryRejectsUnknownArguments(t *testing.T) {
	rules, err := ParseRulesFromSource(`
ok: {head: {rel: "c", args: {id: "$I"}}, body: [{rel: "catalog_entry", args: {id: "$I", schema: "x"}}]}
bad: {head: {rel: "c", args: {id: "$I"}}, body: [{rel: "catalog_entry", args: {id: "$I", A: ""}}]}
`)
	require.NoError(t, err)
	byName := map[string]Rule{}
	for _, r := range rules {
		byName[r.Name] = r
	}
	require.NoError(t, byName["ok"].LoadErr)
	require.ErrorContains(t, byName["bad"].LoadErr, `no argument "A"`)
}
