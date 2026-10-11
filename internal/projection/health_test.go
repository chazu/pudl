package projection

import (
	"context"
	"fmt"
	"testing"

	"github.com/chazu/pudl/internal/database"
	"github.com/stretchr/testify/require"
)

func TestCheckHealthScopesBrokenSchemasAndSyncFailures(t *testing.T) {
	db, err := database.NewCatalogDB(t.TempDir())
	require.NoError(t, err)
	defer db.Close()
	reg := NewRegistry(hostSource(`{"host":{"args":{"name":"name"}}}`), nil)
	addHostEntry(t, db, "e1", "r1", `{"name":"h"}`)
	report, err := Sync(context.Background(), db, reg, func(database.CatalogEntry) (any, error) { return nil, fmt.Errorf("payload unavailable") }, false)
	require.NoError(t, err)
	diagnostics := CheckDiagnostics(db, reg, nil, "host", report, nil)
	require.NotEmpty(t, diagnostics)
	require.Equal(t, "projection_failed", diagnostics[0].Code)
	diagnostics = CheckDiagnostics(db, reg, nil, "host", SyncReport{}, fmt.Errorf("transaction failed"))
	require.NotEmpty(t, diagnostics)
	require.Equal(t, "projection_sync_failed", diagnostics[0].Code)
	_, err = db.AddFact(database.Fact{Relation: "unrelated", Args: `{"value":1}`})
	require.NoError(t, err)
	require.Empty(t, CheckDiagnostics(db, reg, nil, "unrelated", report, fmt.Errorf("transaction failed")))
	broken := NewRegistry(hostSource(`{"host":{"args":{"a":"a[*]","b":"b[*]"}}}`), nil)
	require.Empty(t, CheckDiagnostics(db, broken, nil, "unrelated", SyncReport{}, nil))
	require.NotEmpty(t, CheckDiagnostics(db, broken, nil, "host", SyncReport{}, nil))
}

func TestOmittedQueryValuesKeepChecksUnknownUntilProjectionChanges(t *testing.T) {
	db, err := database.NewCatalogDB(t.TempDir())
	require.NoError(t, err)
	defer db.Close()
	addHostEntry(t, db, "large", "host", `{"name":"h","amount":18446744073709551616}`)
	reg := NewRegistry(hostSource(`{"host":{"args":{"name":"name","amount":"amount"}}}`), nil)
	report, err := Sync(context.Background(), db, reg, loadJSON, false)
	require.NoError(t, err)
	diagnostics := CheckDiagnostics(db, reg, nil, "host", report, nil)
	require.Len(t, diagnostics, 1)
	require.Equal(t, "projection_incomplete", diagnostics[0].Code)
	reg = NewRegistry(hostSource(`{"host":{"args":{"name":"name"}}}`), nil)
	report, err = Sync(context.Background(), db, reg, loadJSON, false)
	require.NoError(t, err)
	require.Empty(t, CheckDiagnostics(db, reg, nil, "host", report, nil))
}
