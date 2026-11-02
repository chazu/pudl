package repair

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/chazu/pudl/internal/artifacts"
)

// dataPermissions tightens catalog data that other local users can read. Raw
// payloads, metadata, snapshots and the SQLite catalog are written owner-only
// (0600 files, 0700 directories); data written before that, or copied in with
// a permissive umask, keeps group or other bits until this repair removes
// them. It touches only the workspace's data/ tree, never schemas, rules,
// models or config, which users share and edit.
var dataPermissions = Repair{
	Name:      "data-permissions",
	Condition: "data files or directories that group or other users can access",
	Diagnose: func(ctx context.Context, env *Env) (Result, error) {
		loose, err := artifacts.FindLoosePaths(dataDir(env))
		if err != nil {
			return Result{}, err
		}
		return dataPermissionsResult(loose, 0, nil), nil
	},
	Apply: func(ctx context.Context, env *Env, dryRun bool) (Result, error) {
		loose, err := artifacts.FindLoosePaths(dataDir(env))
		if err != nil || dryRun {
			return dataPermissionsResult(loose, 0, nil), err
		}
		changed := 0
		var failures []string
		for _, p := range loose {
			if err := artifacts.Tighten(p); err != nil {
				failures = append(failures, err.Error())
				continue
			}
			changed++
		}
		return dataPermissionsResult(loose, changed, failures), nil
	},
}

func dataDir(env *Env) string { return filepath.Join(env.DB.Root(), "data") }

func dataPermissionsResult(loose []artifacts.LoosePath, changed int, failures []string) Result {
	result := Result{Pending: len(loose), Changed: changed, Failed: len(failures), Errors: failures}
	for _, p := range loose {
		result.Changes = append(result.Changes, Change{Subject: p.Path, From: fmt.Sprintf("%04o", p.Mode), To: fmt.Sprintf("%04o", p.Tightened())})
	}
	switch {
	case len(loose) == 0:
		result.Message = "Data files and directories are owner-only"
	case changed > 0:
		result.Message = fmt.Sprintf("Removed group and other access from %d data paths", changed)
	default:
		result.Message = fmt.Sprintf("%d data files or directories are accessible to group or other users", len(loose))
	}
	return result
}
