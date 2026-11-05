`pudl show ID` prints the entry's catalog details, its compliance section and
then the stored record under `RAW DATA` (stored JSON indented with its values
and key order unchanged). Sensitive fields were redacted when the record was
stored, so show prints the redacted value. `--meta-only` prints the details and
compliance without the record; `--raw` is the default and is still accepted.
`--field PATH` prints one payload field as JSON; `--history` lists the
resource's stored versions and snapshot observations.

`--json` prints one object:

```json
{
  "entry": {"id": "…", "proquint": "…", "schema": "…", "origin": "…", "…": "…"},
  "raw": {"…": "the stored record"},
  "compliance": [{"policy": "…", "status": "…", "…": "…"}]
}
```

Stored JSON is embedded byte for byte as `raw`; other formats (YAML, CSV) appear
as the string `raw_text`. `--meta-only` omits `raw`/`raw_text`; `--metadata`
adds the metadata file as `metadata`.
