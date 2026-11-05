# Makefile for pudl

BINARY_NAME := pudl

# Build flags
GO := go
GOFLAGS := -v

# Version info embedded into pudl/cmd via -X ldflags.
# Mirrors the values goreleaser injects so `make`-built binaries report a real version.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/chazu/pudl/cmd.version=$(VERSION) \
	-X github.com/chazu/pudl/cmd.commit=$(COMMIT) \
	-X github.com/chazu/pudl/cmd.date=$(DATE)

INSTALL_PATH := $(shell $(GO) env GOBIN)
ifeq ($(strip $(INSTALL_PATH)),)
INSTALL_PATH := $(shell $(GO) env GOPATH)/bin
endif


.PHONY: web check-web
.PHONY: all build install uninstall clean clean-local test fuzz test-git-walkthrough release snapshot bench bench-cpu bench-mem bench-save bench-compare lint vulncheck test-race coverage ci generate check-skills docs check-docs

all: build

build:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .

install: build
	@mkdir -p $(INSTALL_PATH)
	install -m 755 $(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME)

uninstall:
	rm -f $(INSTALL_PATH)/$(BINARY_NAME)

clean:
	rm -f $(BINARY_NAME)
	$(GO) clean

# Remove large local caches left behind by smoke and walkthrough runs. Only
# ignored runtime state under .pudl/data/ is touched.
clean-local:
	rm -rf .pudl/data/kick-tires .pudl/data/smoke .pudl/data/git-walkthrough

test:
	$(GO) test ./...

# Execute the getting-started guide's command blocks against a fresh workspace,
# run the example's observation journey, and install examples and packs.
# Needs git; no mu or network.
test-git-walkthrough:
	@command -v git >/dev/null 2>&1 || (echo "git is required for test-git-walkthrough" && exit 1)
	CGO_ENABLED=0 $(GO) test -tags=smoke ./test/smoke/ -run '^TestSmoke_(GitInventoryWalkthrough|GitInventoryObservation|ExampleInstall.*|PackInstall.*)$$' -v -count=1 -timeout 5m

# Sync embedded skill copies (internal/skills/files/*.md) from their canonical
# sources in skills/<name>/SKILL.md.
generate:
	$(GO) generate ./...

# Verify the embedded skill copies are in sync with their sources without
# writing anything. Fails if `make generate` needs to be run and committed.
check-skills:
	$(GO) run ./internal/skills/gen -check

# Regenerate docs/cli-reference.md from the command tree and docs/cli/notes/.
docs:
	$(GO) run ./internal/clidocs/gen

# Rebuild the web graph explorer bundle embedded by `pudl ui`
# (internal/webui/static, committed). Only needed after editing web/; building
# pudl itself never needs Node.
web:
	cd web && npm ci && npm run build

# Verify the committed UI bundle matches web/src. Needs Node.
check-web: web
	git diff --exit-code -- internal/webui/static || (echo "internal/webui/static is stale: run make web and commit it" && exit 1)
	@test -z "$$(git ls-files --others --exclude-standard internal/webui/static)" || (echo "internal/webui/static has uncommitted new files: run make web and commit it" && exit 1)

# Verify docs/cli-reference.md is current without writing it. Fails if
# `make docs` needs to be run and committed.
check-docs:
	$(GO) run ./internal/clidocs/gen -check

# Every gated end-to-end smoke test, against a freshly built binary in
# sandboxed workspaces. They clean up after themselves.
smoke:
	CGO_ENABLED=0 $(GO) test -tags=smoke ./test/smoke/ -v -timeout 20m

release:
	goreleaser release --clean

snapshot:
	goreleaser release --snapshot --clean

# Run each native fuzz target for FUZZTIME. `go test -fuzz` accepts one target
# per invocation, so targets are listed as package:Target pairs. New crashers
# land in the package's testdata/fuzz and should be committed with the fix.
FUZZTIME ?= 30s
FUZZ_TARGETS := \
	./internal/importer:FuzzDecodeJSONMatchesStdlib \
	./internal/importer:FuzzDecodeYAMLMatchesYAMLv3 \
	./internal/importer:FuzzDecodeCSVVerbatim \
	./internal/importer:FuzzDetectFormat \
	./internal/idgen:FuzzCanonicalJSON \
	./internal/schemaname:FuzzNormalizeIdempotent \
	./internal/datalog:FuzzParseRulesCompile \
	./internal/ingestprep:FuzzArrayMatchesJSON

fuzz:
	@set -e; for t in $(FUZZ_TARGETS); do \
		pkg=$${t%%:*}; name=$${t##*:}; \
		echo "fuzz $$name ($$pkg, $(FUZZTIME))"; \
		CGO_ENABLED=0 $(GO) test -run='^$$' -fuzz="^$$name$$" -fuzztime=$(FUZZTIME) $$pkg; \
	done

bench:
	$(GO) test -bench=. -benchmem ./...

bench-cpu:
	$(GO) test -bench=. -benchmem -cpuprofile=cpu.prof ./...
	go tool pprof -http=:8080 cpu.prof

bench-mem:
	$(GO) test -bench=. -benchmem -memprofile=mem.prof ./...
	go tool pprof -http=:8080 mem.prof

bench-save:
	mkdir -p benchmarks
	$(GO) test -bench=. -benchmem ./... > benchmarks/$(shell date +%Y%m%d_%H%M%S).txt

bench-compare:
	@command -v benchstat >/dev/null 2>&1 || (echo "benchstat not found. Install with: go install golang.org/x/perf/cmd/benchstat@latest" && exit 1)
	@if [ -z "$(BASELINE)" ]; then echo "Usage: make bench-compare BASELINE=benchmarks/baseline.txt"; exit 1; fi
	$(GO) test -bench=. -benchmem ./... | benchstat $(BASELINE) -

# Linters run through `go run` at pinned versions, so local runs and CI use the
# same build (a separately installed golangci-lint may predate the Go toolchain).
GOLANGCI_LINT_VERSION := v2.14.0
GOVULNCHECK_VERSION := v1.8.0

lint:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

vulncheck:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

# The 300-seed differential oracle is intentionally comprehensive. Pure-Go
# SQLite under race/checkptr instrumentation needs a longer bound on macOS;
# limiting package concurrency also avoids contention in cancellation tests.
test-race:
	$(GO) test -race -p 2 -timeout 20m ./...

coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

ci: check-skills check-docs lint test-race coverage
	@echo "All CI checks passed!"
