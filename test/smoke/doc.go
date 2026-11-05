// Package smoke contains end-to-end smoke tests that drive a freshly built
// pudl binary: the Git walkthrough, example and pack installation, observation, and
// command-surface checks. The tests are gated behind the `smoke` build tag, so
// `go test ./...` compiles this package (via this untagged file) but runs no
// tests. Run them explicitly:
//
//	make test-git-walkthrough # the getting-started walkthrough, examples and packs
//	make smoke                # every smoke test
//
// Each test works in a sandboxed workspace under .pudl/data and cleans up via
// t.Cleanup. None needs network access or external services.
package smoke
