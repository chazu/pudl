// Package bundle captures verified portable PUDL workspaces. A bundle stores
// history and authored configuration, never generated execution workspaces.
package bundle

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/artifacts"
	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/filelock"
	"github.com/chazu/pudl/internal/ingestprep"
	"github.com/klauspost/compress/zstd"
)

const Version = 1
const DefaultMaxBytes int64 = 16 << 30
const catalogPath = "data/sqlite/catalog.db"

type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
}
type Manifest struct {
	Version      int       `json:"version"`
	OriginalRoot string    `json:"original_root"`
	CreatedAt    time.Time `json:"created_at"`
	Files        []File    `json:"files"`
	Notes        []string  `json:"notes,omitempty"`
}

// Export snapshots SQLite and captures referenced evidence while reclamation
// is excluded. Authored files are checked for modification during capture.
func Export(ctx context.Context, root, destination string, maxBytes int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if maxBytes == 0 {
		maxBytes = DefaultMaxBytes
	}
	if maxBytes < 0 {
		return fmt.Errorf("bundle byte limit must be positive")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if absolute, err := filepath.Abs(destination); err != nil {
		return err
	} else if rel, err := filepath.Rel(root, absolute); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("bundle destination must be outside the captured workspace")
	}
	if _, err := config.LoadFrom(root); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(root), ".pudl-bundle-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	manifest := Manifest{Version: Version, OriginalRoot: root, CreatedAt: time.Now().UTC()}
	err = artifacts.WithLock(ctx, root, func() error {
		db, err := database.OpenCatalogDBReadOnly(root)
		if err != nil {
			return err
		}
		defer db.Close()
		dbCopy := filepath.Join(stage, filepath.FromSlash(catalogPath))
		if err := os.MkdirAll(filepath.Dir(dbCopy), 0o700); err != nil {
			return err
		}
		// Online backup captures committed WAL state and preserves tie ordering.
		if err := onlineSnapshot(ctx, db.DB(), dbCopy); err != nil {
			return fmt.Errorf("snapshot catalog: %w", err)
		}
		captured, err := database.OpenCatalogDBReadOnly(stage)
		if err != nil {
			return err
		}
		defer captured.Close()
		entries, err := captured.QueryEntriesContext(ctx, database.FilterOptions{}, database.QueryOptions{})
		if err != nil {
			return err
		}
		notes, err := normalizeLegacyMetadata(ctx, stage, entries.Entries)
		if err != nil {
			return err
		}
		manifest.Notes = notes
		issues, err := captured.VerifyPayloads(ctx)
		if err != nil {
			return err
		}
		if len(issues) > 0 {
			return fmt.Errorf("cannot bundle damaged payload %s: %s", issues[0].EntryID, issues[0].Error)
		}
		paths := map[string]bool{catalogPath: true}
		for _, entry := range entries.Entries {
			for _, path := range []string{entry.StoredPath, entry.MetadataPath} {
				if path == "" {
					continue
				}
				rel, err := relative(root, path)
				if err != nil {
					return err
				}
				paths[rel] = true
			}
		}
		for _, name := range []string{"config.yaml", "workspace.cue", "schema", "definitions", "models", "populators", "rules", "cue.mod"} {
			path := filepath.Join(root, name)
			if _, err := os.Lstat(path); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return err
			}
			if err := filepath.WalkDir(path, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.Type()&os.ModeSymlink != 0 {
					return fmt.Errorf("bundle cannot capture symlink %s", path)
				}
				if d.IsDir() {
					if d.Name() == ".git" {
						return filepath.SkipDir
					}
					return nil
				}
				info, err := d.Info()
				if err != nil {
					return err
				}
				if !info.Mode().IsRegular() {
					return fmt.Errorf("not a regular file: %s", path)
				}
				rel, err := relative(root, path)
				if err != nil {
					return err
				}
				paths[rel] = true
				return nil
			}); err != nil {
				return err
			}
		}
		ordered := make([]string, 0, len(paths))
		for path := range paths {
			ordered = append(ordered, path)
		}
		sort.Strings(ordered)
		total := int64(0)
		for _, rel := range ordered {
			if err := ctx.Err(); err != nil {
				return err
			}
			source := filepath.Join(root, filepath.FromSlash(rel))
			target := filepath.Join(stage, filepath.FromSlash(rel))
			if rel == catalogPath {
				source = target
			}
			checkRoot := root
			if rel == catalogPath {
				checkRoot = stage
			}
			if err := noSymlinks(checkRoot, source); err != nil {
				return err
			}
			info, err := os.Lstat(source)
			if err != nil {
				return fmt.Errorf("missing bundle evidence %s: %w", rel, err)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("bundle evidence is not a regular file: %s", rel)
			}
			total += info.Size()
			if total > maxBytes {
				return fmt.Errorf("bundle exceeds byte limit (%d)", maxBytes)
			}
			before, _, err := artifacts.HashContext(ctx, source)
			if err != nil {
				return err
			}
			if source != target {
				if err := copyFile(ctx, source, target, info.Size()); err != nil {
					return err
				}
			}
			hash, size, err := artifacts.HashContext(ctx, target)
			if err != nil {
				return err
			}
			after, _, err := artifacts.HashContext(ctx, source)
			if err != nil {
				return err
			}
			if before != hash || after != hash || size != info.Size() {
				return fmt.Errorf("file changed during bundle capture: %s", rel)
			}
			manifest.Files = append(manifest.Files, File{Path: rel, SHA256: hash, Size: size, Mode: uint32(info.Mode().Perm())})
		}
		return nil
	})
	if err != nil {
		return err
	}
	return writeArchive(ctx, stage, destination, manifest)
}

func writeArchive(ctx context.Context, stage, destination string, m Manifest) error {
	abs, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(abs), ".pudl-bundle-output-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	encoder, err := zstd.NewWriter(f)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(encoder)
	body, err := json.Marshal(m)
	if err != nil {
		_ = tw.Close()
		_ = encoder.Close()
		return err
	}
	err = tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o600, Size: int64(len(body))})
	if err == nil {
		_, err = tw.Write(body)
	}
	for _, item := range m.Files {
		if err != nil {
			break
		}
		if err = ctx.Err(); err != nil {
			break
		}
		err = tw.WriteHeader(&tar.Header{Name: item.Path, Mode: int64(item.Mode), Size: item.Size})
		if err != nil {
			break
		}
		var source *os.File
		source, err = os.Open(filepath.Join(stage, filepath.FromSlash(item.Path)))
		if err != nil {
			break
		}
		_, err = io.Copy(tw, &ingestprep.Reader{Context: ctx, Source: source, Remaining: item.Size})
		_ = source.Close()
	}
	if closeErr := tw.Close(); err == nil {
		err = closeErr
	}
	if closeErr := encoder.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), abs)
}

// Restore validates a bundle entirely in a sibling private directory and only
// publishes it into a previously absent destination after all checks succeed.
func Restore(ctx context.Context, source, root string, maxBytes int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if maxBytes == 0 {
		maxBytes = DefaultMaxBytes
	}
	if maxBytes < 0 {
		return fmt.Errorf("bundle byte limit must be positive")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(root); err == nil {
		return fmt.Errorf("restore destination already exists: %s", root)
	} else if !os.IsNotExist(err) {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(root), ".pudl-restore-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	m, err := extract(ctx, source, stage, maxBytes)
	if err != nil {
		return err
	}
	if err := rebaseAndVerify(ctx, stage, root, m); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Cooperating restores must serialize the final absence check and rename.
	lock, err := filelock.AcquireContext(ctx, filepath.Join(filepath.Dir(root), ".pudl-restore.lock"))
	if err != nil {
		return err
	}
	defer lock.Release()
	return func() error {
		if _, err := os.Lstat(root); err == nil {
			return fmt.Errorf("restore destination already exists: %s", root)
		} else if !os.IsNotExist(err) {
			return err
		}
		return os.Rename(stage, root)
	}()
}

func extract(ctx context.Context, source, stage string, maxBytes int64) (Manifest, error) {
	var m Manifest
	f, err := os.Open(source)
	if err != nil {
		return m, err
	}
	defer f.Close()
	z, err := zstd.NewReader(f, zstd.WithDecoderMaxMemory(64<<20))
	if err != nil {
		return m, err
	}
	defer z.Close()
	tr := tar.NewReader(z)
	header, err := tr.Next()
	if err != nil {
		return m, err
	}
	if header.Name != "manifest.json" || header.Typeflag != tar.TypeReg || header.Size < 0 || header.Size > 8<<20 {
		return m, fmt.Errorf("invalid bundle manifest header")
	}
	dec := json.NewDecoder(io.LimitReader(tr, header.Size))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return m, fmt.Errorf("invalid trailing manifest data")
	}
	if m.Version != Version {
		return m, fmt.Errorf("unsupported bundle version %d", m.Version)
	}
	if !filepath.IsAbs(m.OriginalRoot) {
		return m, fmt.Errorf("invalid original workspace root")
	}
	expected := map[string]File{}
	total := int64(0)
	for _, item := range m.Files {
		if !safePath(item.Path) || item.Size < 0 || len(item.SHA256) != 64 {
			return m, fmt.Errorf("invalid bundle entry %q", item.Path)
		}
		if _, ok := expected[item.Path]; ok {
			return m, fmt.Errorf("duplicate bundle entry %q", item.Path)
		}
		if item.Size > maxBytes-total {
			return m, fmt.Errorf("bundle exceeds byte limit (%d)", maxBytes)
		}
		total += item.Size
		expected[item.Path] = item
	}
	if _, ok := expected[catalogPath]; !ok {
		return m, fmt.Errorf("bundle has no catalog")
	}
	if _, ok := expected["config.yaml"]; !ok {
		return m, fmt.Errorf("bundle has no configuration")
	}
	seen := map[string]bool{}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return m, err
		}
		item, ok := expected[header.Name]
		if !ok || seen[header.Name] || header.Typeflag != tar.TypeReg || header.Size != item.Size {
			return m, fmt.Errorf("unexpected bundle member %q", header.Name)
		}
		path := filepath.Join(stage, filepath.FromSlash(item.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return m, err
		}
		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return m, err
		}
		n, copyErr := io.Copy(out, &ingestprep.Reader{Context: ctx, Source: tr, Remaining: item.Size})
		closeErr := out.Close()
		if copyErr != nil {
			return m, copyErr
		}
		if closeErr != nil {
			return m, closeErr
		}
		if n != item.Size {
			return m, fmt.Errorf("truncated bundle member %s", item.Path)
		}
		hash, _, err := artifacts.HashContext(ctx, path)
		if err != nil {
			return m, err
		}
		if hash != item.SHA256 {
			return m, fmt.Errorf("bundle checksum mismatch: %s", item.Path)
		}
		if err := os.Chmod(path, os.FileMode(item.Mode)&0o755); err != nil {
			return m, err
		}
		seen[item.Path] = true
	}
	if len(seen) != len(expected) {
		return m, fmt.Errorf("bundle is missing manifest entries")
	}
	// Drain the compressed stream: tar EOF can precede a truncated frame/checksum.
	if _, err := io.Copy(zeroPadding{}, &ingestprep.Reader{Context: ctx, Source: z, Remaining: maxBytes}); err != nil {
		return m, err
	}
	return m, nil
}

func rebaseAndVerify(ctx context.Context, stage, root string, m Manifest) error {
	db, err := database.OpenCatalogDBReadOnly(stage)
	if err != nil {
		return err
	}
	var integrity string
	err = db.DB().QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity)
	if err != nil || integrity != "ok" {
		_ = db.Close()
		return fmt.Errorf("bundle catalog integrity failed: %s (%v)", integrity, err)
	}
	versions, err := db.AppliedMigrationVersions()
	if err != nil {
		_ = db.Close()
		return err
	}
	for _, v := range versions {
		if v > database.LatestMigrationVersion() {
			_ = db.Close()
			return fmt.Errorf("bundle catalog schema %d is newer than supported %d", v, database.LatestMigrationVersion())
		}
	}
	entries, err := db.QueryEntriesContext(ctx, database.FilterOptions{}, database.QueryOptions{})
	_ = db.Close()
	if err != nil {
		return err
	}
	files := map[string]bool{}
	for _, file := range m.Files {
		files[file.Path] = true
	}
	for _, entry := range entries.Entries {
		for _, path := range []string{entry.StoredPath, entry.MetadataPath} {
			if path == "" {
				continue
			}
			rel, err := relative(m.OriginalRoot, path)
			if err != nil {
				return err
			}
			if !files[rel] {
				return fmt.Errorf("catalog references missing evidence: %s", rel)
			}
		}
	}
	// Migration compatibility is checked only against the private copy.
	writable, err := database.NewCatalogDB(stage)
	if err != nil {
		return err
	}
	defer writable.Close()
	tx, err := writable.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, entry := range entries.Entries {
		stored, metadata := entry.StoredPath, entry.MetadataPath
		if stored != "" {
			rel, _ := relative(m.OriginalRoot, stored)
			stored = filepath.Join(root, filepath.FromSlash(rel))
		}
		if metadata != "" {
			rel, _ := relative(m.OriginalRoot, metadata)
			metadata = filepath.Join(root, filepath.FromSlash(rel))
		}
		if _, err := tx.ExecContext(ctx, "UPDATE catalog_entries SET stored_path=?,metadata_path=?,status='unknown' WHERE id=?", stored, metadata, entry.ID); err != nil {
			return err
		}
	}
	for _, statement := range []string{
		"UPDATE run_approvals SET status='rejected',resolved_at=CURRENT_TIMESTAMP WHERE status='pending'",
		"UPDATE run_set_approvals SET status='rejected',resolved_at=CURRENT_TIMESTAMP WHERE status='pending'",
		"DELETE FROM snapshot_pins WHERE owner_kind='approval'",
		"INSERT OR REPLACE INTO snapshot_reuse_blocks(snapshot_id,reason) SELECT snapshot_id,'restored history requires live re-observation' FROM observe_snapshots",
		"UPDATE observe_snapshots SET retained=EXISTS(SELECT 1 FROM snapshot_pins p WHERE p.snapshot_id=observe_snapshots.snapshot_id AND (p.expires_at IS NULL OR julianday(p.expires_at)>julianday('now')))",
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	cfg, err := config.LoadFrom(stage)
	if err != nil {
		return err
	}
	cfg.DataPath = filepath.Join(root, "data")
	cfg.SchemaPath = filepath.Join(root, "schema")
	if err := cfg.SaveTo(stage); err != nil {
		return err
	}
	// Metadata storage paths are provenance-free; originals remain untouched.
	for _, entry := range entries.Entries {
		if entry.MetadataPath == "" {
			continue
		}
		rel, _ := relative(m.OriginalRoot, entry.MetadataPath)
		path := filepath.Join(stage, filepath.FromSlash(rel))
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var value map[string]any
		decoder := json.NewDecoder(strings.NewReader(string(body)))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil {
			continue
		}
		for _, key := range []string{"stored_path", "metadata_path"} {
			if original, ok := value[key].(string); ok {
				if rel, err := relative(m.OriginalRoot, original); err == nil {
					value[key] = filepath.Join(root, filepath.FromSlash(rel))
				}
			}
		}
		body, err = json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func relative(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if !safePath(rel) {
		return "", fmt.Errorf("bundle reference outside workspace: %s", path)
	}
	return rel, nil
}
func safePath(path string) bool {
	return path != "" && path != "." && !strings.Contains(path, "\\") && !strings.HasPrefix(path, "/") && filepath.ToSlash(filepath.Clean(path)) == path && path != ".." && !strings.HasPrefix(path, "../") && path != "manifest.json"
}
func copyFile(ctx context.Context, source, destination string, size int64) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, &ingestprep.Reader{Context: ctx, Source: in, Remaining: size})
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func noSymlinks(root, path string) error {
	rel, err := relative(root, path)
	if err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(rel, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("bundle reference uses symlink: %s", current)
		}
	}
	return nil
}

type zeroPadding struct{}

func (zeroPadding) Write(p []byte) (int, error) {
	for _, b := range p {
		if b != 0 {
			return 0, fmt.Errorf("unexpected nonzero data after bundle archive")
		}
	}
	return len(p), nil
}
