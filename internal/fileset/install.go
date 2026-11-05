// Package fileset installs sets of embedded files (tutorial examples and
// content packs) into initialized PUDL workspaces.
//
// Upgrades: every install records the digest of each file it wrote in a
// manifest (<workspace>/<Kind.RecordDir>/<name>.json). On the next install, a
// file that differs from the new version but still matches its recorded
// digest was never edited and is upgraded in place. A file the user edited,
// or one installed without a record, is a conflict: nothing is written unless
// Overwrite is set, which keeps the old file as <path>.bak.
package fileset

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

// Kind is a family of file sets: what the CLI calls one and where installs of
// it are recorded.
type Kind struct {
	// Noun names a file set in messages and its install command: pudl <Noun> install.
	Noun string
	// RecordDir is the workspace-relative directory of install records.
	RecordDir string
}

// Options controls an install.
type Options struct {
	// Overwrite replaces conflicting files, keeping each as <path>.bak.
	Overwrite bool
}

// Result says what an install did with each file of the file set.
type Result struct {
	Files     []string          `json:"files"`
	Added     []string          `json:"added"`
	Upgraded  []string          `json:"upgraded"`
	Replaced  []string          `json:"replaced"`
	Unchanged []string          `json:"unchanged"`
	Backups   map[string]string `json:"backups,omitempty"`
}

// ConflictError lists files that differ from the file set and were not
// written.
type ConflictError struct {
	Kind  Kind
	Name  string
	Paths []string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%d %s file(s) differ from this version and were left as they are:\n  %s\n"+
		"They were edited, or installed without a record of what was written. "+
		"To replace them, keeping your versions as <file>.bak, run: pudl %s install %s --overwrite",
		len(e.Paths), e.Name, strings.Join(e.Paths, "\n  "), e.Kind.Noun, e.Name)
}

// Install writes file set name, whose files are keyed by workspace-relative
// path, into the workspace at root. All checks run before anything is written.
func Install(root string, kind Kind, name string, files map[string][]byte, opts Options) (*Result, error) {
	manifest, err := readManifest(root, kind, name)
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
		if err := checkParents(root, kind, filepath.Dir(path)); err != nil {
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
			return nil, fmt.Errorf("%s destination is not a regular file: %s", kind.Noun, path)
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
		return nil, &ConflictError{Kind: kind, Name: name, Paths: conflicts}
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
	if err := writeManifest(root, kind, name, files); err != nil {
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

// Refuse symlinked destinations so installation stays in its workspace.
func checkParents(root string, kind Kind, relative string) error {
	for relative != "." {
		info, err := os.Lstat(filepath.Join(root, relative))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && !info.IsDir() {
			return fmt.Errorf("%s destination is not a directory: %s", kind.Noun, relative)
		}
		relative = filepath.Dir(relative)
	}
	return nil
}
