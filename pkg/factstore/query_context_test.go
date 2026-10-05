package factstore_test

import (
	"context"
	"github.com/chazu/pudl/pkg/factstore"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestQueryContextCancelledIsFailure(t *testing.T) {
	s := openStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rows, err := s.QueryContext(ctx, factstore.QueryOptions{Relation: "missing"})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, rows)
	rows, err = s.Query(factstore.QueryOptions{Relation: "missing"})
	require.NoError(t, err)
	require.Empty(t, rows)
}
