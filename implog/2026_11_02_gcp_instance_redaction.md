# GCP pack: redact instance metadata; external-IP facts

The pack's `gcp.#ComputeInstance` stored instance metadata in plaintext. On
GKE nodes that metadata holds `kube-env` (cluster certificates and bootstrap
credentials), and on other VMs startup scripts and user-data. A workspace
that switched from a hand-written instance schema to the pack's would have
started storing them.

## Changes

- `examples/gcp/schema/gcp/compute.cue` (`#ComputeInstance`):
  - `sensitive_fields: ["metadata.items[*].value"]`. Values are redacted
    before storage, and the keys are kept.
  - `secret_scan_ignore` for the Shielded VM Secure Boot state
    (`disks[*].shieldedInstanceInitialState.{pk,keks[*],dbs[*],dbxs[*]}.content`).
    These are public UEFI certificates and revocation hashes: high-entropy but
    not secret.
  - A typed optional `metadata` field.
  - A new fact, `gcp_instance_external_ip {network, address}`, with one fact
    per external IP, giving the network that IP is attached on. It lets
    instance-exposure rules join external addresses to firewall rules.
- Fixture `examples/gcp/populators/gcp/instances.json`: `orders-api-1` gains
  a fake `startup-script` and `kube-env`.

## Public API

- Fact `gcp_instance_external_ip(resource_id, entry_id, network, address)`
  from `gcp.#ComputeInstance`.

## Verification

- `TestGCPContentPack` asserts that metadata keys are listed, values are
  `[REDACTED]`, and neither fixture value is stored. It also asserts exactly
  one `gcp_instance_external_ip` fact (203.0.113.50 on prod-vpc). Run against
  the old schema, the test fails with the plaintext stored.
- `CGO_ENABLED=0 go test ./...` passes except
  `internal/proc TestCommandCancelSendsSIGTERM`, which is flaky under
  full-suite load. It passes alone (3 of 3 runs) and does not touch the pack.

## Upgrading a workspace with its own instance schema

A workspace that installed an older pack, and then wrote its own
`gcp.#Instance` and `gcp-instances` model, can move to the pack:

1. `pudl example install gcp --global --overwrite`. Files from an older
   install are conflicts. Check each `.bak` against the new file before
   deleting it.
2. Remove the workspace's own `gcp.#Instance` and its `gcp-instances` model.
   The pack's `#GCPInstances` uses the same definition name, model name and
   observation scope, so both cannot load together. Checks that depend on
   the model name `gcp-instances` still resolve.
3. Run `pudl doctor --fix --only inference --schema gcp.#Instance --include-explicit`
   to move the stored instances to `gcp.#ComputeInstance`. Its projection
   sync adds `gcp_instance_nic` and the other new facts, so instances get
   subnet, firewall and service-account edges in `pudl graph export`.

Rehearsed on a copy of a workspace, then applied to that workspace:
`pudl doctor` passed, instance metadata values read back as `[REDACTED]`,
and an exposure rule built on `gcp_instance_external_ip` returned the same
rows before and after.

## Issues filed

- `pudl-gvh`: doctor's catalog scan reads `config.yaml` `data_path`, not the
  resolved workspace.
- `pudl-bpy`: doctor flags `.examples` as an unexpected entry.
- `pudl-qaw`: `example install` does not reproject facts on upgrade, and
  does not detect two schemas that claim one `resource_type`.
