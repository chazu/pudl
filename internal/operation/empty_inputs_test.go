package operation

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
)

// sparseModels enumerates three projects and fetches each with fetch.sh,
// which prints <project>.json or, when that is missing, prints <project>.err
// to stderr and exits 1, as gcloud does for a project with the API disabled.
// %[3]s is the populate's empty_when line, if any. sparse-audit is a
// checks-only model that depends on sparse.
const sparseModels = `package systemmodel

#Sparse: #SystemModel & {
	name: "sparse"
	observation: {scope: "test/sparse", complete: true}
	populate: {
		each: argv: ["cat", %[1]q]
		runs: [{argv: ["sh", %[2]q, "{{item}}"], set: project: "{{item}}"}]
		%[3]s
	}
	checks: [{name: "latest", query: "failed_anywhere", expect: "empty", severity: "fail", message: "failed"}]
}

#SparseAudit: #SystemModel & {
	name: "sparse-audit"
	depends_on: ["sparse"]
	checks: [{name: "audit", query: "failed_anywhere", expect: "empty", severity: "fail", message: "failed"}]
}
`

const serviceDisabledStderr = `ERROR: (gcloud.compute.firewall-rules.list) Compute Engine API has not been used in project proj-b before or it is disabled.
- '@type': type.googleapis.com/google.rpc.ErrorInfo
  reason: SERVICE_DISABLED
`

const permissionDeniedStderr = `ERROR: (gcloud.compute.firewall-rules.list) Required 'compute.firewalls.list' permission for 'projects/proj-b'
  reason: IAM_PERMISSION_DENIED
`

// setupSparse writes the sparse models with the given empty_when line, and
// fixtures where p2 fails with p2Stderr.
func setupSparse(t *testing.T, pudlDir, emptyWhen, p2Stderr string) string {
	t.Helper()
	fixtures := t.TempDir()
	files := map[string]string{
		"projects.json": `["p1", "p2", "p3"]`,
		"fetch.sh":      "d=$(dirname \"$0\")\nif [ -f \"$d/$1.json\" ]; then cat \"$d/$1.json\"; else cat \"$d/$1.err\" >&2; exit 1; fi\n",
		"p1.json":       `[{"_schema":"resources.thing","name":"a1","value":"ok"}]`,
		"p2.err":        p2Stderr,
		"p3.json":       `[{"_schema":"resources.thing","name":"c1","value":"ok"}]`,
	}
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(fixtures, name), []byte(content), 0o644))
	}
	model := fmt.Sprintf(sparseModels, filepath.Join(fixtures, "projects.json"), filepath.Join(fixtures, "fetch.sh"), emptyWhen)
	schema := filepath.Join(pudlDir, "schema", "pudl")
	require.NoError(t, os.WriteFile(filepath.Join(schema, "systemmodel", "sparse.cue"), []byte(model), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(schema, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(schema, "rules", "sparse.cue"), []byte(fanoutRules), 0o644))
	return fixtures
}

const emptyWhenServiceDisabled = `empty_when: ["SERVICE_DISABLED"]`

// A run whose stderr matches empty_when is an empty input: the snapshot is
// complete with the other inputs' records, nothing failed, the checks pass,
// and the report lists it with the matched line.
func TestCheckEmptyWhenMatchKeepsTheSnapshotComplete(t *testing.T) {
	h, pudlDir := setupCheckFixture(t)
	setupSparse(t, pudlDir, emptyWhenServiceDisabled, serviceDisabledStderr)

	require.NoError(t, h.run("sparse"))
	report := fanoutReport(t, pudlDir)
	assert.Equal(t, database.RunStatusSucceeded, report.CompletionStatus)
	pr := report.Populate
	require.NotNil(t, pr)
	assert.Equal(t, 2, pr.Records)
	assert.Equal(t, 3, pr.Runs)
	assert.Empty(t, pr.FailedInputs)
	require.Len(t, pr.EmptyInputs, 1)
	empty := pr.EmptyInputs[0]
	assert.Equal(t, "p2", empty.Item)
	assert.Equal(t, 1, empty.Run)
	assert.Equal(t, 1, empty.ExitStatus)
	assert.Equal(t, "SERVICE_DISABLED", empty.Pattern)
	assert.Equal(t, "reason: SERVICE_DISABLED", empty.Reason)
	for _, c := range report.Checks {
		assert.Equal(t, "pass", c.Outcome, c.Name)
	}
	snapshot := fanoutSnapshot(t, pudlDir, pr.SnapshotID)
	assert.True(t, snapshot.Complete, "an empty input is a definite answer")
	assert.Equal(t, 2, snapshot.RecordCount)

	text := report.Markdown()
	assert.Contains(t, text, "- empty inputs: 1 of 3 runs (matched empty_when; observed with no records)")
	assert.Contains(t, text, `  - p2: exit 1: matched "SERVICE_DISABLED"`)
	assert.Contains(t, text, "    stderr: reason: SERVICE_DISABLED")
	assert.NotContains(t, text, "failed inputs")
}

// Without empty_when the same failure is a failed input, as before.
func TestCheckWithoutEmptyWhenTheFailureIsAFailedInput(t *testing.T) {
	h, pudlDir := setupCheckFixture(t)
	setupSparse(t, pudlDir, "", serviceDisabledStderr)

	err := h.run("sparse")
	require.ErrorIs(t, err, errFailedInputs)
	pr := fanoutReport(t, pudlDir).Populate
	require.NotNil(t, pr)
	assert.Empty(t, pr.EmptyInputs)
	require.Len(t, pr.FailedInputs, 1)
	assert.False(t, fanoutSnapshot(t, pudlDir, pr.SnapshotID).Complete)
}

// A failure no pattern matches stays a failed input with empty_when set.
func TestCheckEmptyWhenDoesNotSwallowUnmatchedFailures(t *testing.T) {
	h, pudlDir := setupCheckFixture(t)
	setupSparse(t, pudlDir, emptyWhenServiceDisabled, permissionDeniedStderr)

	err := h.run("sparse")
	require.ErrorIs(t, err, errFailedInputs)
	pr := fanoutReport(t, pudlDir).Populate
	require.NotNil(t, pr)
	assert.Empty(t, pr.EmptyInputs)
	require.Len(t, pr.FailedInputs, 1)
	assert.Equal(t, "p2", pr.FailedInputs[0].Item)
	assert.False(t, fanoutSnapshot(t, pudlDir, pr.SnapshotID).Complete)
}

// A dependent is not blocked when its producer's only non-successes are
// empty inputs: the producer succeeded.
func TestCheckEmptyInputsDoNotBlockDependents(t *testing.T) {
	h, pudlDir := setupCheckFixture(t)
	setupSparse(t, pudlDir, emptyWhenServiceDisabled, serviceDisabledStderr)

	require.NoError(t, h.run("sparse-audit", "sparse"))
	report := latestCheckReportForTest(t, pudlDir)
	assert.Equal(t, database.RunStatusSucceeded, report.Status)
	results := map[string]string{}
	for _, member := range report.Members {
		results[member.Model] = member.Result
	}
	assert.Equal(t, database.RunStatusSucceeded, results["sparse"])
	assert.Equal(t, database.RunStatusSucceeded, results["sparse-audit"])
}
