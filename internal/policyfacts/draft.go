package policyfacts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue/parser"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/policy"
)

// Draft is a policy file loaded beside the installed schemas without being
// installed: a what-if evaluation reads it and writes nothing.
type Draft struct {
	Evaluator *Evaluator
	// Policies are the policies the draft file declares, sorted by name. A
	// draft policy shadows an installed policy of the same name.
	Policies []*policy.Policy
	dir      string
}

// Close removes the draft's temporary schema root.
func (d *Draft) Close() {
	if d != nil && d.dir != "" {
		_ = os.RemoveAll(d.dir)
	}
}

// LoadDraft loads file into a temporary schema root searched before
// schemaPaths. The file keeps its namespace: its directory relative to the
// schema path holding it, else its CUE package name. It must be standalone
// (policies name their type through _pudl.base_schema).
func LoadDraft(file string, schemaPaths []string) (_ *Draft, resultErr error) {
	src, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	parsed, err := parser.ParseFile(file, src, parser.PackageClauseOnly)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", file, err)
	}
	if parsed.PackageName() == "" {
		return nil, fmt.Errorf("%s has no package clause", file)
	}
	d := &Draft{}
	defer func() {
		if resultErr != nil {
			d.Close()
		}
	}()
	if d.dir, err = os.MkdirTemp("", "pudl-policy-draft-"); err != nil {
		return nil, err
	}
	namespace := draftNamespace(file, parsed.PackageName(), schemaPaths)
	target := filepath.Join(d.dir, namespace, strings.TrimSuffix(filepath.Base(file), ".cue")+".cue")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(target, src, 0o600); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(d.dir, "cue.mod"), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(d.dir, "cue.mod", "module.cue"), []byte(draftModule), 0o600); err != nil {
		return nil, err
	}
	inferrer, err := inference.NewSchemaInferrer(append([]string{d.dir}, schemaPaths...)...)
	if err != nil {
		return nil, err
	}
	for _, le := range inferrer.LoadErrors() {
		if le.Path == d.dir {
			return nil, fmt.Errorf("draft %s does not load: %v", file, le.Err)
		}
	}
	d.Evaluator = New(inferrer)
	for _, le := range d.Evaluator.LoadErrors() {
		if source, _ := inferrer.SchemaSource(le.Policy); source == d.dir {
			return nil, le
		}
	}
	for _, p := range d.Evaluator.Set().Policies() {
		if source, _ := inferrer.SchemaSource(p.Name); source == d.dir {
			d.Policies = append(d.Policies, p)
		}
	}
	if len(d.Policies) == 0 {
		return nil, fmt.Errorf("draft %s declares no policy (a definition with _pudl.schema_type: \"policy\")", file)
	}
	return d, nil
}

// Select returns the named draft policies; a name the draft does not declare
// is an error.
func (d *Draft) Select(names []string) ([]*policy.Policy, error) {
	var out []*policy.Policy
	for _, name := range names {
		p, ok := d.Evaluator.lookup(name)
		if !ok || !containsPolicy(d.Policies, p) {
			return nil, fmt.Errorf("the draft declares no policy %q", name)
		}
		out = append(out, p)
	}
	return out, nil
}

func containsPolicy(policies []*policy.Policy, p *policy.Policy) bool {
	for _, q := range policies {
		if q == p {
			return true
		}
	}
	return false
}

// draftNamespace is the package directory the draft's schemas are named by.
func draftNamespace(file, pkg string, schemaPaths []string) string {
	abs, err := filepath.Abs(file)
	if err != nil {
		return pkg
	}
	for _, root := range schemaPaths {
		if root, err = filepath.Abs(root); err != nil {
			continue
		}
		rel, err := filepath.Rel(root, filepath.Dir(abs))
		if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return pkg
}

// draftModule is the draft root's module file. Schema names come from package
// directories, not the module path, and a draft imports nothing.
const draftModule = "module: \"pudl.schemas\"\nlanguage: version: \"v0.14.0\"\n"
