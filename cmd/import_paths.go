package cmd

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/importer"
)

// resolveFilePaths turns --path into absolute file paths. A wildcard pattern
// expands to its matching regular files; a directory expands to the supported
// data files directly inside it, or beneath it when recursive is set; anything
// else names one file.
func resolveFilePaths(pathPattern string, recursive bool) ([]string, error) {
	if containsWildcard(pathPattern) {
		return resolveWildcard(pathPattern)
	}

	info, err := os.Stat(pathPattern)
	if os.IsNotExist(err) {
		return nil, errors.NewFileNotFoundError(pathPattern)
	}
	if err == nil && info.IsDir() {
		return expandDirectory(pathPattern, recursive)
	}

	absPath, err := filepath.Abs(pathPattern)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeFileSystem, "Failed to get absolute path", err)
	}
	return []string{absPath}, nil
}

func resolveWildcard(pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeInvalidInput, "Invalid wildcard pattern", err)
	}

	var filePaths []string
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil || !info.Mode().IsRegular() {
			continue // only accessible regular files
		}
		absPath, err := filepath.Abs(match)
		if err != nil {
			continue
		}
		filePaths = append(filePaths, absPath)
	}
	return filePaths, nil
}

// expandDirectory lists the supported data files in dir, in lexical order.
// Hidden files and directories are skipped, so a workspace's own .pudl tree is
// never re-imported by a recursive import of its parent.
func expandDirectory(dir string, recursive bool) ([]string, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, errors.WrapError(errors.ErrCodeFileSystem, "Failed to get absolute path", err)
	}

	var filePaths []string
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		hidden := strings.HasPrefix(d.Name(), ".") && path != root
		if d.IsDir() {
			if path != root && (hidden || !recursive) {
				return filepath.SkipDir
			}
			return nil
		}
		if !hidden && d.Type().IsRegular() && importer.IsImportableName(d.Name()) {
			filePaths = append(filePaths, path)
		}
		return nil
	})
	if walkErr != nil {
		return nil, errors.WrapError(errors.ErrCodeFileSystem, "Failed to read directory "+dir, walkErr)
	}
	sort.Strings(filePaths)
	return filePaths, nil
}

// containsWildcard checks if a path contains wildcard characters
func containsWildcard(path string) bool {
	return strings.ContainsAny(path, "*?[]")
}
