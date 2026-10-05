package importer

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/chazu/pudl/internal/idgen"
	"github.com/stretchr/testify/require"
)

func TestImportedCollectionPayloadVerificationUsesCanonicalExactJSON(t *testing.T) {
	imp, source, _ := collectionFixture(t, "exact.ndjson", "{\"z\":2,\"name\":\"first\",\"serial\":9007199254740993}\n{\"name\":\"second\",\"serial\":9007199254740994}\n")
	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)
	items, err := imp.CatalogDB().GetCollectionItems(result.ID)
	require.NoError(t, err)
	require.Len(t, items, 2)
	raw, err := os.ReadFile(items[0].StoredPath)
	require.NoError(t, err)
	require.NotEqual(t, *items[0].ContentHash, idgen.ComputeContentID(raw), "pretty stored bytes differ from canonical hash")
	value, err := idgen.DecodeJSONExact(raw)
	require.NoError(t, err)
	canonical, err := json.Marshal(value)
	require.NoError(t, err)
	require.Equal(t, *items[0].ContentHash, idgen.ComputeContentID(canonical))
	require.Contains(t, string(canonical), "9007199254740993")
	issues, err := imp.CatalogDB().VerifyPayloads(context.Background())
	require.NoError(t, err)
	require.Empty(t, issues)
	_, err = imp.CatalogDB().DB().Exec("UPDATE catalog_entries SET content_hash=? WHERE id=?", strings.ToUpper(*items[0].ContentHash), items[0].ID)
	require.NoError(t, err)
	issues, err = imp.CatalogDB().VerifyPayloads(context.Background())
	require.NoError(t, err)
	require.Empty(t, issues, "SHA256 hex letter case does not change the digest")

	require.NoError(t, os.WriteFile(items[0].StoredPath, []byte(`{"name":"first","serial":9007199254740999,"z":2}`), 0600))
	issues, err = imp.CatalogDB().VerifyPayloads(context.Background())
	require.NoError(t, err)
	require.Len(t, issues, 1)
	require.Equal(t, items[0].ID, issues[0].EntryID)
	require.Contains(t, issues[0].Error, "hash mismatch")
}
