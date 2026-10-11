package workspace

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// dependencyRoots validates explicitly vendored definition packages. Paths are
// relative to .pudl, so moving a checkout preserves its dependency set.
func dependencyRoots(pudlDir string, paths []string) ([]string, error) {
	var roots []string
	seen := map[string]bool{}
	for _, path := range paths {
		clean := filepath.Clean(path)
		if filepath.IsAbs(path) || !strings.HasPrefix(clean, "vendor"+string(filepath.Separator)) {
			return nil, fmt.Errorf("definition dependency %q must be a package directory under .pudl/vendor", path)
		}
		root := filepath.Join(pudlDir, clean)
		if seen[root] {
			return nil, fmt.Errorf("duplicate definition dependency %q", path)
		}
		seen[root] = true
		info, err := os.Stat(filepath.Join(root, "schema"))
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("definition dependency %q is missing its schema directory; vendor it before running PUDL", path)
		}
		// Reject symlinks in parents as well as the package: a vendored package
		// must not silently acquire definitions from a different home directory.
		for dir := root; dir != pudlDir; dir = filepath.Dir(dir) {
			info, err := os.Lstat(dir)
			if err != nil {
				return nil, err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("definition dependency %q traverses a symlink", path)
			}
		}
		err = filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("definition dependency %q contains symlink %s", path, name)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		roots = append(roots, root)
	}
	return roots, nil
}
