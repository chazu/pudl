package workspace

import (
	"os"
	"path/filepath"

	"pudl/internal/config"
)

// Context holds the resolved workspace state for the current invocation.
type Context struct {
	// Workspace is non-nil when running inside a per-repo workspace.
	Workspace *Workspace

	// GlobalPudlDir is always ~/.pudl/
	GlobalPudlDir string

	// EffectiveOrigin is the workspace name (if in workspace) or "global"
	EffectiveOrigin string

	// SchemaSearchPaths is the ordered list of schema directories to search.
	// Per-repo first, then global, then embedded.
	SchemaSearchPaths []string

	// DefinitionSearchPaths is the ordered list of definition directories.
	DefinitionSearchPaths []string
}

// NewContext discovers the workspace and builds the resolved context.
func NewContext() (*Context, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	globalDir := config.GetPudlDir()
	ws, err := Discover(cwd)
	if err != nil {
		return nil, err
	}

	return buildContext(ws, globalDir), nil
}

// buildContext constructs a Context from a workspace (possibly nil) and a global dir.
func buildContext(ws *Workspace, globalDir string) *Context {
	ctx := &Context{
		Workspace:     ws,
		GlobalPudlDir: globalDir,
	}

	if ws != nil {
		ctx.EffectiveOrigin = ws.Name
		ctx.SchemaSearchPaths = []string{ws.SchemaPath, filepath.Join(globalDir, "schema")}
		ctx.DefinitionSearchPaths = []string{ws.DefinitionsPath, filepath.Join(globalDir, "schema", "definitions")}
	} else {
		ctx.EffectiveOrigin = "global"
		ctx.SchemaSearchPaths = []string{filepath.Join(globalDir, "schema")}
		ctx.DefinitionSearchPaths = []string{filepath.Join(globalDir, "schema", "definitions")}
	}

	return ctx
}
