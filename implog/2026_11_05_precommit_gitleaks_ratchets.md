# Pre-commit hooks: gitleaks and ratchets (2026-11-05)

## What changed

- `lefthook.yml` — pre-commit hook, two parallel jobs:
  - `gitleaks`: `gitleaks git --pre-commit --staged --redact --no-banner`
    scans only staged changes for secrets.
  - `ratchets`: `ratchets check` checks whole-repo violation counts against
    `ratchet-counts.toml`.
- `.gitleaks.toml` — extends the default gitleaks rules and allowlists two
  known false positives:
  - GCP `keyAlgorithm` enum values (`KEY_ALG_RSA_2048`) in fixtures
    (`packs/gcp/...`, `cmd/gcp_pack_iam_test.go`,
    `internal/secrets/testdata/cloud/gcloud.ndjson`).
  - The fake JWT in `internal/secretscan/secretscan_test.go`.
  `gitleaks dir .` now reports no leaks. `gitleaks git` (full history) still
  reports two SHA checksums in the deleted `.pudl/.examples/gcp.json`; the
  pre-commit hook never sees them.
- `ratchets.toml` — ratchets v2 config for Go, no rules enabled yet.
- `ratchet-counts.toml` — empty budget file.
- `CLAUDE.md` — new "Ratchets" section: every antipattern we want to avoid
  or eliminate gets a ratchet rule.

## Setup per clone

```bash
brew install lefthook gitleaks
cargo install ratchets   # https://github.com/imbue-ai/ratchets
lefthook install
```

## Adding a ratchet

1. Write `ratchets/regex/<id>.toml` (or `ratchets/ast/<id>.toml`) with
   `[rule]` (`id`, `description`, `severity`) and `[match]`
   (`pattern`, `languages`, optional `include`).
2. Add `<id>` to `enabled_ratchets` in `ratchets.toml`.
3. Run `ratchets tighten` to record the current count as the budget.
