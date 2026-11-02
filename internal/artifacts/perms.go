package artifacts

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Catalog data — raw payloads, import metadata, observe snapshots, the SQLite
// catalog, staging and journal files — is owner-only: a record can carry a
// secret that redaction missed, and other local users must not read it. Files
// users share or edit (schemas, rules, models, config, docs) keep their usual
// modes and are not written through these helpers.
const (
	PrivateFileMode fs.FileMode = 0o600
	PrivateDirMode  fs.FileMode = 0o700
)

// WriteFilePrivate writes data to path, creating it owner-only. Like
// os.WriteFile, it leaves the mode of an existing file unchanged.
func WriteFilePrivate(path string, data []byte) error {
	return os.WriteFile(path, data, PrivateFileMode)
}

// CreatePrivate creates or truncates path for reading and writing, creating it
// owner-only.
func CreatePrivate(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, PrivateFileMode)
}

// MkdirPrivate creates dir and any missing parents owner-only. Existing
// directories keep their modes; `pudl doctor --fix --only data-permissions`
// tightens those.
func MkdirPrivate(dir string) error {
	return os.MkdirAll(dir, PrivateDirMode)
}

// CreateFileExclPrivate creates an empty owner-only file at path unless one
// already exists. It is how the SQLite catalog gets its mode: SQLite creates a
// new database 0644 (less umask) and gives its -wal and -shm files the
// database file's mode.
func CreateFileExclPrivate(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, PrivateFileMode)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return f.Close()
}

// LoosePath is a file or directory under a data tree that users other than
// the owner can access.
type LoosePath struct {
	Path string
	Mode fs.FileMode
}

// FindLoosePaths walks dataDir and returns the regular files and directories
// whose group or other permission bits are set. Symbolic links are neither
// followed nor reported. A missing dataDir, or a path removed during the walk
// (such as a finished import's temporary file), has nothing loose.
func FindLoosePaths(dataDir string) ([]LoosePath, error) {
	var loose []LoosePath
	err := filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && !d.Type().IsRegular() {
			return nil
		}
		var info fs.FileInfo
		if err == nil {
			info, err = d.Info()
		}
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			loose = append(loose, LoosePath{Path: path, Mode: perm})
		}
		return nil
	})
	return loose, err
}

// Tightened is p's mode without group and other bits.
func (p LoosePath) Tightened() fs.FileMode { return p.Mode & 0o700 }

// Tighten removes the group and other permission bits from p, keeping the
// owner's.
func Tighten(p LoosePath) error {
	return os.Chmod(p.Path, p.Tightened())
}
