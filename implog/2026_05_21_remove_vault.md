# Remove Vault Subsystem

## Summary

Removed the vault secret storage subsystem from pudl. Secrets management
will be handled by mu instead — pudl as a data catalog has no need to
track sensitive information.

## Changes

- Deleted `internal/vault/` package (Vault interface, EnvVault, FileVault, factory, tests)
- Deleted `cmd/vault*.go` (5 files: vault, vault_get, vault_set, vault_list, vault_rotate)
- Removed `completeVaultPaths()` from `cmd/completion.go`
- Removed `VaultBackend` config field and validation from `internal/config/config.go`
- Dropped `filippo.io/age` dependency from `go.mod`

## Public API Removed

- `pudl vault get <path>`
- `pudl vault set <path>`
- `pudl vault list`
- `pudl vault rotate-key`
- Config key `vault_backend`
