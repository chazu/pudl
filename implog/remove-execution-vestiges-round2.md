# Remove execution vestiges: bootstrap schemas/methods, SearchArtifacts

Second round of cleanup removing execution-layer remnants extracted to mu.

## Removed

- `internal/importer/bootstrap/pudl/model/` — CUE schemas for `#Model`, `#Method`, `#Socket`, `#AuthConfig`, `#QualificationResult`, `#ModelMetadata`, plus example models
- `internal/importer/bootstrap/methods/` — Glojure `.clj` method implementations (ec2_instance/list, ec2_instance/valid_credentials, simple/get_value)
- `SearchArtifacts()` and `ArtifactFilters` from `catalog_artifacts.go` — no callers after data CLI commands were removed
- `TestSearchArtifacts` and `TestSearchArtifactsDoesNotReturnImports` from test file

## Kept

- `GetLatestArtifact()` and `GetLatestArtifactByOrigin()` — actively used by drift checker
- `setupArtifactTestDB` and `addTestArtifact` helpers — used by remaining GetLatest tests
