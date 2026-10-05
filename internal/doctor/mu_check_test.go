package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func fakeMu(t *testing.T, versionOutput string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mu")
	script := "#!/bin/sh\nif [ \"$1\" = version ]; then echo '" + versionOutput + "'; exit 0; fi\nexit 2\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckMu(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name, output, status, message string
	}{
		{"current", "mu " + MinMuVersion, "ok", "mu " + MinMuVersion},
		{"newer", "mu v0.4.1", "ok", "mu v0.4.1"},
		{"older", "mu v0.3.4", "warning", "mu v0.3.4 is older than the minimum supported " + MinMuVersion},
		{"older minor", "mu v0.2.9", "warning", "mu v0.2.9 is older than the minimum supported " + MinMuVersion},
		{"prerelease of minimum", "mu v0.3.5-rc1", "warning", "mu v0.3.5-rc1 is older than the minimum supported " + MinMuVersion},
		{"garbage", "mu development build", "warning", "Unrecognized `mu version` output"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := CheckMuBinary(ctx, fakeMu(t, tc.output))
			assert.Equal(t, tc.status, result.Status)
			assert.Equal(t, tc.message, result.Message)
		})
	}
}

func TestCheckMuAbsentIsAWarning(t *testing.T) {
	result := CheckMuBinary(context.Background(), filepath.Join(t.TempDir(), "no-such-mu"))
	assert.Equal(t, "warning", result.Status, "mu is optional for imports and replays")
	assert.Equal(t, "mu not found on PATH", result.Message)
	assert.Contains(t, result.Fix, MinMuVersion)
}

func TestCompareVersions(t *testing.T) {
	assert.Equal(t, 0, compareVersions("v1.2.3", "v1.2.3"))
	assert.Equal(t, -1, compareVersions("v1.2.3", "v1.10.0"))
	assert.Equal(t, 1, compareVersions("v2.0.0", "v1.99.99"))
	assert.Equal(t, -1, compareVersions("v1.0.0-rc1", "v1.0.0"))
}
