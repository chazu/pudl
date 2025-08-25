# PUDL Build & Style Guide

## Commands
- **Build**: `./build.sh` or `go build -o pudl .`
- **Test**: `go test ./...` (single test: `go test -run TestName`)
- **Lint**: `golangci-lint run` or `go vet ./...`
- **Format**: `go fmt ./...`
- **Run**: `./pudl process example.cue`

## Code Style
- **Imports**: stdlib → 3rd party → local (separated by blank lines)
- **Error handling**: Always check errors with `if err != nil { return fmt.Errorf("context: %w", err) }`
- **Naming**: CamelCase for public, camelCase for private, acronyms uppercase (e.g., CUEProcessor)
- **Types**: Interfaces for extensibility (e.g., CustomFunction interface)
- **Comments**: All exported types/functions must have doc comments
- **Struct tags**: Only when necessary for JSON/API

## Project Structure
- **Main**: CLI entry point in main.go (delegates to Cobra)
- **Commands**: Cobra CLI commands in cmd/ directory
- **Internal**: Core logic in internal/ packages
  - **CUE Processor**: AST processing in internal/cue/processor.go
- **Functions**: Custom CUE operations in op/functions.go