# Retired convenience commands

After upgrading an existing workspace, run `pudl init` to refresh its owned
built-in schemas before using the new observation and check fields.

PUDL keeps shell completion but no longer edits shell profiles. Generate the
completion script with `pudl completion bash`, `pudl completion zsh`, or
`pudl completion fish` and install it through your shell's normal configuration.

If you previously ran `pudl setup`, inspect your shell configuration and manually
remove the block between `# PUDL Shell Integration` and
`# End PUDL Shell Integration`. Preserve any customizations you still use.
Upgrades do not modify shell profiles or their backups.

`pudl model deps --derive` is removed. Declare `depends_on` or use explicit value
bindings to establish dependencies. Migration 24 retracts current
`model_depends_on` facts whose source is exactly `derived:<from>` and whose
arguments are exactly the heuristic's `from` and `to` fields. It preserves fact
history and other sources. That source convention belonged to the retired
heuristic; custom assertions should use their own source.

`pudl list --fancy` remains supported.

## Native Git and CUE replacements

Use `pudl config --paths --json` to locate the active schema tree. Project files
belong to the enclosing repository; personal-mode schemas can have their own
Git repository. `pudl schema show NAME --json` supplies a schema's source path.

| Retired command | Replacement |
| --- | --- |
| `pudl schema status` | `git -C .pudl/schema status -- .` in a project |
| `pudl schema commit` | Stage the intended schema files with `git add`, review the staged diff, then use `git commit` |
| `pudl schema log` | `git -C .pudl/schema log -- .` in a project |
| `pudl schema edit` | Open the source path in your editor, for example `emacsclient <file.cue>` |
| `pudl module add MODULE@VERSION` | Run `cue mod get MODULE@VERSION` from the schema directory, then `cue mod tidy` |
| `pudl module tidy` | Run `cue mod tidy` from the schema directory |
| `pudl module list` / `info` | Inspect `cue.mod/module.cue`, or run `cue mod edit --json` from the schema directory |

In global mode, substitute the schema path printed by
`pudl --global config --paths --json`. These removals do not delete schema files,
Git history, CUE dependency declarations, or editor settings. Schema generation,
validation, listing, and inference remain PUDL operations. There are no command
aliases for the retired wrappers.
