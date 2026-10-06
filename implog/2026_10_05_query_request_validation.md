# Query request validation

Idea 1 from the thirty-candidate selection: query constraints no longer silently
discard malformed tokens or overwrite repeated keys. Empty values, arbitrary
nonblank field names, embedded equals, exact integers and quoted strings keep
their existing semantics. Negative iteration caps and contradictory list requests
fail before evaluation. No new public Go API.

Validation: command-level rejection cases, valid constraint semantics, and existing
numeric/JSON query regression tests passed under Go 1.26.6. CLI reference regenerated.
