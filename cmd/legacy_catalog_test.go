package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
)

// legacyCatalog is a catalog shaped as the convergence and mu era left it:
// converge and mutating run-set reports, pending approvals, a mu build
// manifest with a converging action, a drift observation, an envelope
// item_schemas row, and a run row carrying the retired apply columns.
type legacyCatalog struct {
	manifestID, actionID, driftID string
}

const (
	legacyRunID    = "run_legacy-converge"
	legacyRunSetID = "runset_legacy-mutation"
)

func seedLegacyCatalog(t *testing.T, root string) legacyCatalog {
	t.Helper()
	db, err := database.NewCatalogDB(root)
	require.NoError(t, err)
	defer db.Close()
	exec := func(query string, args ...any) {
		t.Helper()
		_, err := db.DB().Exec(query, args...)
		require.NoError(t, err)
	}
	now := time.Now().UTC()
	raw := filepath.Join(root, "data", "raw", "legacy")
	require.NoError(t, os.MkdirAll(raw, 0o755))
	entry := func(id, name, schema, entryType string, payload any, target *string) {
		t.Helper()
		data, err := json.Marshal(payload)
		require.NoError(t, err)
		path := filepath.Join(raw, name)
		require.NoError(t, os.WriteFile(path, data, 0o644))
		kind := entryType
		require.NoError(t, db.AddEntry(database.CatalogEntry{
			ID: id, StoredPath: path, ImportTimestamp: now, Format: "json", Origin: "mu-build",
			Schema: schema, Confidence: 1, RecordCount: 1, SizeBytes: int64(len(data)),
			EntryType: &kind, Target: target,
		}))
	}

	web := "web"
	entry("legacymanifest", "manifest.json", "pudl/mu.#Manifest", "manifest", map[string]any{
		"version": 1, "type": "mu.build.manifest/v1", "timestamp": "2026-10-01T00:00:00Z", "duration_s": 1.5,
		"targets": []any{map[string]any{"name": "//models/web:drift", "toolchain": "k8s"}},
		"actions": []any{map[string]any{"id": "apply-web", "target": "web", "cached": false, "exit_code": 0, "outputs": map[string]any{}}},
		"summary": map[string]any{"completed": 1, "cached": 0, "failed": 0, "cancelled": 0},
	}, nil)
	entry("legacyaction", "web_action.json", "pudl/mu.#ManifestAction", "manifest-action", map[string]any{
		"id": "apply-web", "target": "web", "cached": false, "exit_code": 0, "outputs": map[string]any{},
	}, &web)
	exec(`UPDATE catalog_entries SET status = 'converging', tags = '{"model":"web","exit_code":0,"cached":false}' WHERE id = 'legacyaction'`)
	driftTarget := "//models/web:drift"
	entry("legacydrift", "drift.json", "pudl/mu.#DriftObservation", "drift-observation", map[string]any{
		"observation_id": "drift_20261001", "timestamp": "2026-10-01T00:00:00Z", "origin": "pudl-run",
		"target": driftTarget, "run_id": legacyRunID, "clean": false, "drifted_count": 1,
		"drifted": []any{map[string]any{"resource": "Deployment/web", "reason": "changed"}},
	}, &driftTarget)
	exec(`INSERT INTO item_schemas (item_id, schema_ref, status) VALUES ('legacymanifest', 'mu/k8s@v1', 'unresolved')`)

	require.NoError(t, db.StartRun(legacyRunID, "web", "converge"))
	require.NoError(t, db.FinishRun(legacyRunID, database.RunConclusion{CompletionStatus: database.RunStatusSucceeded, Verdict: "clean"}))
	exec(`UPDATE runs SET outcome = 'clean', needs_verification = 1, applies = 2, scoped = 1 WHERE run_id = ?`, legacyRunID)
	report := []byte(`{"report_version":2,"run_id":"` + legacyRunID + `","model":"web","mode":"converge","completion_status":"succeeded","ok":true,
		"resource_scope":["web"],"approval_status":"approved",
		"converge":{"outcome":"clean","iterations":2,"needs_verification":false,"mutation_receipts":[{"iteration":1,"manifest_sha256":"abc","status":"completed"}]},
		"sealed_bindings":[{"direction":"output","producer_phase":"converge","output":"TOKEN","store_mode":"create"}],
		"checks":[{"name":"pdb","query":"missing_pdb","severity":"fail","count":0,"advisory_count":1,"scope":"latest-known","passed":true}]}`)
	require.NoError(t, db.SaveRunReport(legacyRunID, "web", report))
	setReport := []byte(`{"report_version":2,"run_set_id":"` + legacyRunSetID + `","mode":"converge","status":"succeeded","plan_digest":"d",
		"approval_id":"` + legacyRunSetID + `","approval_status":"approved","edges":[],"ordered_members":["web"],
		"members":[{"model":"web","run_id":"` + legacyRunID + `","result":"succeeded","mutation_required":true}]}`)
	require.NoError(t, db.SaveRunSetReport(legacyRunSetID, setReport))
	exec(`INSERT INTO run_approvals (run_id, model, request_json, status, created_at) VALUES (?, 'web', '{"model":"web","max_iters":5}', 'pending', ?)`, legacyRunID, now)
	exec(`INSERT INTO run_set_approvals (run_set_id, plan_digest, request_json, plan_json, status, created_at) VALUES (?, 'd', '{}', '{"members":[]}', 'approved', ?)`, legacyRunSetID, now)
	return legacyCatalog{manifestID: "legacymanifest", actionID: "legacyaction", driftID: "legacydrift"}
}

// A catalog written before convergence and mu were removed opens, lists,
// shows and reports without errors, and keeps every recorded value visible.
func TestLegacyMutationEraCatalogStaysReadable(t *testing.T) {
	root := cliWorkspace(t)
	legacy := seedLegacyCatalog(t, root)
	ok := func(args ...string) cliResult {
		t.Helper()
		result := runCLI(t, args...)
		require.NoError(t, result.Err, "pudl %s: %s", strings.Join(args, " "), result.Stderr)
		return result
	}

	listed := ok("list", "--json").Stdout
	for _, id := range []string{legacy.manifestID, legacy.actionID, legacy.driftID} {
		require.Contains(t, listed, id)
	}
	for _, id := range []string{legacy.manifestID, legacy.actionID, legacy.driftID} {
		ok("show", id)
		ok("show", id, "--json")
	}
	shown := ok("show", legacy.actionID, "--json").Stdout
	require.NotContains(t, shown, `"converging"`, "a legacy converging status reads as unknown")

	var statuses []map[string]any
	require.NoError(t, json.Unmarshal([]byte(ok("status", "--json").Stdout), &statuses))
	found := false
	for _, status := range statuses {
		if status["target"] == "web" {
			found = true
			require.Equal(t, "unknown", status["status"])
		}
	}
	require.True(t, found, "status lists the legacy target")

	human := ok("report", legacyRunID).Stdout
	for _, text := range []string{"# model: web", "## legacy fields", "### converge", `"mutation_receipts"`, "### approval_status", "### sealed_bindings", "### resource_scope"} {
		require.Contains(t, human, text)
	}
	machine := ok("report", legacyRunID, "--json").Stdout
	require.Contains(t, machine, `"converge"`, "JSON reports keep every stored field")
	require.Contains(t, machine, `"advisory_count"`)

	setHuman := ok("report", legacyRunSetID).Stdout
	for _, text := range []string{"check " + legacyRunSetID, "### approval_id", "### members[0]", `"mutation_required": true`} {
		require.Contains(t, setHuman, text)
	}
	require.Contains(t, ok("report", legacyRunSetID, "--json").Stdout, `"approval_status"`)

	// The legacy pudl/mu schemas still validate their stored entries.
	doctor := runCLI(t, "doctor", "--json")
	for _, id := range []string{legacy.manifestID, legacy.actionID, legacy.driftID} {
		require.NotContains(t, doctor.Stdout, `"id": "`+id+`", "status": "invalid"`)
	}
	var doctorReport struct {
		Catalog []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"catalog"`
	}
	require.NoError(t, json.Unmarshal([]byte(doctor.Stdout), &doctorReport), doctor.Stdout)
	checked := map[string]bool{}
	for _, entry := range doctorReport.Catalog {
		checked[entry.ID] = true
	}
	for _, id := range []string{legacy.manifestID, legacy.actionID, legacy.driftID} {
		require.True(t, checked[id], "doctor checks legacy entry %s", id)
	}
	for _, entry := range doctorReport.Catalog {
		require.NotEqual(t, "invalid", entry.Status, "legacy entry %s must still validate", entry.ID)
	}
}
