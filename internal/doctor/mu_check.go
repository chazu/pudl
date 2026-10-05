package doctor

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/proc"
)

// MinMuVersion is the oldest mu this PUDL is tested against. CI installs
// exactly this version (MU_VERSION in .github/workflows/go.yml); keep the two
// in step.
const MinMuVersion = "v0.3.5"

// muVersionTimeout bounds `mu version`, which should answer instantly.
const muVersionTimeout = 10 * time.Second

// CheckMu reports whether the mu on PATH is present and recent enough. mu is
// optional — imports, queries and --from-catalog replays work without it — so
// a missing or old mu is a warning, not an error. Finding out here is better
// than finding out halfway through a converge run.
func CheckMu(ctx context.Context) *CheckResult {
	return CheckMuBinary(ctx, "mu")
}

// CheckMuBinary checks a specific mu binary (a name on PATH or a path).
func CheckMuBinary(ctx context.Context, binary string) *CheckResult {
	install := "go install github.com/chazu/mu/cmd/mu@" + MinMuVersion
	if !proc.Available(binary) {
		return &CheckResult{
			Status:  "warning",
			Message: "mu not found on PATH",
			Details: "imports, queries and --from-catalog runs work without mu; live model runs and --converge need it",
			Fix:     install,
		}
	}

	out, err := proc.Output(ctx, muVersionTimeout, binary, "version")
	if err != nil {
		return &CheckResult{
			Status:  "warning",
			Message: "Could not read the mu version",
			Details: err.Error(),
			Fix:     install,
		}
	}
	version, ok := parseMuVersion(string(out))
	if !ok {
		return &CheckResult{
			Status:  "warning",
			Message: "Unrecognized `mu version` output",
			Details: strings.TrimSpace(string(out)),
			Fix:     install,
		}
	}
	if compareVersions(version, MinMuVersion) < 0 {
		return &CheckResult{
			Status:  "warning",
			Message: fmt.Sprintf("mu %s is older than the minimum supported %s", version, MinMuVersion),
			Details: "plan and manifest formats may not match what this PUDL expects",
			Fix:     install,
		}
	}
	return &CheckResult{Status: "ok", Message: "mu " + version}
}

// parseMuVersion extracts the version from `mu version` output ("mu v0.3.5").
func parseMuVersion(output string) (string, bool) {
	for _, field := range strings.Fields(output) {
		if strings.HasPrefix(field, "v") {
			if _, ok := versionParts(field); ok {
				return field, true
			}
		}
	}
	return "", false
}

// compareVersions orders vMAJOR.MINOR.PATCH versions numerically. A
// pre-release suffix (v0.4.0-rc1) sorts before its release.
func compareVersions(a, b string) int {
	pa, _ := versionParts(a)
	pb, _ := versionParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			if pa[i] < pb[i] {
				return -1
			}
			return 1
		}
	}
	preA, preB := strings.Contains(a, "-"), strings.Contains(b, "-")
	switch {
	case preA && !preB:
		return -1
	case !preA && preB:
		return 1
	}
	return 0
}

// versionParts parses vMAJOR.MINOR.PATCH[-pre][+build].
func versionParts(v string) ([3]int, bool) {
	var parts [3]int
	core := strings.TrimPrefix(v, "v")
	if core == v {
		return parts, false
	}
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		core = core[:i]
	}
	fields := strings.Split(core, ".")
	if len(fields) != 3 {
		return parts, false
	}
	for i, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return parts, false
		}
		parts[i] = n
	}
	return parts, true
}
