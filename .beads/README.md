# Beads Rust issue tracking

This repository uses [Beads Rust](https://github.com/Dicklesworthstone/beads_rust)
through the `br` command. Issues are stored locally in SQLite and exported to
the tracked `.beads/issues.jsonl` file for collaboration.

The project issue prefix is `pudl-`.

```bash
br ready
br show <id>
br create --title="..." --description="..."
br update <id> --status=in_progress
br close <id> --reason="..."
br sync --flush-only
```

`br` never commits or pushes changes. Commit `.beads/issues.jsonl` with the
rest of the repository changes when landing issue updates.
