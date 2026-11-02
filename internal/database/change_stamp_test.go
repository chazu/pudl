package database

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChangeStampMovesOnWrites(t *testing.T) {
	dir, err := os.MkdirTemp("", "stamp")
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	db, err := NewCatalogDB(dir)
	require.NoError(t, err)
	defer db.Close()

	s1, err := db.ChangeStamp()
	require.NoError(t, err)
	s2, err := db.ChangeStamp()
	require.NoError(t, err)
	require.Equal(t, s1, s2, "reading does not move the stamp")

	_, err = db.AddFact(Fact{Relation: "host", Args: `{"name":"h1"}`, Source: "test"})
	require.NoError(t, err)
	s3, err := db.ChangeStamp()
	require.NoError(t, err)
	require.NotEqual(t, s1, s3, "a fact write moves the stamp")
}
