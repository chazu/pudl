# Classification explanations

Opt-in InferWithTrace and import --explain preserve candidate ordering, matching, schema selection, and confidence. Candidate diagnostics contain heuristic scores/reasons, bounded failure field paths (no rejected values), winning source search paths, and paths shadowed by them. Load failure paths and candidate attempts are bounded. Explicit schema validation explains chain fallbacks and unavailable schema retention. Existing import confidence is labeled as heuristic, not probabilistic.

schemaAssignment retains its original reason/source path for metadata independently of explanation mode; enrichAssignment writes them and the optional trace to SchemaInfo/results. assignItemSchemaDetailed makes the same diagnostics available to collection preparation. CLI renders traces for documents and bounded collection item traces, including deduplication without a historical trace.

Unit regressions verify inferred/explicit/fallback/unavailable assignment parity, sanitized field path output, and actual first-path source shadowing. Root integrates document and prepared-collection call sites separately alongside their transaction changes; it also regenerates the shared help golden and docs.
