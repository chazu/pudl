package observe

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/secrets"
)

func TestIngestObserveWarnsOfLikelySecretsWithoutValues(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()
	inferrer, err := inference.NewSchemaInferrer(sensitiveSchemaDir(t))
	require.NoError(t, err)
	token := "ghp_" + strings.Repeat("aB3dE5fG7hJ9", 3)
	input := `[{"target":"//hosts/a","current":{"records":[{"host":"a"},{"host":"b","auth":{"token":"` + token + `"}}]}}]`
	result, err := IngestObserve(db, ObserveIngest{
		Reader: strings.NewReader(input), DataDir: dataDir,
		Graph: inferrer.GetInheritanceGraph(), Inferrer: inferrer,
	})
	require.NoError(t, err)
	require.Equal(t, 1, result.SecretWarningsTotal)
	w := result.SecretWarnings[0]
	assert.Equal(t, 1, w.Record)
	assert.Equal(t, "auth.token", w.Path)
	assert.Equal(t, secrets.GitHubToken, w.Kind)
	assert.NotEmpty(t, w.Schema)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), token[4:20], "the warning never carries the value")

	entries, err := db.SnapshotRecordEntries(result.SnapshotID)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	stored, err := os.ReadFile(entries[1].StoredPath)
	require.NoError(t, err)
	assert.Contains(t, string(stored), token, "warnings never change stored data")
}
