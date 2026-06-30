// Package smoke contains end-to-end smoke tests for the convergence and
// cross-model dependency surface. The tests are gated behind the `smoke` build
// tag, so `go test ./...` compiles this package (via this untagged file) but
// runs no tests. Run the smoke tests explicitly:
//
//	go test -tags=smoke ./test/smoke/ -v -timeout 20m   # or: make smoke
//
// Each test skips cleanly when its tooling (docker/k3d/kubectl/mu/bb/jq) is
// absent and cleans up its clusters, containers, and temp dirs via t.Cleanup.
package smoke
