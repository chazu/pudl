# Vault Guide

The vault manages secrets used by definitions and methods. Vault references in definitions are resolved at execution time — resolved values never hit disk or artifacts.

## Vault References

In definition files, reference secrets with `vault://` syntax:

```cue
prod_db: examples.#DatabaseModel & {
    schema: {
        host:     "db.example.com"
        password: "vault://prod/db/password"
    }
}
```

When `pudl method run` executes against this definition, the executor walks the args map and substitutes `vault://` references with actual secret values before passing them to the method.

## Backends

### Environment (default)

Reads secrets from environment variables. The vault path is converted to an environment variable name by uppercasing and replacing `/` with `_`:

- `vault://aws/access_key` reads `AWS_ACCESS_KEY`
- `vault://prod/db/password` reads `PROD_DB_PASSWORD`

This backend is CI-friendly and requires no additional setup.

### File

Stores secrets in an age-encrypted JSON file at `~/.pudl/vaults/default.age`. Requires a passphrase for encryption/decryption.

## Configuration

Set the backend in `config.yaml`:

```yaml
vault_backend: env    # or "file"
```

Or configure via CLI:

```bash
pudl config set vault_backend file
```

## CLI Commands

### Store a secret

```bash
pudl vault set aws/access_key "AKIA..."
pudl vault set prod/db/password "s3cret"
```

Note: `set` is only available with the file backend. For the env backend, set environment variables directly.

### Retrieve a secret

```bash
pudl vault get aws/access_key
```

### List stored paths

```bash
pudl vault list
```

### Rotate encryption key

Re-encrypt the file vault with a new passphrase:

```bash
pudl vault rotate-key
```

## Security

- Resolved vault values are passed to methods in-memory only
- Vault values are never written to artifacts or metadata files
- The file backend uses age encryption for at-rest protection
- Environment variables should be managed through your CI/CD system or secret manager
