package argocdpack

import (
	"os"
	"path/filepath"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"github.com/stretchr/testify/require"
)

// repoIDs evaluates _argocdRepoID on each URL with the pack's own model
// files, as installed.
func repoIDs(t *testing.T, urls []string) map[string]string {
	t.Helper()
	dir := t.TempDir()
	files := []string{"argocd_shape.cue", "argocd_repository.cue"}
	for _, f := range files {
		raw, err := Files.ReadFile("schema/models/" + f)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), raw, 0o644))
	}
	// argocd_pack.cue, which defines _argocdKept, imports the systemmodel
	// module; the test stands in for it.
	test := "package models\n\n_argocdKept: {_ns: string, ok: true}\nurls: [...string]\nids: {for u in urls {(u): (_argocdRepoID & {_url: u}).out}}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "t.cue"), []byte(test), 0o644))
	insts := load.Instances(append(files, "t.cue"), &load.Config{Dir: dir})
	require.NoError(t, insts[0].Err)
	ctx := cuecontext.New()
	v := ctx.BuildInstance(insts[0]).FillPath(cue.ParsePath("urls"), urls)
	require.NoError(t, v.Err())
	var out map[string]string
	require.NoError(t, v.LookupPath(cue.ParsePath("ids")).Decode(&out))
	return out
}

// TestRepoIDOneRepositoryPerForm checks that the https, ssh and scp forms
// of one repository are one repository: one argocd.#Repository record, one
// git.repo key, one graph node.
func TestRepoIDOneRepositoryPerForm(t *testing.T) {
	const repo = "https://github.com/example-org/example-repo"
	forms := []string{
		"git@github.com:example-org/example-repo.git",
		"https://github.com/example-org/example-repo",
		"https://GitHub.com/example-org/example-repo/",
		"https://github.com/example-org/example-repo.git",
		"ssh://git@github.com/example-org/example-repo.git",
		"ssh://git@github.com:22/example-org/example-repo",
		"git+ssh://git@GITHUB.COM/example-org/example-repo.git/",
		"https://user:token@github.com:443/example-org/example-repo.git",
	}
	ids := repoIDs(t, forms)
	for _, f := range forms {
		require.Equal(t, repo, ids[f], f)
	}
}

// TestRepoIDKeepsWhatNamesAnotherRepository checks the parts the
// normalization keeps: the path's case, another port, another scheme, and
// no scheme for a registry path.
func TestRepoIDKeepsWhatNamesAnotherRepository(t *testing.T) {
	cases := map[string]string{
		"https://github.com/Example-Org/Repo":      "https://github.com/Example-Org/Repo",
		"https://git.example.com:8443/org/repo":    "https://git.example.com:8443/org/repo",
		"http://git.example.com:80/org/repo?x=1":   "http://git.example.com/org/repo",
		"oci://Registry.example.com/charts":        "oci://registry.example.com/charts",
		"Registry.example.com/charts/":             "registry.example.com/charts",
		"https://charts.example.com/stable#frag":   "https://charts.example.com/stable",
		"ssh://git:secret@git.example.com/o/r.git": "https://git.example.com/o/r",
	}
	var urls []string
	for u := range cases {
		urls = append(urls, u)
	}
	ids := repoIDs(t, urls)
	for u, want := range cases {
		require.Equal(t, want, ids[u], u)
	}
}
