# Strict explicit schemas

Explicit `--schema` requests now reject unavailable or mismatched schemas before
publishing a file's records. Collection validation is atomic per file; a
multi-file command reports separate successes and failures. Preview and dedup
re-import use the same strict contract. `--allow-schema-fallback` explicitly
retains exploratory fallback and reports requested schema, policy and mismatch
count. Command observers use the same contract across their entire batch and
can opt into `allow_schema_fallback`; mismatches downgrade completeness.
Sensitive data remains redacted during permissive fallback.

Validation: importer, mubridge and cmd package tests passed, including strict
JSON/NDJSON/array/YAML, atomic failure, preview parity, dedup and sensitive
command-observation regressions. Generated help and reference were refreshed.
