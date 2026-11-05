package fileset

import "io/fs"

// TreeFiles returns every file of fsys keyed by its path in fsys, for a
// file set laid out as it is installed.
func TreeFiles(fsys fs.FS) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, path)
		files[path] = data
		return err
	})
	return files, err
}
