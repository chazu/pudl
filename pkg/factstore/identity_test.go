package factstore_test

import (
	"testing"

	"github.com/chazu/pudl/pkg/factstore"
	"github.com/stretchr/testify/require"
)

func TestFactIdentityPreservesExactNumbers(t *testing.T) {
	s := openStore(t)
	for _, pair := range [][2]string{
		{`{"id":9007199254740992}`, `{"id":9007199254740993}`},
		{`{"id":-9007199254740992}`, `{"id":-9007199254740993}`},
		{`{"nested":[{"id":9223372036854775807}]}`, `{"nested":[{"id":9223372036854775808}]}`},
		{`{"n":0.123456789012345678901}`, `{"n":0.123456789012345678902}`},
	} {
		first, err := s.AddFact(factstore.Fact{Relation: "numbers", Args: pair[0], ValidStart: 1})
		require.NoError(t, err)
		second, err := s.AddFact(factstore.Fact{Relation: "numbers", Args: pair[1], ValidStart: 1})
		require.NoError(t, err)
		require.NotEqual(t, first.ID, second.ID, "distinct numbers must not deduplicate: %v", pair)
	}
	history, err := s.FactHistory("numbers")
	require.NoError(t, err)
	require.Len(t, history, 8)
}

func TestFactIdentityIgnoresEquivalentNumberSpellings(t *testing.T) {
	s := openStore(t)
	for _, pair := range [][2]string{
		{`{"n":1}`, `{"n":1.000e0}`},
		{`{"n":0.000001}`, `{"n":1e-6}`},
		{`{"n":1000000000000000000000}`, `{"n":1e21}`},
		{`{"b":[9007199254740993],"a":true}`, `{"a":true,"b":[9.007199254740993e15]}`},
	} {
		first, err := s.AddFact(factstore.Fact{Relation: "spellings", Args: pair[0], ValidStart: 1})
		require.NoError(t, err)
		second, err := s.AddFact(factstore.Fact{Relation: "spellings", Args: pair[1], ValidStart: 1})
		require.NoError(t, err)
		require.Equal(t, first, second, "equivalent content returns the original stored fact")
	}
}

func TestLegacyFactIDCannotSilentlyDiscardDifferentContent(t *testing.T) {
	dir := t.TempDir()
	s, err := factstore.Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	// Old float64 canonicalization gave this ID to BOTH adjacent integers.
	legacy, err := s.AddFact(factstore.Fact{
		ID:       "8c2a030720b219ac7c96500544054b7068fa77a1329a3b49af8bdd32d88cd7d7",
		Relation: "numbers", Args: `{"id":9007199254740993}`, ValidStart: 1,
	})
	require.NoError(t, err)
	require.NoError(t, s.Close())
	s, err = factstore.Open(dir)
	require.NoError(t, err)
	history, err := s.FactHistory("numbers")
	require.NoError(t, err)
	require.Equal(t, []factstore.Fact{legacy}, history, "opening must preserve existing IDs")
	replayed, err := s.AddFact(legacy)
	require.NoError(t, err)
	require.Equal(t, legacy, replayed, "explicit-ID replay preserves old references")

	_, err = s.AddFact(factstore.Fact{
		Relation: "numbers", Args: `{"id":9007199254740992}`, ValidStart: 1,
	})
	require.ErrorContains(t, err, "conflicts with stored content")
	after, err := s.FactHistory("numbers")
	require.NoError(t, err)
	require.Equal(t, history, after, "a legacy collision must not replace evidence")
}
