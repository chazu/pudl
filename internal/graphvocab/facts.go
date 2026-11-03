package graphvocab

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/graphspec"
)

// facts are the vocabulary read from ground facts in rule files.
type facts struct {
	categories    []Category
	edgeTypes     []EdgeType
	providers     []Provider
	resourceTypes []ResourceType
	accountTypes  []accountType
	problems      []string
}

// accountType is a graph_account_type fact: provider's `account` attribute
// names a resource of type.
type accountType struct {
	provider, typ, source string
}

// vocabularyArgs are each vocabulary relation's arguments.
var vocabularyArgs = map[string][]string{
	CategoryRelation:     {"category", "label", "meaning", "order"},
	EdgeTypeRelation:     {"type", "category", "label", "meaning"},
	ProviderRelation:     {"provider", "label"},
	ResourceTypeRelation: {"type", "code", "label", "class"},
	AccountTypeRelation:  {"provider", "type"},
}

// readFacts reads the vocabulary relations' ground facts, in load order.
// The first declaration of a key wins; a different redeclaration is a
// problem. Vocabulary relations take facts only, so a rule with a body is a
// problem too (its tuples would have no source to show).
func readFacts(rules []datalog.Rule) facts {
	var f facts
	seen := map[string]map[string]string{} // relation -> key -> source
	declared := map[string]map[string]string{}
	for _, r := range rules {
		want, ok := vocabularyArgs[r.Head.Rel]
		if !ok || !r.Valid() {
			continue
		}
		src := r.Describe()
		if len(r.Body) > 0 {
			f.problems = append(f.problems, fmt.Sprintf("%s: %s takes facts only (a rule with no body)", src, r.Head.Rel))
			continue
		}
		args, err := stringArgs(r, want)
		if err != nil {
			f.problems = append(f.problems, fmt.Sprintf("%s: %v", src, err))
			continue
		}
		key := args[want[0]]
		if seen[r.Head.Rel] == nil {
			seen[r.Head.Rel], declared[r.Head.Rel] = map[string]string{}, map[string]string{}
		}
		sig := strings.Join(values(args, want), "\x00")
		if prev, dup := seen[r.Head.Rel][key]; dup {
			if declared[r.Head.Rel][key] != sig {
				f.problems = append(f.problems, fmt.Sprintf("%s: %s %q is already declared differently by %s; using that", src, r.Head.Rel, key, prev))
			}
			continue
		}
		seen[r.Head.Rel][key], declared[r.Head.Rel][key] = src, sig
		if err := f.add(r.Head.Rel, args, src); err != nil {
			f.problems = append(f.problems, fmt.Sprintf("%s: %v", src, err))
		}
	}
	sort.SliceStable(f.categories, func(i, j int) bool { return f.categories[i].Order < f.categories[j].Order })
	sort.Slice(f.edgeTypes, func(i, j int) bool { return f.edgeTypes[i].Type < f.edgeTypes[j].Type })
	sort.Slice(f.providers, func(i, j int) bool { return f.providers[i].Provider < f.providers[j].Provider })
	return f
}

func (f *facts) add(rel string, a map[string]string, src string) error {
	switch rel {
	case CategoryRelation:
		order, err := strconv.Atoi(a["order"])
		if err != nil {
			return fmt.Errorf("%s: order %q is not an integer", rel, a["order"])
		}
		f.categories = append(f.categories, Category{ID: a["category"], Label: a["label"], Meaning: a["meaning"], Order: order, Source: src})
	case EdgeTypeRelation:
		f.edgeTypes = append(f.edgeTypes, EdgeType{Type: a["type"], Category: a["category"], Label: a["label"], Meaning: a["meaning"], Source: src})
	case ProviderRelation:
		f.providers = append(f.providers, Provider{Provider: a["provider"], Label: a["label"], Source: src})
	case ResourceTypeRelation:
		d := graphspec.Display{Code: a["code"], Label: a["label"], Class: a["class"]}
		if err := graphspec.CheckDisplay(d); err != nil {
			return fmt.Errorf("%s %q: %w", rel, a["type"], err)
		}
		f.resourceTypes = append(f.resourceTypes, ResourceType{Type: a["type"], Code: d.Code, Label: d.Label, Class: d.Class, Source: src})
	case AccountTypeRelation:
		if ProviderOf(a["type"]) != a["provider"] {
			return fmt.Errorf("%s: type %q does not belong to provider %q", rel, a["type"], a["provider"])
		}
		f.accountTypes = append(f.accountTypes, accountType{provider: a["provider"], typ: a["type"], source: src})
	}
	return nil
}

// stringArgs reads a fact's arguments as strings: exactly the wanted keys,
// each a non-empty scalar.
func stringArgs(r datalog.Rule, want []string) (map[string]string, error) {
	out := map[string]string{}
	for key := range r.Head.Args {
		if !contains(want, key) {
			return nil, fmt.Errorf("%s has no argument %q; arguments are %s", r.Head.Rel, key, strings.Join(want, ", "))
		}
	}
	for _, key := range want {
		term, ok := r.Head.Args[key]
		if !ok {
			return nil, fmt.Errorf("%s needs argument %q", r.Head.Rel, key)
		}
		s := scalar(term.Value)
		if s == "" {
			return nil, fmt.Errorf("%s argument %q is empty", r.Head.Rel, key)
		}
		out[key] = s
	}
	return out, nil
}

func scalar(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	}
	return ""
}

func values(m map[string]string, keys []string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = m[k]
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
