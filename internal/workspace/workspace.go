package workspace

import (
	"fmt"
	"os"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// Workspace represents a discovered per-repo workspace.
type Workspace struct {
	Root              string              // absolute path to repo root (parent of .pudl/)
	PudlDir           string              // absolute path to .pudl/ directory
	Name              string              // workspace name from workspace.cue
	SchemaPath        string              // .pudl/schema/ (may not exist)
	DefinitionsPath   string              // .pudl/definitions/ (may not exist)
	ToolchainMappings []ToolchainOverride // per-repo toolchain overrides
}

// ToolchainOverride maps a schema prefix to a toolchain for this workspace.
type ToolchainOverride struct {
	Prefix    string
	Toolchain string
}

// Discover walks up from startDir looking for .pudl/workspace.cue.
// Returns nil if no workspace is found (global mode).
func Discover(startDir string) (*Workspace, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, err
	}

	for {
		candidate := filepath.Join(dir, ".pudl", "workspace.cue")
		if _, err := os.Stat(candidate); err == nil {
			return load(dir)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, nil // reached filesystem root, no workspace found
		}
		dir = parent
	}
}

// load parses workspace.cue and populates the Workspace struct.
func load(root string) (*Workspace, error) {
	pudlDir := filepath.Join(root, ".pudl")
	cuePath := filepath.Join(pudlDir, "workspace.cue")

	data, err := os.ReadFile(cuePath)
	if err != nil {
		return nil, fmt.Errorf("reading workspace.cue: %w", err)
	}

	ctx := cuecontext.New()
	val := ctx.CompileBytes(data)
	if val.Err() != nil {
		return nil, fmt.Errorf("parsing workspace.cue: %w", val.Err())
	}

	ws := &Workspace{
		Root:            root,
		PudlDir:         pudlDir,
		SchemaPath:      filepath.Join(pudlDir, "schema"),
		DefinitionsPath: filepath.Join(pudlDir, "definitions"),
	}

	// Extract name (optional, defaults to directory name)
	if name := val.LookupPath(cue.ParsePath("name")); name.Exists() {
		ws.Name, _ = name.String()
	} else {
		ws.Name = filepath.Base(root)
	}

	// Extract toolchain_mappings (optional)
	if mappings := val.LookupPath(cue.ParsePath("toolchain_mappings")); mappings.Exists() {
		iter, _ := mappings.List()
		for iter.Next() {
			v := iter.Value()
			prefix, _ := v.LookupPath(cue.ParsePath("prefix")).String()
			toolchain, _ := v.LookupPath(cue.ParsePath("toolchain")).String()
			if prefix != "" && toolchain != "" {
				ws.ToolchainMappings = append(ws.ToolchainMappings, ToolchainOverride{
					Prefix:    prefix,
					Toolchain: toolchain,
				})
			}
		}
	}

	return ws, nil
}
