// Package muschemas implements pudl's local cache of CUE schemas
// referenced by mu plugins.
//
// Schemas are keyed by (module, version) and stored under
// <pudlDir>/schemas/<module>/<version>/<files...>. Versions are
// immutable once cached: an attempt to write differing content for an
// existing key returns ErrVersionMismatch. The cache is append-only —
// older versions stay so historical imports remain reclassifiable
// against the schema they were originally typed under.
//
// This is pudl's own cache. mu maintains a parallel cache at
// ~/.mu/schemas/. The two are not shared; the wire format
// (SchemaSidecar JSON, OCI plugin manifest) is what bridges them.
package muschemas

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNotFound is returned when a (module, version) is not cached.
var ErrNotFound = errors.New("muschemas: not found")

// ErrVersionMismatch is returned when an Insert tries to write content
// for an existing (module, version) that differs from what is already
// stored.
var ErrVersionMismatch = errors.New("muschemas: version content mismatch")

// Cache stores CUE schema modules on the local filesystem.
type Cache struct {
	root string
}

// File is one CUE source file destined for the cache. RelPath is
// slash-separated relative to the (module, version) directory.
type File struct {
	RelPath string
	Content []byte
}

// New returns a Cache rooted at the given directory, creating it if
// necessary. Pass <pudlDir>/schemas in normal use.
func New(root string) (*Cache, error) {
	if root == "" {
		return nil, errors.New("muschemas: root is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("muschemas: create root: %w", err)
	}
	return &Cache{root: root}, nil
}

// Root returns the on-disk root of the cache.
func (c *Cache) Root() string { return c.root }

func (c *Cache) versionDir(module, version string) string {
	return filepath.Join(c.root, filepath.FromSlash(module), version)
}

// Has reports whether the cache contains the given module and version.
func (c *Cache) Has(module, version string) bool {
	if err := validateKey(module, version); err != nil {
		return false
	}
	info, err := os.Stat(c.versionDir(module, version))
	return err == nil && info.IsDir()
}

// Files returns the slash-separated relative paths of every .cue file
// stored under (module, version), sorted lexicographically.
func (c *Cache) Files(module, version string) ([]string, error) {
	if err := validateKey(module, version); err != nil {
		return nil, err
	}
	dir := c.versionDir(module, version)
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	var rels []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".cue") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rels = append(rels, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(rels)
	return rels, nil
}

// Insert writes the supplied files under (module, version). If the
// version is already cached, byte-for-byte content match makes Insert
// a no-op; mismatch returns ErrVersionMismatch. Insert is atomic per
// call (temp dir + rename).
func (c *Cache) Insert(module, version string, files []File) error {
	if err := validateKey(module, version); err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("muschemas: at least one file required")
	}
	for _, f := range files {
		if err := validateRelPath(f.RelPath); err != nil {
			return fmt.Errorf("muschemas: file %q: %w", f.RelPath, err)
		}
		if !strings.HasSuffix(f.RelPath, ".cue") {
			return fmt.Errorf("muschemas: file %q: only .cue files are accepted", f.RelPath)
		}
	}

	target := c.versionDir(module, version)
	if c.Has(module, version) {
		return c.verifyMatches(target, files)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("muschemas: create module dir: %w", err)
	}

	tmp, err := os.MkdirTemp(filepath.Dir(target), ".tmp-"+version+"-*")
	if err != nil {
		return fmt.Errorf("muschemas: tmp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	for _, f := range files {
		dst := filepath.Join(tmp, filepath.FromSlash(f.RelPath))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("muschemas: mkdir: %w", err)
		}
		if err := os.WriteFile(dst, f.Content, 0o644); err != nil {
			return fmt.Errorf("muschemas: write: %w", err)
		}
	}
	if err := os.Rename(tmp, target); err != nil {
		if _, statErr := os.Stat(target); statErr == nil {
			return c.verifyMatches(target, files)
		}
		return fmt.Errorf("muschemas: rename: %w", err)
	}
	return nil
}

func (c *Cache) verifyMatches(dir string, files []File) error {
	want := make(map[string][]byte, len(files))
	for _, f := range files {
		want[f.RelPath] = f.Content
	}
	seen := make(map[string]bool, len(files))
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		relSlash := filepath.ToSlash(rel)
		seen[relSlash] = true
		expected, ok := want[relSlash]
		if !ok {
			return fmt.Errorf("%w: existing file %q not in incoming set", ErrVersionMismatch, relSlash)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytesEqual(got, expected) {
			return fmt.Errorf("%w: file %q content differs", ErrVersionMismatch, relSlash)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for rel := range want {
		if !seen[rel] {
			return fmt.Errorf("%w: incoming file %q missing from cached version", ErrVersionMismatch, rel)
		}
	}
	return nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func validateKey(module, version string) error {
	if module == "" {
		return errors.New("muschemas: module is required")
	}
	if version == "" {
		return errors.New("muschemas: version is required")
	}
	if strings.Contains(module, "..") || strings.HasPrefix(module, "/") {
		return fmt.Errorf("muschemas: invalid module %q", module)
	}
	if strings.ContainsAny(version, "/\\") || version == "." || version == ".." {
		return fmt.Errorf("muschemas: invalid version %q", version)
	}
	return nil
}

func validateRelPath(rel string) error {
	if rel == "" {
		return errors.New("relative path is required")
	}
	if strings.HasPrefix(rel, "/") {
		return fmt.Errorf("relative path %q must not be absolute", rel)
	}
	clean := filepath.ToSlash(filepath.Clean(rel))
	if clean != rel {
		return fmt.Errorf("relative path %q is not in canonical form", rel)
	}
	if strings.HasPrefix(clean, "../") || clean == ".." || strings.Contains(clean, "/../") {
		return fmt.Errorf("relative path %q escapes module dir", rel)
	}
	return nil
}

// ParseRef splits a sidecar/CLI schema reference into its components.
// Accepts:
//
//	"mu/aws@v1"
//	"mu/aws@v1#EC2Instance"
//
// Returns module, version, definition (definition may be empty).
func ParseRef(ref string) (module, version, definition string, err error) {
	at := strings.Index(ref, "@")
	if at < 0 {
		return "", "", "", fmt.Errorf("muschemas: ref %q missing @<version>", ref)
	}
	module = ref[:at]
	rest := ref[at+1:]
	if hash := strings.Index(rest, "#"); hash >= 0 {
		version = rest[:hash]
		definition = rest[hash:]
	} else {
		version = rest
	}
	if module == "" || version == "" {
		return "", "", "", fmt.Errorf("muschemas: ref %q missing module or version", ref)
	}
	return module, version, definition, nil
}
