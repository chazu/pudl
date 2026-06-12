package database

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithFactTxCommit(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	var added Fact
	err := db.WithFactTx(func(tx *FactTx) error {
		var err error
		added, err = tx.AddFact(Fact{
			Relation: "observation",
			Args:     `{"kind":"fact","description":"tx commit"}`,
			Source:   "agent-1",
		})
		return err
	})
	require.NoError(t, err)
	require.NotEmpty(t, added.ID)

	facts, err := db.QueryFacts(FactFilter{Relation: "observation"})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, added.ID, facts[0].ID)

	// current_facts stays in sync through the transactional path too.
	current, err := db.QueryCurrentFacts("observation")
	require.NoError(t, err)
	require.Len(t, current, 1)
	assert.Equal(t, added.ID, current[0].ID)
}

func TestWithFactTxRollback(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	boom := fmt.Errorf("legality check failed")
	err := db.WithFactTx(func(tx *FactTx) error {
		if _, err := tx.AddFact(Fact{Relation: "observation", Args: `{"k":"v"}`}); err != nil {
			return err
		}
		return boom
	})
	assert.ErrorIs(t, err, boom)

	facts, err := db.QueryFacts(FactFilter{Relation: "observation"})
	require.NoError(t, err)
	assert.Empty(t, facts, "writes must roll back when fn errors")

	current, err := db.QueryCurrentFacts("observation")
	require.NoError(t, err)
	assert.Empty(t, current, "current_facts writes must roll back too")
}

func TestWithFactTxReadYourWrites(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	err := db.WithFactTx(func(tx *FactTx) error {
		added, err := tx.AddFact(Fact{Relation: "observation", Args: `{"n":1}`})
		if err != nil {
			return err
		}
		facts, err := tx.QueryFacts(FactFilter{Relation: "observation"})
		if err != nil {
			return err
		}
		require.Len(t, facts, 1)
		assert.Equal(t, added.ID, facts[0].ID)

		hist, err := tx.FactHistory("observation")
		if err != nil {
			return err
		}
		assert.Len(t, hist, 1)
		return nil
	})
	require.NoError(t, err)
}

func TestWithFactTxRetractAndInvalidate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	a, err := db.AddFact(Fact{Relation: "observation", Args: `{"n":1}`})
	require.NoError(t, err)
	b, err := db.AddFact(Fact{Relation: "observation", Args: `{"n":2}`})
	require.NoError(t, err)

	err = db.WithFactTx(func(tx *FactTx) error {
		if err := tx.RetractFact(a.ID); err != nil {
			return err
		}
		return tx.InvalidateFact(b.ID)
	})
	require.NoError(t, err)

	facts, err := db.QueryFacts(FactFilter{Relation: "observation"})
	require.NoError(t, err)
	assert.Empty(t, facts)

	hist, err := db.FactHistory("observation")
	require.NoError(t, err)
	require.Len(t, hist, 2)
	for _, f := range hist {
		switch f.ID {
		case a.ID:
			assert.NotNil(t, f.TxEnd, "a should be retracted")
		case b.ID:
			assert.NotNil(t, f.ValidEnd, "b should be invalidated")
		}
	}
}

// TestWithFactTxSerializesCheckAndWrite exercises the TOCTOU scenario the
// transaction exists for: two writers each check an invariant against the
// current facts and write only if it holds. Because the transaction takes the
// write lock at BEGIN, the second writer's check cannot run until the first
// has committed, so exactly one write lands.
func TestWithFactTxSerializesCheckAndWrite(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Each writer asserts "claim" only if no claim exists yet.
	claim := func(owner string) error {
		return db.WithFactTx(func(tx *FactTx) error {
			facts, err := tx.QueryFacts(FactFilter{Relation: "claim"})
			if err != nil {
				return err
			}
			if len(facts) > 0 {
				return nil // someone already holds the claim
			}
			// Widen the race window: without the up-front write lock, both
			// writers would pass the check before either lands its write.
			time.Sleep(150 * time.Millisecond)
			_, err = tx.AddFact(Fact{
				Relation: "claim",
				Args:     fmt.Sprintf(`{"owner":%q}`, owner),
				Source:   owner,
			})
			return err
		})
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, owner := range []string{"agent-a", "agent-b"} {
		wg.Add(1)
		go func(i int, owner string) {
			defer wg.Done()
			errs[i] = claim(owner)
		}(i, owner)
	}
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])

	facts, err := db.QueryFacts(FactFilter{Relation: "claim"})
	require.NoError(t, err)
	assert.Len(t, facts, 1, "check-and-write must serialize: exactly one claim wins")
}
