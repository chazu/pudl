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

// fallbackModel's populate fails as if the asset API were disabled; its
// fallback prints one record. %[1]s is the fallback's partial line, if any.
const fallbackModel = `package systemmodel

#Partial: #SystemModel & {
	name: "partial"
	observation: {scope: "test/partial", complete: true}
	populate: {
		runs: [{argv: ["sh", "-c", "echo 'reason: SERVICE_DISABLED' >&2; exit 1"]}]
		fallback: {
			when: ["SERVICE_DISABLED"]
			%[1]s
			observe: runs: [{argv: ["echo", "{\"_schema\":\"resources.thing\",\"name\":\"a1\",\"value\":\"ok\"}"]}]
		}
	}
	checks: [{name: "latest", query: "failed_anywhere", expect: "empty", severity: "fail", message: "failed"}]
}
`

func setupFallbackModel(t *testing.T, pudlDir, partial string) {
	t.Helper()
	schema := filepath.Join(pudlDir, "schema", "pudl")
	require.NoError(t, os.WriteFile(filepath.Join(schema, "systemmodel", "partial.cue"), []byte(fmt.Sprintf(fallbackModel, partial)), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(schema, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(schema, "rules", "partial.cue"), []byte(fanoutRules), 0o644))
}

// A partial fallback's snapshot is incomplete, so resources it cannot see are
// not marked gone; the report says so.
func TestCheckPartialFallbackLeavesTheSnapshotIncomplete(t *testing.T) {
	h, pudlDir := setupCheckFixture(t)
	setupFallbackModel(t, pudlDir, "partial: true")

	require.NoError(t, h.run("partial"))
	report := fanoutReport(t, pudlDir)
	assert.Equal(t, database.RunStatusSucceeded, report.CompletionStatus)
	pr := report.Populate
	require.NotNil(t, pr)
	require.NotNil(t, pr.Fallback)
	assert.True(t, pr.Fallback.Partial)
	assert.Equal(t, 1, pr.Records)
	assert.Empty(t, pr.FailedInputs)
	assert.False(t, fanoutSnapshot(t, pudlDir, pr.SnapshotID).Complete)
	assert.Contains(t, report.Markdown(), "- partial fallback: it sees only some of these resources")
}

// Without partial the fallback answers for the whole scope, as before.
func TestCheckFallbackKeepsTheSnapshotComplete(t *testing.T) {
	h, pudlDir := setupCheckFixture(t)
	setupFallbackModel(t, pudlDir, "")

	require.NoError(t, h.run("partial"))
	report := fanoutReport(t, pudlDir)
	pr := report.Populate
	require.NotNil(t, pr)
	require.NotNil(t, pr.Fallback)
	assert.False(t, pr.Fallback.Partial)
	assert.True(t, fanoutSnapshot(t, pudlDir, pr.SnapshotID).Complete)
	assert.NotContains(t, report.Markdown(), "partial fallback")
}
