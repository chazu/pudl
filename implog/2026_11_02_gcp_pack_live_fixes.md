# GCP pack fixes from a live run

Running every live GCP pack collector against a real organization turned
up two pack bugs.

## Disabled APIs that gcloud reports without SERVICE_DISABLED

`empty_when: ["SERVICE_DISABLED"]` matched compute, DNS, SQL and the other
APIs, but not GKE or KMS. Their errors carry no reason code:

```
ERROR: (gcloud.container.clusters.list) ResponseError: code=403, message=Kubernetes Engine API has not been used in project P before or it is disabled. ...
ERROR: (gcloud.kms.keyrings.list) PERMISSION_DENIED: Google Cloud KMS API has not been used in project P before or it is disabled. ...
```

So every project without GKE or KMS was a failed input, the collectors
failed, and the data-protection and GKE-posture families were blocked.

`_#GCPLive` (examples/gcp/schema/models/gcp_pack.cue) now sets:

```cue
empty_when: [
	"SERVICE_DISABLED",
	"API has not been used in project [^ ]+ before or it is disabled",
]
```

A project with billing disabled (`FAILED_PRECONDITION: Billing is disabled
for project P`) is deliberately still a failed input: resources created
before billing lapsed can still exist, so it is not a definite "none".

## Cloud Run services with labels beyond the location

`gcp.#RunService` declared `metadata.labels` as a closed struct with only
`"cloud.googleapis.com/location"`, so a service with any other label failed
the type. Cloud Functions 2nd gen services carry
`goog-cloudfunctions-runtime` and similar, and with the collector's strict
schema one such record failed the whole populate. `labels` is now open.

## Tests

`cmd/gcp_pack_live_test.go`: the fake gcloud fails KMS commands in
`proj-nocompute` with the KMS wording (no SERVICE_DISABLED);
`TestGCPPackLiveCollectorsTreatADisabledAPIAsEmpty` asserts `gcp-kms-keyrings`
and `gcp-kms-keys` (through the helper script) list it as an empty input
matched by the new pattern.

No CLI, JSON or Go API changes.
