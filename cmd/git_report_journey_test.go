package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chazu/pudl/internal/database"
	"github.com/stretchr/testify/require"
)

// This journey uses the installed example and literal public commands throughout.
// The saved observations need neither a mu executable nor a live repository service.
func TestGitInventoryReportJourney(t *testing.T) {
	root := cliWorkspace(t)
	successful := func(args ...string) cliResult {
		t.Helper()
		result := runCLI(t, args...)
		require.NoError(t, result.Err, "pudl %s: %s", strings.Join(args, " "), result.Stderr)
		return result
	}
	decodeReport := func(output string) RunReport {
		t.Helper()
		require.True(t, json.Valid([]byte(strings.TrimSpace(output))), "report stdout must be JSON: %s", output)
		var report RunReport
		decoder := json.NewDecoder(strings.NewReader(output))
		decoder.UseNumber()
		require.NoError(t, decoder.Decode(&report))
		return report
	}
	successful("example", "install", "git-inventory", "--json")
	ingest := func(fixture, origin string) string {
		t.Helper()
		result := successful("mu", "ingest-observe", "--path", filepath.Join(root, "populators/git-inventory", fixture+"-observe.json"), "--origin", origin, "--json")
		var recorded struct {
			SnapshotID string `json:"snapshot_id"`
			Records    int    `json:"records"`
		}
		require.NoError(t, json.Unmarshal([]byte(result.Stdout), &recorded))
		require.NotEmpty(t, recorded.SnapshotID)
		require.Equal(t, 1, recorded.Records)
		return recorded.SnapshotID
	}
	baselineID := ingest("baseline", "git-baseline")
	baseline := decodeReport(successful("run", "git-inventory", "--from-catalog", "--catalog-scope", baselineID, "--json").Stdout)
	require.NotEmpty(t, baseline.RunID)
	require.True(t, baseline.OK)
	require.Equal(t, database.RunStatusSucceeded, baseline.CompletionStatus)
	require.NotNil(t, baseline.Drift)
	require.True(t, baseline.Drift.Clean)
	require.False(t, baseline.Drift.Verified)
	require.Equal(t, baselineID, baseline.Drift.SnapshotID)
	changedID := ingest("changed", "git-changed")
	changed := decodeReport(successful("run", "git-inventory", "--from-catalog", "--catalog-scope", changedID, "--json").Stdout)
	require.True(t, changed.OK, "drift is an observation; a fail-severity assertion is a distinct failure")
	require.False(t, changed.Drift.Clean)
	require.False(t, changed.Drift.Verified)
	require.Equal(t, changedID, changed.Drift.SnapshotID)
	require.Len(t, changed.Drift.Drifted, 1)
	finding := changed.Drift.Drifted[0]
	require.Equal(t, "git.repository/local/demo", finding.Resource)
	require.Equal(t, "changed", finding.Reason)
	require.Len(t, finding.Fields, 1)
	require.Equal(t, "default_branch", finding.Fields[0].Path)
	require.Equal(t, "main", finding.Fields[0].Expected)
	require.Equal(t, "release", finding.Fields[0].Observed)
	require.NotNil(t, finding.Fields[0].Previous)
	require.Equal(t, "no-baseline", finding.Fields[0].Previous.Status, "standalone imports have no model-owned successful prior observation")

	// The operator authors a check against the exact changed inventory evidence.
	// Project only the resource and evidence identifiers needed to investigate it.
	modelPath := filepath.Join(root, "schema/models/git_inventory.cue")
	model, err := os.ReadFile(modelPath)
	require.NoError(t, err)
	model = append(model, []byte(`
#GitInventory: checks: [{
 name: "review-changed-inventory"
 query: "git_changed_inventory"
 expect: "empty"
 severity: "fail"
 message: "the changed repository inventory requires review"
}]
`)...)
	require.NoError(t, os.WriteFile(modelPath, model, 0o644))
	rulesDir := filepath.Join(root, "schema/models/rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "git_review.cue"), []byte(`package rules

git_changed_inventory: {
 head: {rel: "git_changed_inventory", args: {
  resource: "git.repository/local/demo"
  origin: "git-changed"
  entry_id: "$E"
  snapshot_id: "$S"
 }}
 body: [{rel: "catalog_entry", args: {
  id: "$E"
  collection_id: "$S"
  origin: "git-changed"
  schema: "pudl/git.#GitRepository"
  collection_type: "item"
 }}]
}
`), 0o644))
	successful("model", "validate", "git-inventory", "--json")
	failedResult := runCLI(t, "run", "git-inventory", "--from-catalog", "--catalog-scope", changedID, "--json")
	require.ErrorIs(t, failedResult.Err, errFailSeverityChecks)
	failed := decodeReport(failedResult.Stdout)
	require.False(t, failed.OK)
	require.Equal(t, database.RunStatusFailed, failed.CompletionStatus)
	require.Len(t, failed.Drift.Drifted, 1)
	require.Len(t, failed.Checks, 1)
	check := failed.Checks[0]
	require.Equal(t, "empty", check.Expect)
	require.Equal(t, "fail", check.Severity)
	require.False(t, check.Passed)
	require.Equal(t, 1, check.Count)
	require.Zero(t, check.AdvisoryCount)
	require.Equal(t, checkScopeGlobal, check.Scope)
	require.Len(t, check.Witnesses, 1)
	require.False(t, check.WitnessesTruncated)
	witness := check.Witnesses[0]
	require.Equal(t, "git.repository/local/demo", witness.Resource)
	require.False(t, witness.Advisory)
	require.Equal(t, changedID, witness.Arguments["snapshot_id"])
	entryID, ok := witness.Arguments["entry_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, entryID)
	require.Contains(t, witness.EvidenceIDs, entryID)
	require.Contains(t, witness.EvidenceIDs, changedID)

	humanResult := runCLI(t, "run", "git-inventory", "--from-catalog", "--catalog-scope", changedID)
	require.ErrorIs(t, humanResult.Err, errFailSeverityChecks)
	for _, text := range []string{"## findings", "### drift", "### checks", `expected "main", observed "release", previous (no-baseline)`, "review-changed-inventory", entryID, changedID} {
		require.Contains(t, humanResult.Stdout, text)
	}

	// Stored findings survive later model edits/runs, and the current evidence
	// availability annotation names the snapshot that this report retains.
	storedResult := successful("run", "report", failed.RunID, "--json")
	stored := decodeReport(storedResult.Stdout)
	require.Equal(t, failed.Checks, stored.Checks)
	require.Equal(t, failed.Drift, stored.Drift)
	var evidence struct {
		Availability []database.ReportEvidence `json:"evidence_availability"`
	}
	require.NoError(t, json.Unmarshal([]byte(storedResult.Stdout), &evidence))
	require.Equal(t, []database.ReportEvidence{{SnapshotID: changedID, Status: "available"}}, evidence.Availability)
	baselineStored := decodeReport(successful("run", "report", baseline.RunID, "--json").Stdout)
	require.True(t, baselineStored.Drift.Clean)
	require.Empty(t, baselineStored.Checks)
	var shown struct {
		Pins    []database.SnapshotPin `json:"pins"`
		Records int                    `json:"records"`
	}
	require.NoError(t, json.Unmarshal([]byte(successful("snapshot", "show", changedID, "--json").Stdout), &shown))
	require.Equal(t, 1, shown.Records)
	ownerFound := false
	for _, pin := range shown.Pins {
		if pin.OwnerKind == database.SnapshotPinReport && pin.OwnerID == failed.RunID {
			ownerFound = true
			require.NotNil(t, pin.ExpiresAt)
		}
	}
	require.True(t, ownerFound, "snapshot inspection names this failed report as an independent retention owner")
	var snapshots []database.ObserveSnapshot
	require.NoError(t, json.Unmarshal([]byte(successful("snapshot", "list", "--json").Stdout), &snapshots))
	var retained bool
	for _, snapshot := range snapshots {
		if snapshot.SnapshotID == changedID {
			retained = snapshot.Retained
		}
	}
	require.True(t, retained, "persisted changed/failure reports retain their cited evidence")
}
