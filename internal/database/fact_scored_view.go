package database

import "fmt"

// halfLifeSeconds is the decay half-life applied to fact worth: 90 days. A fact's
// decayed_worth halves for every halfLifeSeconds of age. Chosen to match the
// cass-memory convention; tune here if the policy changes.
const halfLifeSeconds = 90 * 24 * 60 * 60 // 7_776_000

// ensureFactScoredView creates the fact_scored_edb view, which exposes each
// currently-valid fact with a read-time decay score. It joins current_facts (the
// live set) with facts (for valid_start) and computes:
//
//	age_seconds    seconds since the fact became valid (now - valid_start)
//	worth          the fact's worth arg (NULL if absent)
//	decayed_worth  worth * 0.5 ^ (age_seconds / halfLifeSeconds)
//
// Decay is computed at query time, never written back — the underlying facts are
// untouched, so historical/bitemporal queries are unaffected. The view is dropped
// and recreated on every open so its definition always matches this source.
//
// Must run after the facts and current_facts tables exist.
func (c *CatalogDB) ensureFactScoredView() error {
	if _, err := c.db.Exec("DROP VIEW IF EXISTS " + FactScoredView); err != nil {
		return fmt.Errorf("drop view %s: %w", FactScoredView, err)
	}

	createView := fmt.Sprintf(`CREATE VIEW %s AS
		SELECT
			cf.id        AS id,
			cf.relation  AS relation,
			cf.source    AS source,
			(unixepoch() - f.valid_start) AS age_seconds,
			json_extract(cf.args, '$.worth') AS worth,
			json_extract(cf.args, '$.worth') * pow(0.5, (unixepoch() - f.valid_start) / %d.0) AS decayed_worth
		FROM current_facts cf
		JOIN facts f ON f.id = cf.id;`, FactScoredView, halfLifeSeconds)

	if _, err := c.db.Exec(createView); err != nil {
		return fmt.Errorf("create view %s: %w", FactScoredView, err)
	}
	return nil
}
