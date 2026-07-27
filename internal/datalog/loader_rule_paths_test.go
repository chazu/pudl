package datalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A search path that is not a readable directory contributes no rules and is not
// an error. A workspace legitimately may not have a rules dir yet, and a path
// that exists but is a file is the same nothing from the caller's point of view.
// Tolerating both here is what lets every caller share one unfiltered search
// order instead of filtering its own.
func TestLoadRulesFromPaths_NonDirectoryPathsAreSkipped(t *testing.T) {
	dir := t.TempDir()

	rules := filepath.Join(dir, "rules")
	require.NoError(t, os.MkdirAll(rules, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rules, "r.cue"),
		[]byte("package rules\n\nr: {\n\thead: {rel: \"r\", args: {x: \"$X\"}}\n\tbody: [{rel: \"s\", args: {x: \"$X\"}}]\n}\n"), 0o644))

	notADir := filepath.Join(dir, "not-a-dir")
	require.NoError(t, os.WriteFile(notADir, []byte("this is a file"), 0o644))

	missing := filepath.Join(dir, "missing")

	loaded, err := LoadRulesFromPaths(missing, notADir, rules)
	require.NoError(t, err)
	assert.Len(t, loaded, 1)
}
