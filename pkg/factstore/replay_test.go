package factstore_test

import (
	"testing"

	"github.com/chazu/pudl/pkg/factstore"
	"github.com/stretchr/testify/require"
)

func TestFactReplayPreservesStoredLifecycle(t *testing.T) {
	for _, terminal := range []string{"retracted", "invalidated"} {
		for _, transactional := range []bool{false, true} {
			name := terminal
			if transactional {
				name += "/transaction"
			}
			t.Run(name, func(t *testing.T) {
				s := openStore(t)
				original, err := s.AddFact(factstore.Fact{
					Relation: "replay", Args: `{"value":"original"}`,
					ValidStart: 1, TxStart: 1, Source: "scanner",
				})
				require.NoError(t, err)
				if terminal == "retracted" {
					require.NoError(t, s.RetractFact(original.ID))
				} else {
					require.NoError(t, s.InvalidateFact(original.ID))
				}
				history, err := s.FactHistory("replay")
				require.NoError(t, err)
				// Invalidation adds a superseding version; history[0] is the
				// original recorded version either way.
				versions := 1
				if terminal == "invalidated" {
					versions = 2
				}
				require.Len(t, history, versions)

				var replayed factstore.Fact
				if transactional {
					err = s.Transact(func(tx *factstore.Tx) error {
						var err error
						replayed, err = tx.AddFact(original)
						return err
					})
				} else {
					replayed, err = s.AddFact(original)
				}
				require.NoError(t, err)

				current, err := s.Query(factstore.QueryOptions{Relation: "replay"})
				require.NoError(t, err)
				require.Empty(t, current, "replay must not revive terminal evidence")
				facts, err := s.QueryFacts(factstore.FactFilter{Relation: "replay"})
				require.NoError(t, err)
				require.Empty(t, facts)
				require.Equal(t, history[0], replayed, "AddFact returns the stored lifecycle on replay")
				after, err := s.FactHistory("replay")
				require.NoError(t, err)
				require.Equal(t, history, after)

				past := int64(1)
				then, err := s.Query(factstore.QueryOptions{Relation: "replay", ValidAt: &past, TxAt: &past})
				require.NoError(t, err)
				require.Len(t, then, 1, "terminal evidence remains in historical queries")
			})
		}
	}
}
