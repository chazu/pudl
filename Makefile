# Makefile for pudl

BINARY_NAME := pudl
INSTALL_PATH := /usr/local/bin

# Build flags
GO := go
GOFLAGS := -v
LDFLAGS := -s -w

.PHONY: all build install uninstall clean test release snapshot bench bench-cpu bench-mem bench-save bench-compare lint test-race coverage ci

all: build

build:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .

install: build
	install -m 755 $(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME)

uninstall:
	rm -f $(INSTALL_PATH)/$(BINARY_NAME)

clean:
	rm -f $(BINARY_NAME)
	$(GO) clean

test:
	$(GO) test ./...

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

ci: lint test-race coverage
	@echo "All CI checks passed!"

