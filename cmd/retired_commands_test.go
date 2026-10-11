package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRetiredConvenienceCommandsFailAndFancyRemains(t *testing.T) {
	cliWorkspace(t)
	for _, args := range [][]string{{"setup"}, {"model", "deps", "--derive"}, {"schema", "edit", "x"}, {"schema", "status"}, {"schema", "commit", "-m", "x"}, {"schema", "log"}, {"module", "list"}, {"module", "add", "example@v1"}, {"module", "tidy"}, {"module", "info"}} {
		r := runCLI(t, args...)
		require.Error(t, r.Err, "retired command must fail: %v", args)
	}
	for _, args := range [][]string{{"help", "list", "--json"}, {"completion", "bash"}} {
		r := runCLI(t, args...)
		require.NoError(t, r.Err)
		if args[0] == "help" {
			require.Contains(t, r.Stdout, `"name": "fancy"`)
		}
	}
}
