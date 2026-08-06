// Package smoke contains end-to-end smoke tests for repository-local run-sets,
// convergence, and cross-model dependencies. The tests are gated behind the
// `smoke` build tag, so `go test ./...` compiles this package (via this untagged
// file) but runs no tests. Run the smoke tests explicitly:
//
//	make test-kick-tires # real mu, repository-local fixtures
//	make smoke          # complete external-tools/infrastructure suite
//
// Each test cleans up its clusters, containers, and workspaces via t.Cleanup.
// The generic smoke target skips cases whose tools are absent; the named
// kick-the-tires target requires git, mu, and python3.
package smoke
