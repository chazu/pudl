With no ID, `report` reads the newest stored report; with an ID, that report:
a check ID, or the run ID of one model within a check. `--json` prints the
report as stored. Reports keep their historical values and witnesses; reading
one adds whether its evidence snapshots are still available, without
recomputing findings (see
[evidence lifetime](evidence.md#reports-and-evidence-lifetime)).

`--since ID2` compares instead of printing: each check's findings are
classified against the same check in ID2, models paired by name, as `new`,
`persisting`, `resolved` or `unknown`. A check is skipped when ID2 lacks it,
reached no verdict for it, or evaluated it differently. `--json` writes
`report`, `since`, and per model each check's `changes` and `findings`. See
[changes over time](governance.md#track-changes-over-time).

Reports recorded by the retired `pudl run` and `pudl run set` read the same
way. Older run-set reports (versions 1 and 2) carry only member summaries;
read a member with `pudl report RUN_ID`. Fields current reports no longer
have are shown under "legacy fields".
