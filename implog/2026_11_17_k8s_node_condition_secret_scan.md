# Kubernetes pack: Node condition types and reasons skip the secret scan

A live run of `k8s-nodes` against GKE clusters printed a likely-secret
warning (`status.conditions[*].type (high_entropy)`) for every Node record.
Node condition types and reasons are CamelCase constants, often
node-problem-detector names such as `FrequentContainerdRestart` or
`NoCorruptDockerOverlay2`. A name with a digit and many distinct letters
can reach the entropy heuristic's bar. These values are not secrets.

## Changes

- `packs/k8s/schema/k8s/nodes.cue` (`#Node`):
  `secret_scan_ignore: ["status.conditions[*].type", "status.conditions[*].reason"]`.
  `status.conditions[*].message` is free text and stays scanned.
- Fixture `packs/k8s/populators/k8s/nodes.json`: both Nodes carry GKE's full
  condition set (node-problem-detector conditions, `NetworkUnavailable`,
  the kubelet pressure conditions and `Ready`) with realistic reasons. The
  GKE names score below the entropy bar, so node `-0001` also has a
  `FrequentNTPv4ClockJump` condition. Its type and its reason
  `NoFrequentNTPv4ClockJump` are flagged without the ignore list.
- `cmd/k8s_pack_objects_test.go` (`TestK8sPackFixtureObjects`): the check
  output (stdout JSON and stderr) names no `status.conditions[*]` path, and
  the stored Node keeps all 13 conditions. Without the ignore list the test
  fails on both paths.

## Public API

- No new API. `k8s.#Node` gains `_pudl.secret_scan_ignore`.

## Verification

- `CGO_ENABLED=0 go test ./cmd -run TestK8sPack` passes, including
  `TestK8sPackHostileObjectsPlantNothing`.
- `make test` passes.
