# Packs name no project, account or region; global example install

## Global example install

`pudl example install NAME --global` installs into `~/.pudl`. Before, global
mode built a workspace policy with no repository workspace, so
`cmd/example.go` always refused and told the user to run `pudl init`, which
could not help.

- `exampleInstallRoot(global bool)` in `cmd/example.go` picks the root: the
  repository workspace, or `wsPolicy.GlobalDir` only when `--global` is
  passed, so running outside a repository never writes to `~/.pudl` by
  accident. The global workspace must hold `config.yaml` (written by
  `pudl init --global`).
- Outside a workspace without `--global` the error now reads "run pudl init
  here, or pass --global to install into ~/.pudl".
- Smoke test `TestSmoke_ExampleInstallGlobal`.

Known gap, not changed: `pudl init --global` writes `schema/cue.mod` only when
the `cue` binary is on PATH, so without it models that import
`pudl.schemas/...` fail to load in the global workspace.

## GCP pack: projects discovered at run time

The live collectors (`gcp-<type>`) no longer read a hard-coded
`_gcpProjects` list. `_#GCPLive` sets `populate.each` to `_gcpProjectList`
(`gcloud projects list --format=json`, `select: "projectId"`), and every run
uses `--project={{item}}` and `set: project: "{{item}}"`.

- Settings (both defaulted, so they can be set from a user's own file in
  package `models` without editing the pack):
  - `_gcpProjectFilter: string | *""` adds `--filter=<value>` to the project
    list.
  - `_gcpKMSLocations: [...string] | *["global"]`.
- `_gcpKMSKeyRings` is removed. `gcp-kms-keys` runs
  `populators/gcp/kms-keys.sh PROJECT LOCATION...`, which lists each
  location's key rings and prints each ring's keys (one JSON array per ring).
- Fixture collectors are unchanged.
- Test `TestGCPPackLiveCollectorsEnumerateProjects` (fake `gcloud` on PATH):
  inputs, record counts, the helper's calls and the filter override.
- Docs: `examples/gcp/README.md` ("Run against live projects"), `examples/aws/README.md` ("Run against live accounts"),
  `cmd/guide/packs.txt`, regenerated `docs/cli-reference.md`.

## AWS pack: profiles, accounts and regions discovered at run time

`_awsAccounts` and `_awsRegions` are removed. `_#AWSLive` sets
`populate.each` to `_awsProfileList` (`aws configure list-profiles`, printed
as JSON strings, filtered by the shell glob `_awsProfileFilter: string |
*"*"`). Every live run is one `sh -c` script per profile:

- `_awsShPrelude` gains `account PROFILE` (`sts get-caller-identity`) and
  `regions PROFILE FILTER` (`ec2 describe-regions --filters
  Name=region-name,Values=FILTER`, asked of the profile's configured region,
  else us-east-1). `_awsRegionFilter: string | *"*"`.
- `_awsRegional` entries are `{query, args}`; `_awsScripts.regional` walks the
  regions and adds AccountId/Region with `--query "<query> | [].merge(@,
  `{...}`)"`. `iam-roles` does the same with AccountId. The S3, KMS and IAM
  user scripts print AccountId (and Region) themselves. `set` is no longer
  used by live runs.
- Region lists are assigned before looping, so a failing describe-regions
  fails the run instead of yielding an empty, "complete" observation.
- Test `TestAWSPackLiveCollectorsEnumerateProfiles` (fake `aws` on PATH):
  three profiles, two regions, merged fields, both filters.

## Tutorial, guide and skill

- `gcp-network-hygiene-live` enumerates projects with `each`; scope is
  `gcp/firewalls` (`docs/gcp-network-hygiene.md`).
- `pudl guide observe` and the `pudl-core` skill (`skills/pudl-core/SKILL.md`,
  regenerated embed) show `each` as the collector example instead of a fixed
  project list.
- `cmd/testdata/help.golden.json` updated for the example install help text.
