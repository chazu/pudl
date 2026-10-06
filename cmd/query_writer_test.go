package cmd

import (
	"bytes"
	"errors"
	"testing"

	"github.com/chazu/pudl/internal/database"
	"github.com/stretchr/testify/require"
)

type queryFailWriter struct {
	err        error
	beforeFail int
}

func (w *queryFailWriter) Write(p []byte) (int, error) {
	if w.beforeFail > 0 {
		w.beforeFail--
		return len(p), nil
	}
	return 0, w.err
}

func TestQueryReturnsOutputFailuresInEveryMode(t *testing.T) {
	root := cliWorkspace(t)
	db, err := database.NewCatalogDB(root)
	require.NoError(t, err)
	for _, fact := range []database.Fact{
		{Relation: "writer_numbers", Args: `{"n":9007199254740993}`, ValidStart: 1, TxStart: 1},
		{Relation: "writer_edges", Args: `{"from":"app","to":"db"}`, ValidStart: 1, TxStart: 1},
	} {
		_, err := db.AddFact(fact)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())
	for _, tc := range []struct {
		name       string
		args       []string
		beforeFail int
	}{
		{"json", []string{"query", "writer_numbers", "--json"}, 0},
		{"tuples", []string{"query", "writer_numbers"}, 0},
		{"count", []string{"query", "writer_numbers"}, 1},
		{"empty", []string{"query", "writer_numbers", "n=0"}, 0},
		{"list", []string{"query", "--list"}, 0},
		{"topology", []string{"query", "writer_edges", "--topo"}, 0},
		{"empty topology", []string{"query", "writer_edges", "from=missing", "--topo"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetCommandFlags(rootCmd)
			failure := errors.New("query result sink failed")
			rootCmd.SetOut(&queryFailWriter{err: failure, beforeFail: tc.beforeFail})
			var diagnostics bytes.Buffer
			rootCmd.SetErr(&diagnostics)
			rootCmd.SetArgs(tc.args)
			defer func() {
				rootCmd.SetOut(nil)
				rootCmd.SetErr(nil)
				rootCmd.SetArgs(nil)
				resetCommandFlags(rootCmd)
			}()
			require.ErrorIs(t, rootCmd.Execute(), failure)
		})
	}
}
