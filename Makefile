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

.PHONY: all build install uninstall clean test release snapshot bench bench-cpu bench-mem bench-save bench-compare lint test-race coverage ci generate check-skills

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

test:
	$(GO) test ./...

# Sync embedded skill copies (internal/skills/files/*.md) from their canonical
# sources in skills/<name>/SKILL.md.
generate:
	$(GO) generate ./...

# Verify the embedded skill copies are in sync with their sources without
# writing anything. Fails if `make generate` needs to be run and committed.
check-skills:
	$(GO) run ./internal/skills/gen -check

# Gated end-to-end smoke tests (convergence + cross-model deps). They skip
# cleanly when docker/k3d/kubectl/mu/bb/jq are missing and clean up after
# themselves. Set PUDL_SMOKE_MU_ROOT to point the infra tests at a mu project.
smoke:
	CGO_ENABLED=0 $(GO) test -tags=smoke ./test/smoke/ -v -timeout 20m

release:
	goreleaser release --clean

snapshot:
	goreleaser release --snapshot --clean

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

lint:
	@command -v golangci-lint >/dev/null 2>&1 || (echo "golangci-lint not found. Install from https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run ./...

test-race:
	$(GO) test -race ./...

coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

ci: check-skills lint test-race coverage
	@echo "All CI checks passed!"

