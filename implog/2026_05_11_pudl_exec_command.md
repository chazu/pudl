# pudl exec CLI command

**Date:** 2026-05-11

## Summary

Added `pudl exec` command for running pith VM programs against the pudl data lake. Intended for testing and debugging pith programs with full access to pudl driver words (catalog/*, fact/*, schema/*).

## Public API

### CLI Usage

```
pudl exec '<json-program>'       # Run program from JSON string argument
pudl exec -f <file>              # Run program from JSON file
pudl exec --trace ...            # Enable trace mode (stack printed to stderr after each op)
pudl exec --context key=value    # Set context values (repeatable), available as ctx.key field refs
pudl exec --json ...             # Format result as indented JSON
```

### Behavior

- Program is a JSON array of pith ops (strings, numbers, booleans, arrays/quotations, objects)
- Opens catalog DB and schema manager, registers all pudl driver words
- Prints top-of-stack as JSON after execution; prints nothing if stack is empty
- Context values are parsed as JSON when possible, otherwise treated as strings

## Files

- `cmd/exec.go` (~140 lines) - single-file command implementation
