# Phase 5: Vault System — Credential Management

**Date:** 2026-03-07

## Summary

Implemented a vault system with two backends (environment variables, encrypted file) for secure credential management. Vault references (`vault://path`) in definition socket bindings are resolved at method execution time. Resolved values never hit disk or artifacts.

## Public API

### Vault Interface (`internal/vault/`)

```go
type Vault interface {
    Get(path string) (string, error)
    Set(path, value string) error
    List() ([]string, error)
    Close() error
}
```

### Backends

- **EnvVault** — Maps vault paths to env vars: `my/api/key` → `PUDL_VAULT_MY_API_KEY`
- **FileVault** — Age-encrypted JSON in `~/.pudl/vaults/default.age`, passphrase from `PUDL_VAULT_PASSPHRASE`

### Factory

```go
vault.New(backend string, pudlDir string) (Vault, error)
```

### Config

- `vault_backend` config key: `"env"` (default) or `"file"`

### Executor Integration

- `resolveArgs()` walks all string args; values starting with `vault://` are resolved via the vault before method execution
- Vault is optional — nil vault skips resolution

### CLI Commands

- `pudl vault get <path>` — Retrieve a secret
- `pudl vault set <path> <value>` — Store a secret
- `pudl vault list` — List stored secret paths
- `pudl vault rotate-key` — Re-encrypt file vault with new passphrase (requires `PUDL_VAULT_NEW_PASSPHRASE`)

## Files Created

| File | Lines | Purpose |
|------|-------|---------|
| `internal/vault/vault.go` | ~65 | Vault interface + EnvVault backend |
| `internal/vault/file_vault.go` | ~130 | Age-encrypted file vault backend |
| `internal/vault/factory.go` | ~15 | Backend factory |
| `internal/vault/vault_test.go` | ~60 | EnvVault tests |
| `internal/vault/file_vault_test.go` | ~95 | FileVault tests (round-trip, rotate) |
| `cmd/vault.go` | ~20 | Parent `pudl vault` command |
| `cmd/vault_get.go` | ~45 | Get secret CLI |
| `cmd/vault_set.go` | ~45 | Set secret CLI |
| `cmd/vault_list.go` | ~45 | List secrets CLI |
| `cmd/vault_rotate.go` | ~50 | Rotate key CLI |

## Files Modified

| File | Change |
|------|--------|
| `internal/config/config.go` | Added `VaultBackend` field, config key, validation |
| `internal/executor/executor.go` | Added `vault` field to Executor, updated `New()` signature |
| `internal/executor/args.go` | Added `vault://` resolution in `resolveArgs()` |
| `internal/executor/executor_test.go` | Updated `New()` call with nil vault |
| `cmd/method_run.go` | Create vault, pass to executor |
| `cmd/method_list.go` | Updated `New()` call with nil vault |
| `go.mod` / `go.sum` | Added `filippo.io/age` dependency |

## Tests

10 new tests, all passing:
- `TestPathToEnvVar` — env var name generation
- `TestEnvVaultGet/Set/List/Close` — env backend operations
- `TestFileVaultRoundTrip` — create, set, get, list, save, reload from disk
- `TestFileVaultMissingSecret` — error on nonexistent key
- `TestFileVaultNoPassphrase` — error when env var missing
- `TestFileVaultRotateKey` — re-encrypt with new passphrase, verify old fails
- `TestFileVaultClose` — clean shutdown
