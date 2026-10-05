Standalone runs and exact sets share `report`, `resume` and `reject`. With no
ID, `report` selects the newest persisted standalone or aggregate set report.
With an ID, it reads that exact report. The JSON payload retains its existing
standalone or set representation. `resume` and `reject` look up the stored
operation kind; set approvals rebuild and validate the exact plan, while
standalone approvals retain their request-level behavior.
