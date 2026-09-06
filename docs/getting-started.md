# Getting started: Git inventory and drift

This walkthrough captures a supplied Git repository inventory, checks its default
branch against a declared expectation, and retains the evidence for each run.
It uses the built-in `pudl/git.#GitRepository` schema and takes about five minutes
once the local tools are installed. No external account or credentials are needed.
The inventory describes a fictional `local/demo` repository; the observer reads
JSON fixtures rather than contacting a Git hosting service.

## Prerequisites

Use Bash on macOS or Linux, Git, Python 3, and `mu` on your `PATH`. This workflow
is tested with **mu v0.3.5**. Install that version with Go if needed:

```bash
go install github.com/chazu/mu/cmd/mu@v0.3.5
export PATH="$(go env GOPATH)/bin:$PATH"
mu version
```

From a PUDL source checkout, build the CLI using the repository's pinned toolchain
(`mise`), or use `make build` with the Go version declared in `go.mod`:

```bash
mise exec -- make build
```

Run the following blocks in order, in the same Bash shell, starting at the root
of that checkout. Setup checks the required tools before creating the workspace.

## 1. Create an isolated workspace

<!-- walkthrough:setup -->
```bash
pudl_source="$PWD"
pudl_bin="$pudl_source/pudl"
test -x "$pudl_bin" || { echo 'Build PUDL first: mise exec -- make build' >&2; exit 1; }
for tool in git mu python3; do
  command -v "$tool" >/dev/null 2>&1 || { echo "Missing required tool: $tool" >&2; exit 1; }
done
walkthrough="$(mktemp -d "${TMPDIR:-/tmp}/pudl-git-inventory.XXXXXX")"
git init -q "$walkthrough"
cd "$walkthrough"
"$pudl_bin" repo init
mkdir -p .pudl/populators/git-inventory .pudl/data/mu .pudl/data/walkthrough
cp "$pudl_source/examples/git-inventory/model.cue" .pudl/schema/models/git_inventory.cue
for file in observe.py baseline.json changed.json; do
  cp "$pudl_source/examples/git-inventory/$file" .pudl/populators/git-inventory/
done
cp .pudl/populators/git-inventory/baseline.json .pudl/populators/git-inventory/current.json
python3 - <<'PY' > .pudl/data/mu/mu.cue
import json
from pathlib import Path
print('package mu')
print('cache: backends: [{type: "disk", path: ' + json.dumps(str(Path('.pudl/data/mu/cache').resolve())) + '}]')
PY
"$pudl_bin" model validate git-inventory
printf 'Workspace: %s\n' "$walkthrough"
```

The supplied model in `.pudl/schema/models/git_inventory.cue` declares
`default_branch: "main"` for `local/demo`. Its observer reads `current.json`;
PUDL resolves the `git.repository` resource type to the shipped Git schema.
The model runs the local Python observer through mu's `command` form, so mu does
not install a global plugin copy. `--mu-root` and the explicit disk backend keep
mu's project and cache local. PUDL's catalog lives at `.pudl/data/sqlite/catalog.db`.

## 2. Capture the clean baseline

<!-- walkthrough:baseline -->
```bash
"$pudl_bin" run git-inventory --mu-root .pudl/data/mu --json > .pudl/data/walkthrough/baseline.json
"$pudl_bin" run report
```

The report shows one observed record and clean drift: the observed default
branch and the model's expectation are both `main`. The JSON report retains
`run_id` and `populate.snapshot_id` for inspecting this exact observation later.

## 3. Change the inventory and find drift

Replace the observed inventory with the supplied version whose default branch
is `release`. The model continues to expect `main`.

<!-- walkthrough:change -->
```bash
cp .pudl/populators/git-inventory/changed.json .pudl/populators/git-inventory/current.json
"$pudl_bin" run git-inventory --mu-root .pudl/data/mu --json > .pudl/data/walkthrough/changed.json
"$pudl_bin" run report
```

The drift section identifies the resource and both values:

```text
git.repository/local/demo (changed): default_branch: release → want main
```

In JSON, `drift.clean` is `false` and `drift.drifted` contains that finding.
`ok: true` and a zero exit status mean this observation completed successfully;
they do not mean the inventory matches. This model declares no fail-severity
checks and performs no convergence. For automation, inspect `drift.clean`.

## 4. Repeat the observation

<!-- walkthrough:repeat -->
```bash
"$pudl_bin" run git-inventory --mu-root .pudl/data/mu --json > .pudl/data/walkthrough/repeat.json
"$pudl_bin" run report
```

The same mismatch remains, with a new run ID and snapshot ID. Each run compares
the expectation against its own snapshot. The earlier clean observation is
still available by its run ID. The repeat's populate count is zero because its
record content is already stored; the new snapshot still contains one member.

## 5. Follow the finding to its evidence

Use the changed run's stored IDs, even though the repeat run is now the latest.
The snapshot identifies its run and observed record count; listing its members
and showing the record reveals the actual `release` value.
Run observations use origin `pudl-run`; specify it when listing to override the
default filter for imports named after the workspace.

<!-- walkthrough:evidence -->
```bash
changed_run_id="$(python3 -c 'import json; print(json.load(open(".pudl/data/walkthrough/changed.json"))["run_id"])')"
changed_snapshot_id="$(python3 -c 'import json; print(json.load(open(".pudl/data/walkthrough/changed.json"))["populate"]["snapshot_id"])')"
"$pudl_bin" run report "$changed_run_id"
"$pudl_bin" run report "$changed_run_id" --json
"$pudl_bin" show "$changed_snapshot_id" --raw
"$pudl_bin" list --origin pudl-run --collection-id "$changed_snapshot_id" --json > .pudl/data/walkthrough/changed-records.json
changed_record_id="$(python3 -c 'import json; print(json.load(open(".pudl/data/walkthrough/changed-records.json"))["entries"][0]["id"])')"
"$pudl_bin" show "$changed_record_id" --raw
baseline_run_id="$(python3 -c 'import json; print(json.load(open(".pudl/data/walkthrough/baseline.json"))["run_id"])')"
"$pudl_bin" run report "$baseline_run_id"
```

The baseline report remains clean. The changed and repeat reports retain the
observed/expected comparison. The raw record is typed as
`pudl/git.#GitRepository`. All catalogs, snapshots, generated mu projects, and
saved reports are inside the temporary workspace printed during setup.

Keep that directory to explore, or return to your source checkout and delete
the printed temporary directory when finished. Running setup again from the
checkout creates a new independent workspace.

## Maintained acceptance check

From the source checkout, run:

```bash
mise exec -- make test-git-walkthrough
```

The test executes this guide's marked setup, baseline, change, repeat, and
evidence blocks verbatim, with the real mu executable. It checks clean/drift
verdicts, durable report replay, typed snapshot records, and workspace isolation.
CI runs it on every push and pull request with mu v0.3.5.

For your own data and models, continue with [concepts](concepts.md), the
[CLI reference](cli-reference.md), [mu integration](mu-integration.md), or
[cross-model dependencies](cross-model-dependencies.md).
