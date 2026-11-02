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

`pudl facts add`, `pudl facts retract` and `pudl facts invalidate` are not
retired. They are advanced commands: `pudl facts --help` and shell completion
no longer list them, but they run unchanged and are documented under "Advanced
commands" in [cli-reference.md](cli-reference.md#advanced-commands). Read the
fact store with `pudl query`; Go programs write facts through `pkg/factstore`.

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

## Convergence and mu integration

PUDL discovers, catalogs and governs infrastructure; it no longer changes it.
Convergence and every mu integration were removed on 2026-11-01
([design](design/2026-11-01-remove-convergence-and-mu.md)). Terraform, mu and
similar tools own changes. Existing catalogs open unchanged: stored converge
and run-set reports, approval rows and run history remain readable.

| Retired command or flag | Replacement |
| --- | --- |
| `pudl run --converge` and `--only`, `--dry-run`, `--max-iters`, `--max-applies`, `--require-approval` | `pudl run <model>` reports drift between `desired` and what was observed; apply changes with the tool that owns them, then run again to verify |
| `pudl run resume`, `pudl run reject` | None. Nothing waits for approval any more |
| `pudl run set --converge` and its `--require-approval`, `--max-iters`, `--max-applies` | `pudl run set <model>...` (observe-only) |
| `pudl run --populate plugin:<name>` and `--input` | `pudl run --populate 'command:<cmdline>'` running the same program |
| `pudl model new --populate plugin:<name>` and `--input` | `pudl model new <name> --populate 'command:<cmdline>'`; without `--populate` the scaffold is a `#CommandObserve` placeholder |
| `pudl model populator add`, `pudl model populator new` | Keep helper programs and fixtures anywhere in the repository and reference them from a `#CommandObserve` run (`dir:` is relative to the model file) |
| `pudl model show --discover` | None. Models no longer reference mu plugins |
| `pudl run --mu-root`, `pudl run set --mu-root`, `--mu-timeout`, `pudl mu ingest-observe --plugin-dir` | None. Bound a command with `populate.timeout` |

### Retired model fields

A model that still uses one of these fields fails to load. The error names the
field and points here. Remove the field to load the model again.

| Field | Replacement |
| --- | --- |
| `converge:` (`#PluginPlan`) | Remove it. `desired` still drives drift reports |
| `sealed_inputs`, `sealed_outputs` | Remove them. Give a populate command its credentials through its own environment or configuration (for example a `gcloud` or `kubectl` profile) |
| `populate: #PluginObserve` (`plugin`, `input`, `differential`) | `populate: runs: [{argv: [...]}]` running a program that prints the same records as JSON. Differential drift is gone; drift always compares `desired` with the observed records |
| `populate: #EweTarget` (`eweSource`, `outputs`) | `populate: runs: [{argv: [...]}]` running a program that prints the records |
| `plugins:` (`#PluginDef`) | Remove it; a `#CommandObserve` run names its program directly in `argv` |

The `secrets: writable_refs` setting in `workspace.cue` is ignored.
