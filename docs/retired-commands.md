# Retired convenience commands

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
