package init

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// gitWithoutIdentity makes git behave like a fresh CI runner: no global or
// system config, and no guessing an identity from the host.
func gitWithoutIdentity(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "user.useConfigOnly")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "EMAIL"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

func TestInitialCommitWorksWithoutAGitIdentity(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	gitWithoutIdentity(t)
	dir := t.TempDir()
	require.NoError(t, initGitRepo(dir, false, io.Discard))

	out, err := exec.Command("git", "-C", dir, "log", "-1", "--format=%an <%ae>").Output()
	require.NoError(t, err)
	require.Equal(t, "pudl <pudl@localhost>", strings.TrimSpace(string(out)))
}

func TestInitialCommitKeepsAConfiguredIdentity(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	gitWithoutIdentity(t)
	t.Setenv("GIT_CONFIG_COUNT", "3")
	t.Setenv("GIT_CONFIG_KEY_1", "user.name")
	t.Setenv("GIT_CONFIG_VALUE_1", "Ada")
	t.Setenv("GIT_CONFIG_KEY_2", "user.email")
	t.Setenv("GIT_CONFIG_VALUE_2", "ada@example.com")
	dir := t.TempDir()
	require.NoError(t, initGitRepo(dir, false, io.Discard))

	out, err := exec.Command("git", "-C", dir, "log", "-1", "--format=%an <%ae>").Output()
	require.NoError(t, err)
	require.Equal(t, "Ada <ada@example.com>", strings.TrimSpace(string(out)))
}
