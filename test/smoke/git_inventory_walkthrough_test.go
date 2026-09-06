//go:build smoke

package smoke

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSmoke_GitInventoryWalkthrough(t *testing.T) {
	root, sandbox, env := exampleWorkspace(t, "git")
	repo, err := repoRoot()
	require.NoError(t, err)
	doc, err := os.ReadFile(filepath.Join(repo, "docs/getting-started.md"))
	require.NoError(t, err)
	blocks := regexp.MustCompile("(?s)<!-- walkthrough:([a-z-]+) -->\\s*```bash\\n(.*?)\\n```").FindAllStringSubmatch(string(doc), -1)
	var stages []string
	for _, block := range blocks {
		stages = append(stages, block[1])
	}
	require.Equal(t, []string{"repository", "setup", "baseline", "change", "repeat", "evidence"}, stages)
	cli := func(args ...string) []byte {
		t.Helper()
		command := exec.Command(filepath.Join(sandbox, "installed tools", args[0]), args[1:]...)
		command.Dir, command.Env = root, env
		out, err := command.CombinedOutput()
		require.NoError(t, err, "%v: %s", args, out)
		return out
	}
	var reports []walkthroughReport
	var saved [][]byte
	for _, block := range blocks {
		for _, line := range strings.Split(block[2], "\n") {
			args := strings.Fields(line)
			if len(args) == 0 {
				continue
			}
			if block[1] == "repository" {
				if args[0] == "cd" {
					require.Equal(t, []string{"cd", "pudl-tutorial"}, args)
					root = filepath.Join(root, args[1])
					continue
				}
				require.Equal(t, []string{"git", "init", "pudl-tutorial"}, args)
			} else {
				require.Equal(t, "pudl", args[0], "tutorial steps must be PUDL commands: %s", line)
				require.NotContains(t, line, "$")
				require.NotContains(t, line, "|")
				require.NotContains(t, line, ">")
			}
			if len(args) > 2 && args[1] == "show" && args[2] == "RECORD_ID" {
				var entries struct {
					Entries []struct {
						ID, Schema string
						StoredPath string `json:"stored_path"`
					}
				}
				require.NoError(t, json.Unmarshal(cli("pudl", "list", "--origin", "git-changed", "--items-only", "--json"), &entries))
				require.Len(t, entries.Entries, 1)
				require.Equal(t, "pudl/git.#GitRepository", entries.Entries[0].Schema)
				relative, err := filepath.Rel(filepath.Join(root, ".pudl/data"), entries.Entries[0].StoredPath)
				require.NoError(t, err)
				require.True(t, filepath.IsLocal(relative), "evidence must stay inside the workspace")
				args[2] = entries.Entries[0].ID
			}
			if args[len(args)-1] == "BASELINE_RUN_ID" {
				args[len(args)-1] = reports[0].RunID
			}
			output := cli(args...)
			if len(args) > 2 && args[1] == "show" {
				require.Contains(t, string(output), `"default_branch": "release"`)
				require.Contains(t, string(output), "pudl/git.#GitRepository")
			}
			if len(args) > 2 && args[1] == "run" && args[2] == "git-inventory" {
				data := cli("pudl", "run", "report", "--json")
				var report walkthroughReport
				require.NoError(t, json.Unmarshal(data, &report))
				reports = append(reports, report)
				saved = append(saved, data)
				if block[1] != "baseline" {
					require.Contains(t, string(output), "default_branch: release → want main")
				}
			}
		}
	}
	require.Len(t, reports, 3)
	require.True(t, reports[0].Drift.Clean)
	for i, report := range reports {
		require.False(t, report.Drift.Verified, "imported evidence is not a live observation")
		require.Empty(t, report.Populate.SnapshotID)
		require.Equal(t, "succeeded", report.CompletionStatus)
		require.JSONEq(t, string(saved[i]), string(cli("pudl", "run", "report", report.RunID, "--json")))
		if i > 0 {
			require.NotEqual(t, reports[i-1].RunID, report.RunID)
			require.False(t, report.Drift.Clean)
			require.Len(t, report.Drift.Drifted, 1)
			require.Equal(t, "git.repository/local/demo", report.Drift.Drifted[0].Resource)
			require.Equal(t, "default_branch: release → want main", report.Drift.Drifted[0].Diff)
		}
	}
	for _, global := range []string{".pudl", ".mu"} {
		require.NoDirExists(t, filepath.Join(sandbox, "home", global))
	}
}
