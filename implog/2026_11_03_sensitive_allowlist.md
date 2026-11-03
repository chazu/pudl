# Redaction allowlists in `sensitive_fields` (pudl-q4v.1)

**Date:** 2026-11-03

## Work done

`_pudl.sensitive_fields` can now keep a few safe values in an array and
redact the rest. The first use is `gcp.#ComputeInstance`: instance metadata
values were all redacted; now the keys in `_gcpSafeMetadataKeys` (OS Login,
serial port, project SSH keys, logging and similar settings) keep their
values, and every other value, including keys nobody has reviewed, is still
redacted.

The allowlist is a field-path form, so every consumer that already works on
parsed paths handles it unchanged: import and observe redaction, the
retroactive scrub, doctor's secret checks, secret-scan coverage and
projection fingerprints.

## Public API

**CUE** (`_pudl.sensitive_fields` entries may now be structs):

```cue
sensitive_fields: [{
	each:  "metadata.items[*]"   // must end in [*]
	value: "value"               // optional; omit to redact the element
	keep_when: {field: "key", in: [...string]}
}]
```

Unknown keys, a missing `each`, `keep_when`, `keep_when.field` or
`keep_when.in`, and entries that are neither strings nor structs are errors.
Redaction fails closed on them, as on any broken declaration.

**Field paths** (`internal/fieldpath`): a filtered wildcard
`[*!field=["a","b"]]` fans out like `[*]` but skips objects whose `field`
holds a listed string. Elements without the field, with a non-string value
there, or that are not objects are not skipped. `[*!` always starts a filter;
a malformed filter is a parse error. Value order and duplicates do not affect
equality. A filtered wildcard never equals a plain one, so secret-scan
coverage does not extend to the values an allowlist keeps.

- `fieldpath.ExceptWhere(each, field string, keep []string, value string) (Path, error)`

**Validator:** `decodeSensitiveFields` turns each struct into the equivalent
path string, so `SchemaMetadata.SensitiveFields` stays `[]string`.

**GCP pack:** `_gcpSafeMetadataKeys: [...string] | *[...]` in package `gcp`
(`examples/gcp/schema/gcp/compute.cue`), overridable from another file of
the package.

## Files

- `internal/fieldpath/filter.go` (new), `fieldpath.go` (grammar, parse,
  lookup, replace)
- `internal/validator/sensitive_fields.go` (new), `pudl_strict.go`
- `examples/gcp/schema/gcp/compute.cue`, fixture `instances.json`
- Docs: `docs/projection.md` (field paths, allowlists),
  `docs/schema-authoring.md`, `examples/gcp/README.md`

## Tests

- `internal/fieldpath/filter_test.go`: lookup and replace skip kept
  elements, non-objects and missing or non-string fields are redacted,
  one-or-many, equality, parse forms and errors, `ExceptWhere`, plain paths
  unchanged.
- `internal/importer/sensitive_allowlist_test.go`: an import keeps listed
  values and redacts the rest, including an unlisted new key; five broken
  allowlists fail closed.
- `TestGCPContentPack` asserts an allowlisted metadata value is stored; it
  fails against the previous schema.

## Not done

The repository's own workspace copy (`.pudl/schema/gcp/compute.cue`) still
has the old blanket redaction until the pack is reinstalled there.
