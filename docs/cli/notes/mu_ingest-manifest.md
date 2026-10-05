`ingest-manifest` records each applied action and sets the affected resources
to `converging` (applied, pending verification). Passing `--model <name>` tags
those rows with the model so a later clean `pudl run <name>` drift re-check
promotes exactly that model's resources from `converging` to `clean`. Without
`--model`, promotion falls back to matching the model's desired-resource names.
