# Phase 12: Fixed-Point Verification (`pudl verify`)

## Summary

Added a `pudl verify` command that re-runs schema inference on all catalog entries and confirms every entry still resolves to the same schema it was originally assigned. This is a correctness invariant inspired by defn's two-pass idempotency check.

## What was built

- `cmd/verify.go` — New top-level `pudl verify` command

## Public API

### CLI

```
pudl verify
```

Iterates all catalog entries, loads each entry's stored data, re-runs schema inference with the same hints (origin, format, collection type), and compares the result against the stored schema.

Output format:
```
Verifying 42 catalog entries...
  my_file.json: OK (pudl/core.#Item)
  other.json: MISMATCH (stored: aws.#EC2Instance, inferred: pudl/core.#Item)

Result: 41 OK, 1 mismatch
```

Exits with error if any mismatches are found.

## Implementation details

- Uses `database.NewCatalogDB()` + `db.QueryEntries()` to iterate all entries
- Uses `config.Load()` to get the schema path
- Uses `inference.NewSchemaInferrer()` + `inferrer.Infer()` to re-infer schemas
- Reads data from `entry.StoredPath`, unmarshals JSON, passes to inference with same hints
- Reports OK/MISMATCH/ERROR per entry, with a summary line at the end
- Returns non-nil error (causing non-zero exit) when mismatches exist
