# GCP pack: batch (d) — build and deploy types (pudl-q4v.7)

Date: 2026-11-09

Fourth of the six batches of new GCP types in pudl-q4v.7 (design:
`docs/design/2026-11-03-gcp-cloud-asset-collectors.md` §5, §7). Batches
(e)–(f) remain.

## What was added

Six types collected from Cloud Asset Inventory (CAI), five with a gcloud
fallback per project:

| Collector | CAI asset type | gcloud fallback | Schema | Identity |
| --- | --- | --- | --- | --- |
| `gcp-build-triggers` | `cloudbuild.googleapis.com/BuildTrigger` | `builds triggers list --region=L` per `_gcpBuildLocations` (partial) | `gcp.#BuildTrigger` | `project`, `id` |
| `gcp-appengine-apps` | `appengine.googleapis.com/Application` | `app describe` | `gcp.#AppEngineApplication` | `name` |
| `gcp-appengine-services` | `appengine.googleapis.com/Service` | none (CAI only) | `gcp.#AppEngineService` | `name` |
| `gcp-appengine-versions` | `appengine.googleapis.com/Version` | `app versions list`, unwrapped | `gcp.#AppEngineVersion` | `name` |
| `gcp-functions` | `cloudfunctions.googleapis.com/Function` | `functions list` | `gcp.#CloudFunction` | `name` |
| `gcp-run-domain-mappings` | `run.googleapis.com/DomainMapping` | `beta run domain-mappings list --region=R` per `_gcpRunDomainMappingRegions` (partial) | `gcp.#RunDomainMapping` | project number, location, domain |

Each has a `gcp-fixture-*` twin reading `populators/gcp/cai/<type>.json`
and, except App Engine services, `populators/gcp/<type>.json`.

Files: `schema/gcp/deploy.cue`, `schema/models/gcp_deploy.cue`, CAI and
gcloud fixtures. Shared changes:

- `_gcpAssetTransform` (`gcp_assets.cue`) passes the asset to `_gcpSkip` as
  `_asset`.
- `_gcpGcloudRecord` (`gcp_assets.cue`): a type's gcloud transform can
  unwrap the API object from gcloud's own record first (App Engine
  versions).
- Live settings `_gcpBuildLocations` (default `["global"]`) and
  `_gcpRunDomainMappingRegions` (default `["us-central1"]`).
- `_gcpEmptyWhen` matches "has not been used in project P before or it is
  disabled" without "API" (Cloud Build words it so), and App Engine's "does
  not contain an App Engine application" and "Apps instance [P] not found".

App Engine services have no fallback: `gcloud app services list` omits the
traffic split and ingress setting, and `app services describe` needs each
service's ID, a second level of enumeration a command source lacks.

### Shapes

- Build triggers: `location`, read from the CAI asset name (neither source
  always prints `resourceName`); a gcloud run sets the location it listed.
- Cloud Functions: `location`, read from the name; gcloud's `satisfiesPzi`
  dropped.
- App Engine: the legacy `//appengine.googleapis.com/apps/...` copy CAI
  also exports is dropped (`_gcpAppEngineLegacy`).
- Maps redacted whole get a sorted list of their names:
  `substitutionNames`, `envVariableNames`, `buildEnvVariableNames`,
  `serviceConfig.environmentVariableNames`,
  `buildConfig.environmentVariableNames`.

### Redaction

- Build triggers: `substitutions`, and an inline build's `substitutions`,
  step `args`, `env`, `script`, `options.env`, `secrets[*].secretEnv` and
  `availableSecrets.inline[*].envMap`.
- App Engine: the application's `iap.oauth2ClientSecret`; a version's
  `envVariables` and `buildEnvVariables`.
- Cloud Functions: service and build `environmentVariables`, and the signed
  `sourceUploadUrl`. Secret references are kept.
- Domain mappings: creator/modifier e-mail annotations, as for Cloud Run
  services.

### Facts (public relations)

- `gcp_build_trigger(project, id, name, disabled, service_account)`; the
  service account as an e-mail, "" when unset
- `gcp_appengine_app(project, name, location, serving_status)`
- `gcp_appengine_service(project, name, ingress)`
- `gcp_appengine_version(project, name, service, runtime, env,
  serving_status)`
- `gcp_function(project, name, environment, state, runtime, ingress,
  service_account)`; `gcp_function_run_service(service)` when the function
  runs as a Cloud Run service
- `gcp_run_domain_mapping(project, domain, location, route)`

No graph edges yet.

## Verification

- Live, in a scratch workspace (nothing committed): all six collectors
  succeeded; record counts equal CAI's assets
  after the legacy App Engine copies are dropped; a rerun stored no new
  records; every stored substitution and environment map is
  `[REDACTED]`; doctor clean (graph vocabulary complete, no likely secrets
  outside schema coverage).
- Live parity: field paths and values compared per type, then end to end:
  a gcloud run (`_gcpAssetScope: "off"`) after CAI stored no new records,
  and CAI after gcloud stored none either. Projects without the API or an
  App Engine application are empty inputs; the remaining failed inputs are
  projects the caller may not read.
- Fixtures (synthetic): three triggers (GitHub push with substitutions,
  disabled manual trigger without `resourceName` or service account,
  regional inline build), an application with IAP and its legacy copy, two
  services and two versions (standard with env variables, flexible with
  build env variables) plus legacy copies, a 2nd gen function with env and
  secret references and a 1st gen one with a signed upload URL, and a
  domain mapping.
- Tests: `cmd/gcp_pack_deploy_test.go` (`TestGCPPackDeployTypes`: counts,
  no fixture secret anywhere in storage, name lists, locations, facts, and
  the partial trigger fallback keeping the regional trigger); the six types
  in `gcpPackTypes` and `gcpPackSchemas`; App Engine apps and versions,
  functions and domain mappings in `TestGCPPackAssetSourcesStoreIdenticalRecords`;
  triggers, App Engine services and versions in `gcpPackReshaped`. `CGO_ENABLED=0 go test ./...`
  passes.

## Deviation from the design

§1 plans a gcloud fallback per type. App Engine services have none (above),
and the trigger and domain mapping fallbacks are partial. The design doc's
"Findings during delivery" records these and the App Engine duplicate
copies.

## Follow-ups

- A command source with a second level of enumeration would give App
  Engine services a fallback (`app services describe` per service).
- Graph edges (functions to their Cloud Run service, domain mappings to
  their route, triggers to their service account).
