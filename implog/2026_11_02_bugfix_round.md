# Bugs from the docs rewrite fixed

Fixes the five bugs filed while running the examples in the 2026-11-02 docs
rewrite. Each has its own delivery log:

| Bead | Fix | Log |
| --- | --- | --- |
| `pudl-f7z` (P1) | Doctor no longer validates a collection's stored array against the built-in `#Collection` metadata schema, so JSON array imports no longer fail doctor and `doctor --fix`. Doctor no longer prints usage on failure. | [doctor and search](2026_11_02_doctor_collections_and_fts_search.md) |
| `pudl-jfs` (P3) | `facts search` quotes punctuated terms (`prod-nat`) as FTS5 phrases; the documented syntax passes through. | [doctor and search](2026_11_02_doctor_collections_and_fts_search.md) |
| `pudl-dx4` (P2) | Observe ingest deduplicates by entry ID, so a byte-identical record from a second collector is reused and added to that model's snapshot instead of failing on the UNIQUE constraint. The now-unused `GetLatestObserveByContentHash` is removed. | [duplicate records](2026_11_02_dup_record_ingest.md) |
| `pudl-8wr` (P3) | Inference gives +0.3 to a schema whose top-level field pinned to one string (e.g. `kind: "compute#firewall"`) matches the record, so pack types outrank generic `infra.*` schemas. Raw gcloud output still lacks `project`, so hand imports need `--set project=`; the GCP pack README says so. | [GCP pack inference](2026_11_02_gcp_pack_inference.md) |
| `pudl-ltq` (P3) | `schema add` no longer warns about `_identity`/`_tracked`/`_version`; `_pudl` is already decoded strictly at load. `schema new` no longer writes `cascade_fallback`. | [legacy metadata](2026_11_02_schema_legacy_metadata.md) |

## Public API

No exported API added. Removed: `CatalogDB.GetLatestObserveByContentHash`,
the `CatalogTx`/`CatalogWriter` method of that name, and the
`schema.ValidationResult` fields `HasMetadata` and `MetadataInfo` (with the
`MetadataInfo` type). Added the internal field
`validator.SchemaMetadata.Discriminators` (tagged `json:"-"`).

## Validation (merged branch `bugfix-round`)

- `CGO_ENABLED=0 mise exec -- go test -count=1 ./...`: pass.
- `make test-race`, `make test-git-walkthrough` (smoke and system): pass.
- `CGO_ENABLED=0 mise exec -- make lint`: 0 issues.
- `make check-docs check-skills`: pass.
- `make vulncheck`: 0 vulnerabilities affecting pudl code.
- By hand with a fresh binary: the GCP pack firewall fixture imported with
  `--set project=` classifies as `gcp.#Firewall`, `facts search allow-ssh`
  returns a match, and `pudl doctor` exits 0 after importing a JSON array.
