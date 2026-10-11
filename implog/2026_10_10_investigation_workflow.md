# Daily evidence investigation workflow

Added the bundled gcp-network-hygiene example with fixture data, correction data,
schema, relationship rule, snapshot-scoped check, and separately named live
gcloud model. The user walkthrough needs no authored CUE to find the first
fixture violation. It identifies broad IPv4 ALLOW declarations, not effective
packet reachability.

New public surfaces:

- `list --where path=value --select path --all`: payload matching before
  pagination, exact typed values, explicit missing fields and scan budgets.
- `show --field` and `show --history`: field inspection and resource chronology.
- `snapshot show NEW --compare OLD`: scope-aware snapshot changes.
- `model new NAME --check RELATION --evidence SELECTOR --max-age DURATION`:
  save an existing query as a check without writing model boilerplate.
- `doctor --json` includes reviewable repair arguments without applying them.
- Saved observation ingestion accepts explicit scope/completeness/coverage.

Inventory drift now distinguishes not-observed in partial inventory from proven
absence. Such uncertainty cannot claim fresh verification or promote status.
`list --fancy` remains available and can browse payload-filtered results.

Validation: focused and full command/lister/database/acute tests, including the
GCP fixture journey, old-report preservation, field/wildcard access, pagination,
null versus missing, saved checks, snapshots and resource history.

A scripted native-binary trial compared the review-baseline binary with the new
workflow on the same fixture and correction. Both found one violation, then
passed after correction, and reopened the original report; neither had an
unexpected command failure. Baseline: two CLI calls to the first finding plus
three supplied CUE files and fixture data. New: three CLI calls (`init`, example
installation, `run`), zero custom CUE files, and bundled fixture data. CLI elapsed
time in this single warm local trial was 0.126s baseline and 0.069s new, excluding
build, authoring and reasoning time. These timings are not a performance claim
or a measurement of human time to understanding. The full agent test also
exercises inspection, saved checks, comparison and history. Live GCP access and
an actual human usability trial were not performed.
