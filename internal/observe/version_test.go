package observe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
)

// identityInferrer loads one schema, provider.custom, identified by name.
func identityInferrer(t *testing.T) *inference.SchemaInferrer {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cue.mod"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cue.mod", "module.cue"), []byte("module: \"test.schemas\"\nlanguage: version: \"v0.14.0\"\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schemas"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas", "custom.cue"), []byte(`package schemas

#Thing: {
	_pudl: {
		schema_type: "base"
		resource_type: "provider.custom"
		identity_fields: ["name"]
	}
	name: string
	value: string
	...
}
`), 0o644))
	inferrer, err := inference.NewSchemaInferrer(dir)
	require.NoError(t, err)
	return inferrer
}

func thingObservation(values ...string) string {
	var records []string
	for _, v := range values {
		records = append(records, fmt.Sprintf(`{"_schema":"provider.custom","name":"thing-1","value":%q}`, v))
	}
	return `[{"target":"//producer","current":{"records":[` + strings.Join(records, ",") + `]}}]`
}

func versionsOf(t *testing.T, db *database.CatalogDB) map[string]int {
	t.Helper()
	entry, err := db.GetLatestObserve("producer")
	require.NoError(t, err)
	require.NotNil(t, entry)
	history, err := db.FindByResourceID(*entry.ResourceID)
	require.NoError(t, err)
	versions := map[string]int{}
	for _, e := range history {
		require.NotNil(t, e.Version)
		data, err := os.ReadFile(e.StoredPath)
		require.NoError(t, err)
		versions[string(data)] = *e.Version
	}
	return versions
}

// A changed record of a known resource is its next version, not a second
// version 1 (pudl-42w: a model instance whose definition changed was recorded
// twice as version 1).
func TestIngestObserve_ChangedRecordIsNextVersion(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()
	inferrer := identityInferrer(t)
	ingest := func(snapshotID, input string) {
		_, err := IngestObserve(db, ObserveIngest{
			Reader: strings.NewReader(input), DataDir: dataDir, SnapshotID: snapshotID,
			Graph: inferrer.GetInheritanceGraph(), Inferrer: inferrer,
		})
		require.NoError(t, err)
	}

	ingest("snap_1", thingObservation("a"))
	ingest("snap_2", thingObservation("b"))
	// Unchanged content deduplicates: no new version.
	ingest("snap_3", thingObservation("b"))
	// Two states of one resource in one observation take consecutive versions.
	ingest("snap_4", thingObservation("c", "d"))

	versions := versionsOf(t, db)
	require.Len(t, versions, 4)
	got := []int{}
	for _, v := range []string{"a", "b", "c", "d"} {
		for content, version := range versions {
			if strings.Contains(content, fmt.Sprintf(`"value":%q`, v)) {
				got = append(got, version)
			}
		}
	}
	assert.Equal(t, []int{1, 2, 3, 4}, got)
}

// An unchanged record first stored without an identity, re-observed once its
// schema identifies it, joins the identified resource as that resource's next
// version, not with the version it held alone (pudl-xr6: it kept version 1
// beside the resource's own version 1).
func TestIngestObserve_IdentifiedRecordIsNextVersionOfItsResource(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()
	identified := identityInferrer(t)
	ingest := func(snapshotID, input string, inferrer *inference.SchemaInferrer) {
		in := ObserveIngest{Reader: strings.NewReader(input), DataDir: dataDir, SnapshotID: snapshotID}
		if inferrer != nil {
			in.Graph, in.Inferrer = inferrer.GetInheritanceGraph(), inferrer
		}
		_, err := IngestObserve(db, in)
		require.NoError(t, err)
	}

	ingest("snap_1", thingObservation("a"), identified)
	// No schemas loaded: "b" is stored without an identity, alone at version 1.
	ingest("snap_2", thingObservation("b"), nil)
	// Re-observed with its schema: "b" gains thing-1's identity.
	ingest("snap_3", thingObservation("b"), identified)

	versions := versionsOf(t, db)
	require.Len(t, versions, 2)
	got := map[string]int{}
	for content, version := range versions {
		for _, v := range []string{"a", "b"} {
			if strings.Contains(content, fmt.Sprintf(`"value":%q`, v)) {
				got[v] = version
			}
		}
	}
	assert.Equal(t, map[string]int{"a": 1, "b": 2}, got)
}
