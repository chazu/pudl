# Native tool wrappers retired

Removed schema status/commit/log/edit and module add/tidy/list/info, plus the
unused internal/git wrapper. Native replacement commands use the paths exposed
by config --paths and schema show. Schemas, CUE modules, Git history and editor
configuration are preserved. list --fancy, Bubble Tea/Bubbles and completion
remain supported. Updated current help, bootstrap guidance, docs and skills.

Validation: command and UI tests, removed-command failure checks, generated
command-help coverage, and the native consolidated-command smoke passed.
Full integration qualification is recorded in the completion audit.
