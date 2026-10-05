package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addPastFact records a fact believed and valid since t, so as-of queries can
// address moments before an invalidation without sleeping.
func addPastFact(t *testing.T, db *CatalogDB, t0 int64) Fact {
	t.Helper()
	f, err := db.AddFact(Fact{Relation: "host", Args: `{"name":"h1"}`, ValidStart: t0, TxStart: t0, Source: "test"})
	require.NoError(t, err)
	return f
}

func queryHosts(t *testing.T, db *CatalogDB, filter FactFilter) []Fact {
	t.Helper()
	filter.Relation = "host"
	facts, err := db.QueryFacts(filter)
	require.NoError(t, err)
	return facts
}

func TestInvalidation_DoesNotRewriteEarlierBelief(t *testing.T) {
	// Reproduction: add a fact, invalidate it later, then ask what we believed
	// before the invalidation about a later moment. At that point nothing said
	// the fact would end, so the answer is the fact.
	db, cleanup := setupTestDB(t)
	defer cleanup()

	f := addPastFact(t, db, 100)
	require.NoError(t, db.InvalidateFact(f.ID))
	now := time.Now().Unix()

	later := now + 1000
	before := int64(200)
	believed := queryHosts(t, db, FactFilter{ValidAt: &later, TxAt: &before})
	require.Len(t, believed, 1)
	assert.Equal(t, f.ID, believed[0].ID)
	assert.Nil(t, believed[0].ValidEnd, "the earlier belief had no valid_end")

	// With current knowledge the fact is no longer true then.
	assert.Empty(t, queryHosts(t, db, FactFilter{ValidAt: &later, TxAt: &later}))
	assert.Empty(t, queryHosts(t, db, FactFilter{ValidAt: &later}))
	assert.Empty(t, queryHosts(t, db, FactFilter{}))
}

func TestInvalidation_TxAtBeforeAndAfter(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	f := addPastFact(t, db, 100)
	started := time.Now().Unix()
	require.NoError(t, db.InvalidateFact(f.ID))
	now := time.Now().Unix()
	validDuring := int64(150)

	before := int64(200)
	old := queryHosts(t, db, FactFilter{ValidAt: &validDuring, TxAt: &before})
	require.Len(t, old, 1)
	assert.Equal(t, f.ID, old[0].ID)

	after := now + 1
	current := queryHosts(t, db, FactFilter{ValidAt: &validDuring, TxAt: &after})
	require.Len(t, current, 1, "exactly one version is believed after the invalidation")
	assert.Equal(t, f.ID, current[0].Supersedes)
	require.NotNil(t, current[0].ValidEnd)
	assert.GreaterOrEqual(t, *current[0].ValidEnd, started)
	assert.LessOrEqual(t, *current[0].ValidEnd, now)

	// Transaction-time-only queries see whichever version was believed then.
	assert.Equal(t, f.ID, queryHosts(t, db, FactFilter{TxAt: &before})[0].ID)
	assert.Equal(t, current[0].ID, queryHosts(t, db, FactFilter{TxAt: &after})[0].ID)
}

func TestSameSecondAddAndRetract_VisibleBySequence(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	f, err := db.AddFact(Fact{Relation: "host", Args: `{"name":"blip"}`})
	require.NoError(t, err)
	require.NoError(t, db.RetractFact(f.ID))
	stored, err := db.GetFact(f.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.TxEndSeq)
	require.Greater(t, *stored.TxEndSeq, stored.TxSeq)

	// Whole-second state after the second it lived in: already retracted.
	if *stored.TxEnd == stored.TxStart {
		at := stored.TxStart
		assert.Empty(t, queryHosts(t, db, FactFilter{TxAt: &at}))
	}

	// After the add and before the retraction, by sequence, it was believed.
	seq := stored.TxSeq
	during := queryHosts(t, db, FactFilter{TxSeqAt: &seq})
	require.Len(t, during, 1)
	assert.Equal(t, f.ID, during[0].ID)

	assert.Empty(t, queryHosts(t, db, FactFilter{TxSeqAt: stored.TxEndSeq}))

	history, err := db.FactHistory("host")
	require.NoError(t, err)
	require.Len(t, history, 1, "the audit trail keeps the zero-length belief")
}

func TestFactFilter_RejectsTxAtWithTxSeqAt(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	at, seq := int64(1), int64(1)
	_, err := db.QueryFacts(FactFilter{Relation: "host", TxAt: &at, TxSeqAt: &seq})
	assert.Error(t, err)
}

func TestSupersededIDsResolveThroughChain(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	f := addPastFact(t, db, 100)
	require.NoError(t, db.InvalidateFact(f.ID))

	latest, err := db.LatestFactVersion(f.ID)
	require.NoError(t, err)
	assert.Equal(t, SupersededFactID(f.ID, *latest.ValidEnd), latest.ID)

	self, err := db.LatestFactVersion(latest.ID)
	require.NoError(t, err)
	assert.Equal(t, latest.ID, self.ID)

	for _, id := range []string{f.ID, latest.ID} {
		versions, err := db.FactVersions(id)
		require.NoError(t, err)
		require.Len(t, versions, 2)
		assert.Equal(t, f.ID, versions[0].ID)
		assert.Equal(t, latest.ID, versions[1].ID)
	}

	// Writes addressed to the old ID act on the newest version.
	err = db.InvalidateFact(f.ID)
	assert.Error(t, err, "the newest version is already invalidated")
	require.NoError(t, db.RetractFact(f.ID))
	retracted, err := db.GetFact(latest.ID)
	require.NoError(t, err)
	assert.NotNil(t, retracted.TxEnd)

	validDuring := int64(150)
	assert.Empty(t, queryHosts(t, db, FactFilter{ValidAt: &validDuring}),
		"after retraction nothing is believed about the fact")
}

func TestInvalidatingRetractedFactFails(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	f := addPastFact(t, db, 100)
	require.NoError(t, db.RetractFact(f.ID))
	assert.Error(t, db.InvalidateFact(f.ID))

	history, err := db.FactHistory("host")
	require.NoError(t, err)
	assert.Len(t, history, 1)
}

func TestFactSequencesIncreaseWithEveryWrite(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	a, err := db.AddFact(Fact{Relation: "host", Args: `{"name":"a"}`})
	require.NoError(t, err)
	b, err := db.AddFact(Fact{Relation: "host", Args: `{"name":"b"}`})
	require.NoError(t, err)
	assert.Greater(t, b.TxSeq, a.TxSeq)

	// Caller-supplied sequence and supersession are ignored.
	c, err := db.AddFact(Fact{Relation: "host", Args: `{"name":"c"}`, TxSeq: 1, Supersedes: a.ID})
	require.NoError(t, err)
	assert.Greater(t, c.TxSeq, b.TxSeq)
	assert.Empty(t, c.Supersedes)
}

func TestFactVersioningMigrationBackfillsLegacyRows(t *testing.T) {
	dir := t.TempDir()
	db, err := NewCatalogDB(dir)
	require.NoError(t, err)

	early, err := db.AddFact(Fact{Relation: "host", Args: `{"name":"early"}`, ValidStart: 100, TxStart: 100})
	require.NoError(t, err)
	late, err := db.AddFact(Fact{Relation: "host", Args: `{"name":"late"}`, ValidStart: 300, TxStart: 300})
	require.NoError(t, err)
	closed, err := db.AddFact(Fact{Relation: "host", Args: `{"name":"closed"}`, ValidStart: 100, TxStart: 100})
	require.NoError(t, err)

	// Return the catalog to its pre-migration shape: no sequence columns, no
	// counter, migration 19 unrecorded. The closed row was retracted at 200.
	for _, stmt := range []string{
		`UPDATE facts SET tx_end = 200 WHERE id = '` + closed.ID + `'`,
		`DROP INDEX idx_facts_supersedes`,
		`DROP INDEX idx_facts_tx_seq`,
		`ALTER TABLE facts DROP COLUMN supersedes`,
		`ALTER TABLE facts DROP COLUMN tx_seq`,
		`ALTER TABLE facts DROP COLUMN tx_end_seq`,
		`DROP TABLE fact_sequence`,
		`DELETE FROM schema_migrations WHERE version = 19`,
	} {
		_, err := db.db.Exec(stmt)
		require.NoError(t, err, stmt)
	}
	require.NoError(t, db.Close())

	reopened, err := NewCatalogDB(dir)
	require.NoError(t, err)
	defer reopened.Close()

	get := func(id string) *Fact {
		f, err := reopened.GetFact(id)
		require.NoError(t, err)
		return f
	}
	e, c, l := get(early.ID), get(closed.ID), get(late.ID)
	assert.Less(t, e.TxSeq, l.TxSeq)
	require.NotNil(t, c.TxEndSeq)
	assert.Less(t, c.TxSeq, *c.TxEndSeq)
	assert.Less(t, *c.TxEndSeq, l.TxSeq, "the closing at 200 precedes the add at 300")

	fresh, err := reopened.AddFact(Fact{Relation: "host", Args: `{"name":"fresh"}`})
	require.NoError(t, err)
	assert.Greater(t, fresh.TxSeq, l.TxSeq, "the counter resumes after backfilled sequences")
}
