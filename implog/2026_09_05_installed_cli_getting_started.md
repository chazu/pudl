# Getting started from an installed PUDL CLI

Follow-up to `pudl-sjk`: the user requested an install-and-use tutorial with PUDL
on `PATH`, a new Git repository, and PUDL commands instead of pasted scripts.

## Delivered behavior

`pudl example install git-inventory` installs the maintained example from assets
embedded in the binary. It supplies the registered model, baseline and changed
inventories, saved observation files, and the optional live observer and local
mu configuration. It needs an initialized repository workspace and performs no
downloads. Reinstallation preserves identical files. Conflicting content and
non-regular destinations fail preflight before any files are written; there is
no overwrite flag. The `example` command family never auto-initializes global
PUDL state.

The getting-started guide now starts with `go install github.com/chazu/pudl@main`
and PATH setup, then `git init` and `cd`. Every tutorial step after repository
creation is a PUDL command. `pudl mu ingest-observe` imports the supplied saved
observations, and `pudl run --from-catalog --catalog-scope` checks the explicit
baseline or changed origin. The tutorial needs neither mu nor Python; importing
saved observations is a PUDL operation. It explicitly distinguishes catalog
replay from fresh observation: drift is unverified, while the report and
imported snapshot/records remain durable evidence.

The guide uses printed IDs for `pudl show` and named report replay, with no shell
variables, command substitutions, Python parsing, file-copy commands, or shell
scripts. README, CLI reference, testing documentation, and the development plan
describe the installed CLI workflow. `@main` is intentional: the new example
command is not in older tagged releases.

## Public interface

- `pudl example install git-inventory`
- `pudl example install git-inventory --json`: returns `example`, absolute
  workspace `root`, and relative `files` paths.
- `pudl example install --help` documents prerequisites and conflict behavior.

Existing import, observation, drift, and report APIs are unchanged.

## Validation

`make test-git-walkthrough` executes the marked guide commands with a freshly
built PUDL binary and Git as the only tools on `PATH`. It uses a fresh Git
repository with spaces in its path and no source fixtures to copy. Tests assert
the clean baseline, changed/repeated mismatch, typed raw evidence, named report
replay, and absence of global PUDL/mu state. ID placeholders are filled from CLI
output as directed by the guide. Installer coverage checks idempotence,
preservation of edits, no partial writes on conflict, and refusal outside an
initialized workspace.

The optional real-mu Git observation test now installs the same bundled example
and runs under `make test-kick-tires`. It retains baseline/change/repeat snapshot
and evidence checks. CI runs the command-only tutorial in the ordinary test job
and the live observation path in the mu v0.3.5 job.

Validation uses the pinned Go toolchain via `mise exec --`: unit tests, race tests
with the repository's checkptr exception, vet, build, generated-skill checks,
the command-only walkthrough, and the real-mu kick-the-tires matrix. The latter
was run with the separately built published mu v0.3.5 binary.
