package database

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCatalogCancellationRollsBackWithFreshContext(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	err := db.WithCatalogTxContext(ctx, func(tx *CatalogTx) error {
		require.NoError(t, tx.AddEntry(txTestEntry("cancelled", "web")))
		cancel()
		return ctx.Err()
	})
	require.ErrorIs(t, err, context.Canceled)
	_, err = db.GetEntry("cancelled")
	require.Error(t, err)
	require.NoError(t, db.WithCatalogTx(func(tx *CatalogTx) error { return tx.AddEntry(txTestEntry("after", "web")) }))
}
func TestQueryFactsContextDoesNotReturnSuccessAfterCancellation(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := db.QueryFactsContext(ctx, FactFilter{Relation: "missing"})
	require.ErrorIs(t, err, context.Canceled)
}

func TestCatalogWriterWaitIsCancellable(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	held, err := db.db.Conn(context.Background())
	require.NoError(t, err)
	defer held.Close()
	_, err = held.ExecContext(context.Background(), "BEGIN IMMEDIATE")
	require.NoError(t, err)
	defer held.ExecContext(context.Background(), "ROLLBACK")
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = db.WithCatalogTxContext(ctx, func(*CatalogTx) error { t.Fatal("must not enter callback"); return nil })
	require.Error(t, err)
	require.Less(t, time.Since(start), time.Second)
}
