# Explicit observation scope and completeness

Migration 25 records observation scope, complete/partial status and declared
schema coverage. Models accept `observation`; checks accept `evidence` selectors
and `max_age`. Snapshot checks project only selected records into a temporary
SQLite view, retaining each source time and snapshot reference in reports.
Incomplete/stale/ambiguous/missing evidence yields unknown. Latest-known facts
remain separate from complete inventories and historical payloads are preserved.
Shared identity extraction now handles nested/quoted paths in inventory matching
without changing persisted resource IDs.

Validation: cmd, database, acute, systemmodel, mubridge and bundle packages passed.
Regressions cover complete disappearance/reappearance, zero records, partial and
failed collection, freshness, owner ambiguity and identity delimiter collisions.
Materialized payloads have a 256 MiB bound; live-cloud qualification is separate.
