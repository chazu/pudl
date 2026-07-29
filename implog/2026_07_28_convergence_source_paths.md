# Convergence source paths and durable errors

## Outcome

Closed the stocktake failure where catalog-installed plugins could observe a
desired manifest but convergence failed before apply because `mu.cue` sources
were interpreted from the extracted plugin bundle directory. Reconcile
workspaces now render absolute manifest sources within the mu project root.
Mu's action resolver accepts those contained absolute input paths while keeping
the existing project-root escape check.

Convergence errors are copied into the run report before the report is rendered
or persisted, so `pudl run report <run-id> --json` retains the terminal error.

## Verification

- PUDL focused source-path and durable-error regressions: pass.
- Mu absolute-input and path-traversal resolver tests: pass.
- PUDL: `go test ./...`, `go vet ./...`, `go build ./...`, generated-skill
  check, `git diff --check`, and `br lint`: pass.
- Mu: `go test ./...`, `go vet ./...`, `go build ./...`, and `git diff --check`:
  pass.
- Installed k8s end-to-end convergence and approval resume no longer report
  missing `desired_0.json`; the fixture reaches the existing cap because its
  fake cluster intentionally does not change after apply. Durable reports
  include the cap/verification error.

## Boundary

This slice repairs reconcile execution and reporting only. It does not begin
cross-model value wiring or change the typed projection/validation boundary.
