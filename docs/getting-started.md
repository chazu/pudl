# Getting started: Git inventory and drift

Import a sample Git inventory, compare it with a declared expectation, and
inspect a drift finding. This tutorial takes about five minutes and uses only
PUDL commands after you create the repository. The example ships inside PUDL;
you do not need a source checkout, scripts, mu, Python, or external credentials.

## 1. Install PUDL

With Go 1.26.2 or newer installed, install the current development version, which
includes the bundled example command:

```bash
go install github.com/chazu/pudl@main
```

Put Go's binary directory on your shell's `PATH` if it is not already there.
The default is `~/go/bin`; a configured `GOBIN` overrides it. Open a new terminal
after changing your shell settings, then confirm that PUDL is available:

```bash
pudl version
pudl example install --help
```

You also need Git. The remaining steps work from any directory where you want
to keep a new repository.

## 2. Create the repository and install the example

<!-- walkthrough:repository -->
```bash
git init pudl-tutorial
cd pudl-tutorial
```

<!-- walkthrough:setup -->
```bash
pudl repo init
pudl example install git-inventory
pudl model show git-inventory
pudl model validate git-inventory
```

The model expects `local/demo` to have default branch `main`. The supplied
inventories describe that fictional repository; they do not inspect or change
your new repository's branches. PUDL installs the model and data under `.pudl/`.
Reinstalling is safe: identical files are left alone, and edited files are
preserved with a conflict message.

## 3. Import and check the baseline

<!-- walkthrough:baseline -->
```bash
pudl mu ingest-observe --path .pudl/populators/git-inventory/baseline-observe.json --origin git-baseline
pudl list --origin git-baseline --items-only
pudl run git-inventory --from-catalog --catalog-scope git-baseline
```

The report says the desired resources exist and match: the imported default
branch and the model's expectation are both `main`. Keep the printed `run_id`
if you want to return to this exact report later.

`pudl mu ingest-observe` imports a saved observation and records its snapshot in
PUDL's catalog. Despite the command's name, the mu executable is not required.
The example observation is supplied with PUDL.

`--from-catalog` checks observations you already imported. `--catalog-scope` selects
which inventory to use; here it is the `git-baseline` origin supplied during
import. This is a check of stored evidence, so it does not claim to have observed
a live Git server.

## 4. Import the changed inventory and find drift

<!-- walkthrough:change -->
```bash
pudl mu ingest-observe --path .pudl/populators/git-inventory/changed-observe.json --origin git-changed
pudl run git-inventory --from-catalog --catalog-scope git-changed
```

The changed inventory has default branch `release`. The model still expects
`main`, so the report identifies the mismatch:

```text
git.repository/local/demo (changed): default_branch: release → want main
```

The two origins keep the baseline and changed inventories separate. Importing
the changed data preserves the earlier evidence.

## 5. Repeat the check

<!-- walkthrough:repeat -->
```bash
pudl run git-inventory --from-catalog --catalog-scope git-changed
pudl run report
pudl run report --json
```

The same mismatch appears under a new run ID. In JSON, `drift.clean` is `false`;
`drift.verified` is also `false` because this is catalog replay. `ok: true` means
the check ran successfully, even though it found drift. For automation, inspect
`drift.clean` to determine whether the inventory matches.

## 6. Inspect the evidence

List the changed inventory's records, then replace `RECORD_ID` below with the
record ID printed by the list command. Replace `BASELINE_RUN_ID` with the run ID
from step 3 to reopen the original clean report.

<!-- walkthrough:evidence -->
```bash
pudl list --origin git-changed --items-only
pudl show RECORD_ID --raw
pudl run report BASELINE_RUN_ID
```

The raw record shows `default_branch: "release"` and the assigned
`pudl/git.#GitRepository` schema. The baseline report remains clean. Catalog
replay creates a durable run report; it does not create a new observation
snapshot. Your catalogs, imported records, models, and reports stay inside this
repository's `.pudl/` directory.

You can keep the repository to experiment with your own data. For live
observation and model authoring, continue with [mu integration](mu-integration.md).
See the [CLI reference](cli-reference.md) for import and report options, or
[concepts](concepts.md) for schemas, identity, and drift.

The [automated acceptance check](TESTING.md#git-inventory-walkthrough) executes
these tutorial commands with only PUDL and Git on `PATH`.
