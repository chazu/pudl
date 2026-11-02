package uiserver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// FileTreeStamp fingerprints the .cue files under dirs by path, size and
// modification time. Missing directories contribute nothing. It is cheap
// enough to call on every poll.
func FileTreeStamp(dirs ...string) string {
	var lines []string
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".cue") {
				return nil
			}
			if info, err := d.Info(); err == nil {
				lines = append(lines, fmt.Sprintf("%s|%d|%d", path, info.Size(), info.ModTime().UnixNano()))
			}
			return nil
		})
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:8])
}
