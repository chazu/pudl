# pudl prime command

Added `pudl prime` — a command that outputs a structured prompt teaching AI
agents how to use pudl.

## Public API

- `pudl prime` — prints agent-oriented documentation to stdout covering:
  - Core concepts (catalog, schemas, definitions, fact store, Datalog, workspaces)
  - All major command groups with examples
  - Agent-specific conventions (--source, --scope, --json, temporal queries)

## Usage

Add a line like this to CLAUDE.md or equivalent in any repo:

    Run `pudl prime` to learn how to use the pudl data lake CLI.

The agent reads the output and knows how to use pudl effectively.

## Files

- `cmd/prime.go` — command implementation with embedded prompt text
