# Symmetric bundle manifest limits

Idea 4: export and restore share the 8 MiB manifest size cap. The writer validates
serialized metadata before creating a temporary output or compressor, so it cannot
report success for framing that its own reader rejects. Oversized inventories now
fail explicitly rather than creating an unusable backup. Bundle version and public
API are unchanged.

Validation: full bundle suite passed, including a manifest above the reader limit
that preserves an existing archive and creates no output temporary file.
