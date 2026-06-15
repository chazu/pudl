package importer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chazu/pudl/internal/inference"
)

// gitSchemaDir copies the embedded bootstrap schemas into a temp module so the
// inferrer can load the built-in git family end-to-end.
func gitSchemaDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	modDir := filepath.Join(root, "cue.mod")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("mkdir cue.mod: %v", err)
	}
	mod := "language: version: \"v0.16.0\"\n\nmodule: \"pudl.schemas@v0\"\n\nsource: kind: \"self\"\n"
	if err := os.WriteFile(filepath.Join(modDir, "module.cue"), []byte(mod), 0644); err != nil {
		t.Fatalf("write module.cue: %v", err)
	}
	if err := CopyBootstrapSchemas(root); err != nil {
		t.Fatalf("CopyBootstrapSchemas: %v", err)
	}
	// The bootstrap `definitions/` tree carries a stale import of the removed
	// pudl/model package and fails to load; it holds definition specs, not
	// schemas, so it is irrelevant to schema inference. Drop it so the loader
	// sees a clean schema module.
	if err := os.RemoveAll(filepath.Join(root, "definitions")); err != nil {
		t.Fatalf("remove definitions: %v", err)
	}
	return root
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// TestGitFamilyRegistration verifies the bootstrap git family loads as schemas
// while the inline #GitRemote/#GitBranch components do not (D1), and that the
// platform specializations inherit identity_fields ["name"] unchanged.
func TestGitFamilyRegistration(t *testing.T) {
	inferrer, err := inference.NewSchemaInferrer(gitSchemaDir(t))
	if err != nil {
		t.Fatalf("NewSchemaInferrer: %v", err)
	}

	schemas := inferrer.GetAvailableSchemas()

	for _, want := range []string{
		"pudl/git.#GitRepository",
		"pudl/git.#GitHubRepository",
		"pudl/git.#GitLabRepository",
	} {
		if !contains(schemas, want) {
			t.Errorf("expected schema %q to be registered; got %v", want, schemas)
		}
	}
	for _, component := range []string{"pudl/git.#GitRemote", "pudl/git.#GitBranch"} {
		if contains(schemas, component) {
			t.Errorf("component %q must not be registered as a schema", component)
		}
	}

	// Family-identity invariant: specializations inherit identity ["name"].
	graph := inferrer.GetInheritanceGraph()
	for _, child := range []string{"pudl/git.#GitHubRepository", "pudl/git.#GitLabRepository"} {
		parent, ok := graph.GetParent(child)
		if !ok || parent != "pudl/git.#GitRepository" {
			t.Errorf("%s: expected parent pudl/git.#GitRepository, got %q (ok=%v)", child, parent, ok)
		}
		meta, ok := inferrer.GetSchemaMetadata(child)
		if !ok {
			t.Fatalf("no metadata for %s", child)
		}
		if len(meta.IdentityFields) != 1 || meta.IdentityFields[0] != "name" {
			t.Errorf("%s: identity_fields must be [name], got %v", child, meta.IdentityFields)
		}
	}
}

// TestGitFamilyInference verifies a github.com / gitlab.com / local repository
// blob classifies to the right member of the family.
func TestGitFamilyInference(t *testing.T) {
	inferrer, err := inference.NewSchemaInferrer(gitSchemaDir(t))
	if err != nil {
		t.Fatalf("NewSchemaInferrer: %v", err)
	}

	cases := []struct {
		label string
		data  map[string]interface{}
		want  string
	}{
		{
			label: "github",
			data: map[string]interface{}{
				"name":           "github.com/chazu/pudl",
				"default_branch": "main",
				"owner":          "chazu",
				"visibility":     "public",
			},
			want: "pudl/git.#GitHubRepository",
		},
		{
			label: "gitlab",
			data: map[string]interface{}{
				"name":           "gitlab.com/group/project",
				"default_branch": "main",
				"namespace":      "group",
			},
			want: "pudl/git.#GitLabRepository",
		},
		{
			label: "local",
			data: map[string]interface{}{
				"name":           "/home/me/dev/widget",
				"default_branch": "trunk",
			},
			want: "pudl/git.#GitRepository",
		},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			res, err := inferrer.Infer(tc.data, inference.InferenceHints{})
			if err != nil {
				t.Fatalf("Infer: %v", err)
			}
			if res.Schema != tc.want {
				t.Errorf("got schema %q (confidence %.2f, reason %q), want %q",
					res.Schema, res.Confidence, res.Reason, tc.want)
			}
		})
	}
}
