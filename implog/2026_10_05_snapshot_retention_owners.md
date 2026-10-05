# Snapshot evidence retention ownership

Manual pins, each pending exact approval, and each retained report now own independent snapshot pins. Releasing one owner leaves the others intact. Migration conservatively preserves legacy retained booleans as manual pins and reconstructs named pending approval owners from persisted member reports.

Reports retain referenced populate, binding, drift, and historical snapshot evidence for 30 days after their last save. References survive expiration and pruning; RunReportEvidence distinguishes available from pruned payloads while persisted report findings remain unchanged. Snapshot show exposes owner and expiration. Legacy reports receive a window based on their original saved timestamp.

Prune selects victims inside its deletion transaction, protects the latest successful eligible observation for each model/workspace even if newer observations failed, and holds the artifact lock through reference-aware postcommit cleanup. Minimal snapshot tombstones preserve prior-history identity and original order for historical comparisons.

Validation: independent manual/two-approval pins, expired report evidence and immutable findings, newer failed observation preserving last success, legacy manual migration, and existing snapshot/approval/report suites passed.

Follow-up audit: explicit catalog deletion now refuses an active pinned snapshot, an item referenced by pinned evidence, and the latest eligible successful baseline. This prevents manual deletion from bypassing approval/report retention guarantees. Read-only legacy catalogs fall back to their retained boolean without applying migrations. Additional deletion-protection and legacy-read regression tests passed; focused race tests passed for versions, deletion, retention, and pruning.
