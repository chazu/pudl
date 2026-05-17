# pudl guide command

Added `pudl guide` command providing quick-reference documentation for agents and humans, modeled after mu's guide system.

## Public API

- `pudl guide` — show topic index
- `pudl guide <topic>` — show guide for a specific topic
- Shell completion for topic names

## Topics

- `overview` — what pudl is, mental model, day-to-day verbs
- `import` — formats, schemas, wildcards, stdin, envelopes
- `schemas` — CUE schema system, inference, versioning, modules
- `facts` — bitemporal fact store, observations, retraction, time-travel
- `datalog` — query engine, rules, SQL compilation, evaluation modes
- `definitions` — named schema instances, sockets, dependency graphs
- `drift` — state divergence detection, export-actions
- `pith` — stack-based VM, vocabulary, heredoc input
- `mu` — ACUTE loop, observation pipeline, shared pith VM
- `agents` — conventions, --source, --scope, --json, workflows

## Implementation

Single file: `cmd/guide.go`. Topic dispatch via map for easy extension. Follows mu's pattern of plain-text formatted output with sections.
