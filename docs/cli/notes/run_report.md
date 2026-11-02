Standalone runs and run sets share `report`. With no ID, `report` selects the
newest persisted standalone or aggregate set report. With an ID, it reads that
exact report. The JSON payload retains its existing standalone or set
representation.

Reports preserve historical previous/current/expected values and bounded check witnesses. Reading a report adds current snapshot evidence availability without recomputing its findings. See [evidence lifetime](evidence.md#reports-and-evidence-lifetime).
