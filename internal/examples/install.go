// Package examples installs bundled examples into initialized PUDL workspaces.
//
// Upgrades: every install records the digest of each file it wrote in a
// manifest (<workspace>/.examples/<name>.json). On the next install, a file
// that differs from the new version but still matches its recorded digest
// was never edited and is upgraded in place. A file the user edited, or one
// installed by a pudl that kept no manifest, is a conflict: nothing is
// written unless Overwrite is set, which keeps the old file as <path>.bak.
package examples

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Options controls an install.
type Options struct {
	// Overwrite replaces conflicting files, keeping each as <path>.bak.
	Overwrite bool
}

// Result says what an install did with each file of the example.
type Result struct {
	Files     []string          `json:"files"`
	Added     []string          `json:"added"`
	Upgraded  []string          `json:"upgraded"`
	Replaced  []string          `json:"replaced"`
	Unchanged []string          `json:"unchanged"`
	Backups   map[string]string `json:"backups,omitempty"`
}

// ConflictError lists files that differ from the example and were not
// written.
type ConflictError struct {
	Example string
	Paths   []string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%d %s file(s) differ from this version and were left as they are:\n  %s\n"+
		"They were edited, or installed by an earlier pudl that kept no record of what it wrote. "+
		"To replace them, keeping your versions as <file>.bak, run: pudl example install %s --overwrite",
		len(e.Paths), e.Example, strings.Join(e.Paths, "\n  "), e.Example)
}

// Install writes example name into the workspace at root. All checks run
// before anything is written.
func Install(root, name string, opts Options) (*Result, error) {
	files, err := packFiles(name)
	if err != nil {
		return nil, err
	}
	manifest, err := readManifest(root, name)
	if err != nil {
		return nil, err
	}
	res := &Result{Added: []string{}, Upgraded: []string{}, Replaced: []string{}, Unchanged: []string{}}
	for path := range files {
		res.Files = append(res.Files, path)
	}
	sort.Strings(res.Files)

	var conflicts []string
	for _, path := range res.Files {
		if err := checkParents(root, filepath.Dir(path)); err != nil {
			return nil, err
		}
		info, err := os.Lstat(filepath.Join(root, path))
		switch {
		case os.IsNotExist(err):
			res.Added = append(res.Added, path)
			continue
		case err != nil:
			return nil, err
		case !info.Mode().IsRegular():
			return nil, fmt.Errorf("example destination is not a regular file: %s", path)
		}
		current, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			return nil, err
		}
		switch {
		case bytes.Equal(current, files[path]):
			res.Unchanged = append(res.Unchanged, path)
		case manifest[path] != "" && manifest[path] == digest(current):
			res.Upgraded = append(res.Upgraded, path)
		case opts.Overwrite:
			res.Replaced = append(res.Replaced, path)
		default:
			conflicts = append(conflicts, path)
		}
	}
	if len(conflicts) > 0 {
		return nil, &ConflictError{Example: name, Paths: conflicts}
	}

	res.Backups = map[string]string{}
	for _, path := range res.Replaced {
		backup, err := backupFile(root, path)
		if err != nil {
			return nil, err
		}
		res.Backups[path] = backup
	}
	for _, list := range [][]string{res.Added, res.Upgraded, res.Replaced} {
		for _, path := range list {
			if err := writeFile(filepath.Join(root, path), files[path]); err != nil {
				return nil, err
			}
		}
	}
	if err := writeManifest(root, name, files); err != nil {
		return nil, err
	}
	return res, nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// backupFile copies path to the first free <path>.bak[.N]. The suffix is not
// .cue, so CUE never loads a backup.
func backupFile(root, path string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		return "", err
	}
	backup := path + ".bak"
	for n := 1; ; n++ {
		if _, err := os.Lstat(filepath.Join(root, backup)); os.IsNotExist(err) {
			break
		}
		backup = fmt.Sprintf("%s.bak.%d", path, n)
	}
	return backup, os.WriteFile(filepath.Join(root, backup), data, 0o644)
}

// Refuse symlinked destinations so example installation stays in its workspace.
func checkParents(root, relative string) error {
	for relative != "." {
		info, err := os.Lstat(filepath.Join(root, relative))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && !info.IsDir() {
			return fmt.Errorf("example destination is not a directory: %s", relative)
		}
		relative = filepath.Dir(relative)
	}
	return nil
}
