# Schema review status (`_pudl.review`) — pudl-xf5.8.7

Delivery step 7 of `docs/design/2026-11-01-compliance-policies.md`.

A schema's `_pudl` block now accepts:

```cue
review: {status: "draft" | "validated", by?: string, at?: string}
```

An absent block means `draft`. "Validated" is a review blessing on a type
schema: a person or agent reviewed it as a complete representation of the
resource. The record-level `validation_status` is unchanged.

## Behavior

- **Strict decode.** `review` is decoded with the other strict `_pudl` parts
  (`decodeStrictPudlParts`, next to `sensitive_fields` and `facts`). A
  non-concrete value, a status other than `draft`/`validated`, a missing
  status, a non-string `by`/`at`, or an unknown key sets
  `SchemaMetadata.ReviewError`. The schema is then treated as `draft` (an
  invalid review never counts as validated), and `CheckModuleIntegrity`
  reports `schema X has invalid _pudl.review: ...` as a `SchemaLoadError`. The
  error therefore appears wherever load errors are surfaced (`pudl doctor`,
  import load warnings), and `ValidateModuleIntegrity` rejects it.
- **Display.** `pudl schema list` appends `review=draft` or
  `review=validated by X at Y` to each schema line. `--json` includes
  `metadata.review`. `pudl schema show` prints `Review: ...` under the header
  and adds a `review` object to its JSON. A definition that is not a registered
  schema (a component) shows no review.
- **Generation.** `pudl schema new` (the `schemagen` generator) writes
  `review: status: "draft"` into the generated `_pudl` block, with a comment
  telling the author to set `validated` (with `by`/`at`) after review.

The policy engine's use of review status (required fields of a validated type
count as collected; results against draft types are provisional) is
implemented and tested in `pudl-xf5.8.2` (`internal/policy`).

## Public API (`internal/validator`)

```go
const ReviewDraft = "draft"
const ReviewValidated = "validated"

type SchemaReview struct {
    Status string `json:"status"`
    By     string `json:"by,omitempty"`
    At     string `json:"at,omitempty"`
}
func (r SchemaReview) Describe() string      // "draft", "validated by X at Y"

// New SchemaMetadata fields
Review      SchemaReview `json:"review"`   // draft when absent or invalid
ReviewError string       `json:"-"`
func (m SchemaMetadata) IsValidated() bool
```

## Files

- `internal/validator/review.go` (new): types, `decodeReview`, `parseReview`.
- `internal/validator/pudl_strict.go`, `validation_result.go`,
  `cue_loader.go`: wiring and the integrity report.
- `cmd/schema_list.go`, `cmd/schema_show.go`: display.
- `internal/schemagen/generator.go`: one line for the draft default.

The `schema list` long help was deliberately left unchanged so the generated
CLI reference and help golden do not churn while other branches rewrite the
command tree.

## Tests

- `internal/validator/review_test.go`: table-driven decode (absent, draft,
  validated with by/at, invalid status, missing status, unknown key,
  non-string value, non-concrete value) and `Describe`.
- `cmd/schema_review_test.go`: text and JSON display in `schema list` and
  `schema show`.
- `internal/schemagen/review_test.go`: a generated schema contains the draft
  block and loads as draft without errors.

`CGO_ENABLED=0 go test ./...` passes. `CGO_ENABLED=0 mise exec -- make lint`:
0 issues.

## Line delta (`git diff --numstat`)

- Production: +147 / -5 (net +142).
- Tests: +201.
