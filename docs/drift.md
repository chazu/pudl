# Drift Detection Guide

Drift detection compares a definition's declared state (socket bindings) against live state (method output artifacts). This identifies when infrastructure has changed outside of PUDL's management.

## How It Works

1. PUDL reads the definition's declared socket binding values
2. Retrieves the latest artifact for the definition (or re-executes the method with `--refresh`)
3. Performs a JSON deep diff between declared and live state
4. Reports added, removed, and changed fields with dot-notation paths

## CLI Commands

### Check a single definition

```bash
pudl drift check my_instance
```

Output shows field-level differences:

```
Definition: my_instance
Method:     list
Status:     drifted

Differences (2):
  ~ State.Name: "running" -> "stopped"
  + Tags.maintenance: "scheduled"
```

### Check all definitions

```bash
pudl drift check --all
```

Shows a summary for each definition with drift status and difference count.

### Specify the method

By default, drift detection auto-detects which method's artifact to compare. Override with `--method`:

```bash
pudl drift check my_instance --method list
```

### Refresh before comparing

Re-execute the method to get fresh live state before comparing:

```bash
pudl drift check my_instance --refresh
```

### Pass tags

```bash
pudl drift check my_instance --tag env=prod
```

### View last saved report

Display the most recent drift report without re-running:

```bash
pudl drift report my_instance
```

## Report Storage

Drift reports are stored in `.pudl/data/.drift/<definition>/<timestamp>.json`. Each report contains:

- Definition name and method
- Timestamp of the check
- Status (`clean` or `drifted`)
- List of differences with type (`added`, `removed`, `changed`), path, declared value, and live value

## Flags Reference

| Flag | Description |
|------|-------------|
| `--method` | Method whose artifact to compare (default: auto-detect) |
| `--refresh` | Re-execute the method before comparing |
| `--all` | Check all definitions |
| `--tag` | Extra args as key=value (repeatable) |
