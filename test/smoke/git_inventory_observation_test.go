//go:build smoke

package smoke

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSmoke_GitInventoryObservation(t *testing.T) {
	root, sandbox, env := exampleWorkspace(t, "git", "mu", "python3")
	cli := func(args ...string) []byte {
		t.Helper()
		command := exec.Command(pudlBin, args...)
		command.Dir, command.Env = root, env
		out, err := command.CombinedOutput()
		require.NoError(t, err, "%s", out)
		return out
	}
	cli("init")
	cli("example", "install", "git-inventory")
	var saved [][]byte
	for _, stage := range []string{"baseline", "changed", "repeat"} {
		if stage == "changed" {
			data, err := os.ReadFile(filepath.Join(root, ".pudl/populators/git-inventory/changed.json"))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(root, ".pudl/populators/git-inventory/current.json"), data, 0o644))
		}
		saved = append(saved, cli("run", "git-inventory", "--mu-root", ".pudl/data/mu", "--json"))
	}
	var reports []walkthroughReport
	for index, stage := range []string{"baseline", "changed", "repeat"} {
		data := saved[index]
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
		var replay map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(cli("run", "report", report.RunID, "--json"), &replay))
		var availability []struct {
			SnapshotID string `json:"snapshot_id"`
			Status     string `json:"status"`
		}
		require.NoError(t, json.Unmarshal(replay["evidence_availability"], &availability))
		require.Contains(t, availability, struct {
			SnapshotID string `json:"snapshot_id"`
			Status     string `json:"status"`
		}{report.Populate.SnapshotID, "available"})
		delete(replay, "evidence_availability")
		frozen, err := json.Marshal(replay)
		require.NoError(t, err)
		require.JSONEq(t, string(data), string(frozen))
		if stage != "baseline" {
			var detail struct {
				Drift struct {
					Drifted []struct {
						Fields []struct {
							Previous struct {
								Status     string          `json:"status"`
								SnapshotID string          `json:"snapshot_id"`
								Value      json.RawMessage `json:"value"`
							} `json:"previous"`
						} `json:"fields"`
					} `json:"drifted"`
				} `json:"drift"`
			}
			require.NoError(t, json.Unmarshal(data, &detail))
			require.Len(t, detail.Drift.Drifted, 1)
			require.Len(t, detail.Drift.Drifted[0].Fields, 1)
			previous := detail.Drift.Drifted[0].Fields[0].Previous
			require.Equal(t, "available", previous.Status)
			require.Equal(t, reports[len(reports)-1].Populate.SnapshotID, previous.SnapshotID)
			want := `"main"`
			if stage == "repeat" {
				want = `"release"`
			}
			require.Equal(t, want, string(previous.Value))
		}
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
	_, err := os.Stat(filepath.Join(sandbox, "home", ".pudl"))
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
