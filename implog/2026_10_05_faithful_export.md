# Faithful, fail-closed exports

Export now walks structured JSON values, YAML documents, CSV rows, and NDJSON records one record at a time into an on-disk spool, and expands catalog collections through normalized membership. Records selected both directly and through collections are emitted once. Exact JSON numeric tokens survive JSON/NDJSON/CSV output and receive numeric YAML tags. CSV headers sort deterministically; nested values and scalar records fail explicitly; flush failures propagate.

Format validation precedes destination creation. File exports use a same-directory temporary file, sync/close, then rename, preserving an existing destination on failure. Default source failures abort; --allow-partial discards each failed entry's staged prefix, names omissions, publishes readable records, and exits nonzero with an incomplete-output error.

Focused export tests pass. Full cmd tests currently require regenerating the help golden for the added flag; root integrates documentation and the shared CLI golden after all command flags land.

Follow-up acceptance exercises the command itself: invalid format and unreadable selected evidence preserve a preexisting output; --allow-partial publishes exact readable evidence and returns an incomplete error. Direct command invocations normalize nil contexts. YAML export reads source nodes to preserve integers and decimals without float64 rounding, while validating aliases/duplicate keys and resolving normal merge precedence; importer classification contracts are unchanged.
