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
	requireTools(t, "git", "mu", "python3", "bash")
	repo, err := repoRoot()
	require.NoError(t, err)
	doc, err := os.ReadFile(filepath.Join(repo, "docs", "getting-started.md"))
	require.NoError(t, err)
	// Execute the guide's command blocks verbatim in one shell, preserving its
	// variables and cwd across steps. The surrounding prose is not test input.
	blocks := regexp.MustCompile("(?s)<!-- walkthrough:([a-z-]+) -->\\s*```bash\\n(.*?)\\n```").FindAllStringSubmatch(string(doc), -1)
	var stages []string
	for _, block := range blocks {
		stages = append(stages, block[1])
	}
	require.Equal(t, []string{"setup", "baseline", "change", "repeat", "evidence"}, stages)

	base := filepath.Join(repo, ".pudl", "data", "git-walkthrough", "test-runs")
	require.NoError(t, os.MkdirAll(base, 0o755))
	sandbox, err := os.MkdirTemp(base, "acceptance-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(sandbox) })
	source := filepath.Join(sandbox, "source checkout")
	require.NoError(t, os.MkdirAll(source, 0o755))
	require.NoError(t, os.Symlink(pudlBin, filepath.Join(source, "pudl")))
	require.NoError(t, os.CopyFS(filepath.Join(source, "examples", "git-inventory"), os.DirFS(filepath.Join(repo, "examples", "git-inventory"))))
	tmp := filepath.Join(sandbox, "temporary workspaces")
	require.NoError(t, os.MkdirAll(tmp, 0o755))
	var script strings.Builder
	script.WriteString("set -eu\n")
	for _, block := range blocks {
		script.WriteString(block[2] + "\n")
	}
	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", script.String())
	cmd.Dir = source
	cmd.Env = envWith(map[string]string{"HOME": filepath.Join(sandbox, "home"), "TMPDIR": tmp})
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	roots, err := filepath.Glob(filepath.Join(tmp, "pudl-git-inventory.*"))
	require.NoError(t, err)
	require.Len(t, roots, 1)
	root := roots[0]
	cli := func(args ...string) []byte {
		t.Helper()
		command := exec.Command(pudlBin, args...)
		command.Dir, command.Env = root, cmd.Env
		out, err := command.CombinedOutput()
		require.NoError(t, err, "%s", out)
		return out
	}
	var reports []walkthroughReport
	for _, stage := range []string{"baseline", "changed", "repeat"} {
		data, err := os.ReadFile(filepath.Join(root, ".pudl", "data", "walkthrough", stage+".json"))
		require.NoError(t, err)
		var report walkthroughReport
		require.NoError(t, json.Unmarshal(data, &report))
		require.Equal(t, "git-inventory", report.Model)
		require.Equal(t, "observe-only (inventory)", report.Mode)
		require.Equal(t, "succeeded", report.CompletionStatus)
		require.True(t, report.OK, "observation completes even when inventory drifts")
		require.NotEmpty(t, report.RunID)
		require.NotEmpty(t, report.Populate.SnapshotID)
		newRecords := 1
		if stage == "repeat" {
			newRecords = 0 // Identical content is already in the catalog.
		}
		require.Equal(t, newRecords, report.Populate.Records)
		require.True(t, report.Drift.Verified)
		// Replay after every run has completed: later observations must not
		// overwrite the baseline verdict or any run's retained evidence.
		require.JSONEq(t, string(data), string(cli("run", "report", report.RunID, "--json")))
		snapshot := string(cli("show", report.Populate.SnapshotID, "--raw"))
		require.Contains(t, snapshot, `"run_id": "`+report.RunID+`"`)
		require.Contains(t, snapshot, `"snapshot_id": "`+report.Populate.SnapshotID+`"`)
		require.Contains(t, snapshot, `"record_count": 1`)
		var records struct {
			Entries []struct {
				ID, Schema string
				StoredPath string `json:"stored_path"`
			} `json:"entries"`
		}
		require.NoError(t, json.Unmarshal(cli("list", "--origin", "pudl-run", "--collection-id", report.Populate.SnapshotID, "--json"), &records))
		require.Len(t, records.Entries, 1)
		require.Equal(t, "pudl/git.#GitRepository", records.Entries[0].Schema)
		relative, err := filepath.Rel(filepath.Join(root, ".pudl", "data"), records.Entries[0].StoredPath)
		require.NoError(t, err)
		require.True(t, filepath.IsLocal(relative), "raw evidence must stay within the workspace: %s", relative)
		raw := string(cli("show", records.Entries[0].ID, "--raw"))
		require.Contains(t, raw, `"name": "local/demo"`)
		branch := "release"
		if stage == "baseline" {
			branch = "main"
		}
		require.Contains(t, raw, `"default_branch": "`+branch+`"`)
		reports = append(reports, report)
	}
	require.True(t, reports[0].Drift.Clean)
	require.Empty(t, reports[0].Drift.Drifted)
	for i := 1; i < len(reports); i++ {
		require.NotEqual(t, reports[i-1].RunID, reports[i].RunID)
		require.NotEqual(t, reports[i-1].Populate.SnapshotID, reports[i].Populate.SnapshotID)
		require.False(t, reports[i].Drift.Clean)
		require.Len(t, reports[i].Drift.Drifted, 1)
		drift := reports[i].Drift.Drifted[0]
		require.Equal(t, "git.repository/local/demo", drift.Resource)
		require.Equal(t, "changed", drift.Reason)
		require.Equal(t, "default_branch: release → want main", drift.Diff)
	}
	require.Equal(t, reports[1].Drift.Drifted, reports[2].Drift.Drifted)
	require.Contains(t, string(output), "git.repository/local/demo (changed): default_branch: release → want main")
	require.Contains(t, string(output), `"snapshot_id": "`+reports[1].Populate.SnapshotID+`"`)
	require.Contains(t, string(output), `"default_branch": "release"`)
	_, err = os.Stat(filepath.Join(sandbox, "home", ".pudl"))
	require.True(t, os.IsNotExist(err), "walkthrough must not create a global PUDL workspace")
	globalMu, err := os.ReadDir(filepath.Join(sandbox, "home", ".mu"))
	var globalMuNames []string
	for _, entry := range globalMu {
		globalMuNames = append(globalMuNames, entry.Name())
	}
	require.True(t, os.IsNotExist(err), "walkthrough must not create global mu state: %v", globalMuNames)
}

type walkthroughReport struct {
	Model            string `json:"model"`
	RunID            string `json:"run_id"`
	Mode             string `json:"mode"`
	CompletionStatus string `json:"completion_status"`
	OK               bool   `json:"ok"`
	Populate         struct {
		Records    int    `json:"records"`
		SnapshotID string `json:"snapshot_id"`
	} `json:"populate"`
	Drift struct {
		Clean    bool                                      `json:"clean"`
		Verified bool                                      `json:"verified"`
		Drifted  []struct{ Resource, Reason, Diff string } `json:"drifted"`
	} `json:"drift"`
}
