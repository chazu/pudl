`report` reads every stored report: check reports, the report of one model
within a check (by its run ID), and reports recorded by the retired `pudl run`
and `pudl run set` commands. With no ID it selects the newest stored report.
With an ID it reads that exact report. The JSON payload is the report as stored.

Check reports from `report_version` 3 embed each model's full report under
`members[].report`, and the human rendering shows each model's findings before
the check summary. Older run-set reports (versions 1 and 2) carry only member
summaries; read a member's findings with `pudl report <run_id>`. Fields a stored
report has that current reports do not, such as those from the retired
convergence workflow, are shown as recorded under "legacy fields".

Reports preserve historical previous/current/expected values and bounded check witnesses. Reading a report adds current snapshot evidence availability without recomputing its findings. See [evidence lifetime](evidence.md#reports-and-evidence-lifetime).
