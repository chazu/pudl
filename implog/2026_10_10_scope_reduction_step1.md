# Scope reduction step 1

Removed `setup` and `model deps --derive`. Completion and `list --fancy` remain.
Migration 24 retracts only heuristic-owned dependency facts and retains history.
Added migration coverage for declared, binding, custom, and mismatched sources.
Updated active documentation, generated command help, and embedded agent skills.
Focused database migration and command/run-set checks passed.

Public surface: the two retired entry points now fail; no aliases are installed.
See [migration guidance](../docs/retired-commands.md).
