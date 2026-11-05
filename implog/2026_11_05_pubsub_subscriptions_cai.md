# Pub/Sub subscriptions move to Cloud Asset Inventory (2026-11-05)

Issue: `pudl-q4v.10` (epic `pudl-q4v`).

## Live parity check (paths and counts only)

CAI `pubsub.googleapis.com/Subscription` across the organization, then
`gcloud pubsub subscriptions list` per project (one project refused the
list). On those in both: every shared value equal; only gcloud has `state`
and `topicMessageRetentionDuration`.
Neither is used by a fact, policy or check.

## Changes

- `_gcpAssetTypes`: `pubsub-subscriptions` → `pubsub.googleapis.com/Subscription`,
  so `gcp-pubsub-subscriptions` searches CAI first and falls back to gcloud.
- `_gcpShapes["pubsub-subscriptions"]`: also drops `state` and
  `topicMessageRetentionDuration`, so both sources store identical records.
- Fixture `populators/gcp/cai/pubsub-subscriptions.json` (the gcloud fixture
  as CAI search results).
- `cmd/gcp_pack_assets_test.go`: subscriptions in `gcpAssetTypes`; the CAI
  then gcloud fixture runs store nothing new the second time.
- `packs/gcp/README.md` and the GCP design doc's findings updated.

## Tests

- `CGO_ENABLED=0 go test ./...` passes; `make lint` 0 issues.
