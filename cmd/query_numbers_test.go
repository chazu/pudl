package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/workspace"
	"github.com/stretchr/testify/require"
)

func TestQueryCommandPreservesNumericConstraintsAndJSON(t *testing.T) {
	previousContext := queryCmd.Context()
	queryCmd.SetContext(context.Background())
	t.Cleanup(func() { queryCmd.SetContext(previousContext) })
	root := t.TempDir()
	dir := filepath.Join(root, ".pudl")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "workspace.cue"), []byte(`name: "query-numbers"`), 0o644))
	policy, err := workspace.Resolve(root, filepath.Join(t.TempDir(), "global"))
	require.NoError(t, err)
	previousPolicy, previousJSON := wsPolicy, jsonOutput
	previousRule, previousValid, previousTx := queryRuleFile, queryAsOfValid, queryAsOfTx
	previousList, previousTopo, previousAll := queryList, queryTopo, queryAllWorkspace
	wsPolicy, jsonOutput = policy, true
	queryRuleFile, queryAsOfValid, queryAsOfTx = "", "", ""
	queryList, queryTopo, queryAllWorkspace = false, false, false
	t.Cleanup(func() {
		wsPolicy, jsonOutput = previousPolicy, previousJSON
		queryRuleFile, queryAsOfValid, queryAsOfTx = previousRule, previousValid, previousTx
		queryList, queryTopo, queryAllWorkspace = previousList, previousTopo, previousAll
	})
	db, err := database.NewCatalogDB(dir)
	require.NoError(t, err)
	for _, raw := range []string{`{"n":9007199254740992}`, `{"n":9007199254740993}`, `{"n":"9007199254740993"}`} {
		_, err = db.AddFact(database.Fact{Relation: "numbers", Args: raw, ValidStart: 1, TxStart: 1})
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())
	for _, historical := range []bool{false, true} {
		if historical {
			queryAsOfValid, queryAsOfTx = "1", "1"
		}
		for _, constraint := range []string{"n=9007199254740993", "n=9.007199254740993e15", `n="9007199254740993"`} {
			output := captureQueryOutput(t, func() error { return queryCmd.RunE(queryCmd, []string{"numbers", constraint}) })
			var rows []struct {
				Args map[string]json.RawMessage `json:"args"`
			}
			require.NoError(t, json.Unmarshal([]byte(output), &rows))
			require.Len(t, rows, 1)
			want := "9007199254740993"
			if strings.Contains(constraint, `"`) {
				want = `"9007199254740993"`
			}
			require.Equal(t, want, string(rows[0].Args["n"]))
		}
	}
	err = queryCmd.RunE(queryCmd, []string{"numbers", "n=9223372036854775808"})
	require.ErrorContains(t, err, "unsupported query number")
}

func captureQueryOutput(t *testing.T, run func() error) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "stdout")
	require.NoError(t, err)
	defer file.Close()
	previous := os.Stdout
	os.Stdout = file
	defer func() { os.Stdout = previous }()
	require.NoError(t, run())
	data, err := os.ReadFile(file.Name())
	require.NoError(t, err)
	return string(data)
}
