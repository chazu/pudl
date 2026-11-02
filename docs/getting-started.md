# Getting started: Git inventory and drift

Observe a sample Git inventory, compare it with a declared expectation, and
inspect a drift finding. This tutorial takes about five minutes and uses PUDL
commands plus one `cp` after you create the repository. The example ships
inside PUDL; you do not need a source checkout, scripts, Python, or external
credentials.

## 1. Install PUDL

With Go 1.26.9 or newer installed, install the current development version, which
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
pudl init
pudl example install git-inventory
pudl model show git-inventory
pudl schema show pudl/git.#GitRepository
pudl model validate git-inventory
```

The model expects `local/demo` to have default branch `main`. The supplied
inventories describe that fictional repository; they do not inspect or change
your new repository's branches. PUDL installs the model and data under `.pudl/`.
Reinstalling is safe: identical files are left alone, and edited files are
preserved with a conflict message.

`model show` lists one desired resource with schema `pudl/git.#GitRepository`
and resource type `git.repository`. Schema names use `package.#Definition`;
pass the full name to `pudl schema show` to inspect the CUE data shape. The
resource type is the model's `_schema` routing tag. `schema show` prints the
containing file; look for `#GitRepository` and its remote and branch types.
Use `pudl model show git-inventory --json` to inspect the model's desired values.

## 3. Observe and check the baseline

<!-- walkthrough:baseline -->
```bash
pudl run git-inventory
```

The model's populate step runs `cat` over the supplied inventory
(`.pudl/populators/git-inventory/current.json`) and records what it prints as
an observation snapshot. Any command that prints JSON records works the same
way; a real model would run a Git or forge CLI instead. The report says the
desired resources exist and match: the observed default branch and the model's
expectation are both `main`. Keep the printed `run_id` if you want to return to
this exact report later.

## 4. Change the inventory and find drift

<!-- walkthrough:change -->
```bash
cp .pudl/populators/git-inventory/changed.json .pudl/populators/git-inventory/current.json
pudl run git-inventory
```

The changed inventory has default branch `release`. The model still expects
`main`, so the report identifies the mismatch:

```text
- ~ git.repository/local/demo (changed): default_branch: release → want main
  - default_branch: expected "main", observed "release", previous "main" (snapshot snap_...)
  - observed_at: 2026-10-05T19:16:47Z
```

Each finding names the resource and a reason: `missing` (nothing observed with
that identity), `changed` (a desired field differs), `unidentifiable` (the
desired record has no identity to match), or `ambiguous` (several differing
observed records share its identity). Every differing field is listed with its
expected and observed values, the value from the previous observation, and
`observed_at` says when the compared record was recorded. The JSON report
carries the same finding under `drift.drifted`, with the field differences in
`fields`.

PUDL reports the difference and changes nothing. Each run keeps its own
snapshot, so the baseline evidence is preserved.

## 5. Repeat the check

<!-- walkthrough:repeat -->
```bash
pudl run git-inventory
pudl run report
pudl run report --json
```

The same mismatch appears under a new run ID. In JSON, `drift.clean` is `false`
and `drift.verified` is `true` because the run observed the inventory. `ok: true`
means the check ran successfully, even though it found drift. For automation,
inspect `drift.clean` or use `pudl run --detailed-exitcode`.

## 6. Inspect the evidence

List the changed record, then replace `RECORD_ID` below with the record ID
printed by the list command. Replace `BASELINE_RUN_ID` with the run ID from
step 3 to reopen the original clean report.

<!-- walkthrough:evidence -->
```bash
pudl list --where default_branch=release --items-only
pudl show RECORD_ID --raw
pudl run report BASELINE_RUN_ID
```

The raw record shows `default_branch: "release"` and the assigned
`pudl/git.#GitRepository` schema. The baseline report remains clean. Your
catalogs, records, models, and reports stay inside this repository's `.pudl/`
directory.

You can keep the repository to experiment with your own data. To observe a
real system, point a model's `populate.runs` at a command that prints JSON
records, such as `gcloud … --format=json` or `kubectl get … -o json`; see the
[GCP network hygiene example](gcp-network-hygiene.md). See the
[CLI reference](cli-reference.md) for import and report options, or
[concepts](concepts.md) for schemas, identity, and drift.

The [automated acceptance check](TESTING.md#git-inventory-walkthrough) executes
these tutorial commands with only PUDL, Git, `cat` and `cp` on `PATH`.

Reports keep previous/current/expected values and failed-check witnesses. A
catalog replay (`pudl run --from-catalog`) re-checks a stored snapshot without
observing; without an eligible prior snapshot it records an explicit
unavailable baseline rather than inventing observation history. See
[evidence and recovery](evidence.md) for report retention, faithful exports,
and portable backups.
