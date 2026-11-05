Available packs: `gcp` and `aws`. No checkout or download is needed.
Identical files are left alone. Conflicting edits or symlinked destinations
cause an error before installation writes files. This command does not
initialize global state. Installs are recorded in
`<workspace>/.packs/<name>.json`.

`gcp` is the GCP content pack: draft schemas for the core GCP inventory,
best-practice policies, gcloud and fixture collectors, and three check
families. See [the pack README](../packs/gcp/README.md).

`aws` is the AWS content pack: draft schemas for VPC networking, EC2, EBS,
S3, RDS, KMS and IAM, best-practice policies, relationship and impact rules,
aws CLI and fixture collectors, and three check families. See
[the pack README](../packs/aws/README.md).

Packs installed with `pudl example install gcp|aws` (before packs had their
own command) have no `.packs` record; install once with `--overwrite`. See
[retired commands](retired-commands.md#content-packs).

`--json` returns `pack`, the workspace `root`, relative `files` paths, and
the `added`, `upgraded`, `replaced` and `unchanged` lists.

When an install adds or changes files, the facts of records already in the
catalog are reprojected (as `pudl doctor --fix --only projections` would), so
a pack upgrade that changes a type's `_pudl.facts` takes effect at once. A
workspace with no catalog yet is left without one.
