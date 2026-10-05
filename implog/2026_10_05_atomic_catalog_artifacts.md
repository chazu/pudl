# Atomic deletion and resource versions

Entry deletion now commits row and membership changes atomically before reclaiming files. Shared items survive cascading collection deletion. Reclamation checks remaining catalog references and reports cleanup failures separately; publishers and reclaimers share a workspace artifact lock acquired before SQLite transactions.

Single-document imports allocate the resource version and perform authoritative content deduplication inside an immediate transaction, and write metadata with that allocated version. Doctor detects legacy duplicate resource/version pairs without renumbering evidence.

Validation: database rollback injection preserves rows, memberships, and raw payload; orphan cleanup preserves referenced files; two public importer instances concurrently allocate distinct versions and their result/catalog/metadata versions agree. Focused database, lister, importer, doctor tests passed.
