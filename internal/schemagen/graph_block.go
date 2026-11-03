package schemagen

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/chazu/pudl/internal/graphspec"
)

// Field names tried, in order, for the graph block's labels.
var (
	graphNameFields    = []string{"name", "Name", "displayName", "DisplayName", "title"}
	graphAccountFields = []string{"project", "projectId", "AccountId", "accountId", "account", "OwnerId"}
	graphRegionFields  = []string{"region", "Region", "location", "Location"}
)

// Words in a definition name that suggest its visual class, most specific
// first. Anything else defaults to compute.
var graphClassHints = []struct{ word, class string }{
	{"securitygroup", "security"}, {"firewall", "security"}, {"key", "security"}, {"secret", "security"}, {"certificate", "security"},
	{"user", "identity"}, {"role", "identity"}, {"policy", "identity"}, {"serviceaccount", "identity"}, {"group", "identity"},
	{"subnet", "network"}, {"network", "network"}, {"vpc", "network"}, {"route", "network"}, {"address", "network"}, {"gateway", "network"}, {"interface", "network"},
	{"bucket", "data"}, {"database", "data"}, {"db", "data"}, {"sql", "data"}, {"volume", "data"}, {"disk", "data"}, {"table", "data"},
	{"queue", "messaging"}, {"topic", "messaging"}, {"subscription", "messaging"}, {"stream", "messaging"},
	{"loadbalancer", "edge"}, {"endpoint", "edge"}, {"zone", "edge"}, {"dns", "edge"},
	{"account", "admin"}, {"project", "admin"}, {"folder", "admin"}, {"organization", "admin"},
}

// writeGraphBlock writes a `graph` block for a new type: labels from fields
// the data has, display info guessed from the definition name, and the view
// roles as commented examples. It is valid as written; the TODOs say what to
// check. `pudl graph vocab` shows the result.
func writeGraphBlock(b *strings.Builder, analysis *FieldAnalysis, opts GenerateOptions) {
	b.WriteString("\t\t// How pudl ui draws and places this type. Edit, then check with\n")
	b.WriteString("\t\t// 'pudl graph vocab' (docs/schema-authoring.md, \"Make a type appear in pudl ui\").\n")
	b.WriteString("\t\tgraph: {\n")
	writeGraphField(b, "name", firstField(analysis, graphNameFields), "field holding the resource's display name")
	writeGraphField(b, "account", firstField(analysis, graphAccountFields), "field naming the owning account or project")
	writeGraphField(b, "region", firstField(analysis, graphRegionFields), "field holding the region")
	fmt.Fprintf(b, "\t\t\tdisplay: {code: %q, label: %q, class: %q} // TODO: check; class is one of %s\n",
		graphCode(opts.DefinitionName), graphLabel(opts.DefinitionName), graphClass(opts.DefinitionName), strings.Join(graphspec.Classes, ", "))
	b.WriteString("\t\t\t// Roles in the views (all optional; edge types come from graph_edge rules):\n")
	b.WriteString("\t\t\t// network: {via: [\"in_subnet\"]}         // network view: placed under the target of an in_subnet edge\n")
	b.WriteString("\t\t\t// network: {group: true, region: true} // network view: contains others, inside a region group\n")
	b.WriteString("\t\t\t// security: control: true               // security view: always shown\n")
	b.WriteString("\t\t\t// port_of: \"attached_to\"                // drawn as a port on the resource it is attached to\n")
	b.WriteString("\t\t\t// admin: parent: [\"in_folder\"]          // an administrative node (account, project, folder)\n")
	b.WriteString("\t\t}\n")
}

func writeGraphField(b *strings.Builder, key, field, what string) {
	if field == "" {
		fmt.Fprintf(b, "\t\t\t// %s: \"...\" // TODO: %s\n", key, what)
		return
	}
	fmt.Fprintf(b, "\t\t\t%s: %q // %s\n", key, field, what)
}

func firstField(analysis *FieldAnalysis, candidates []string) string {
	for _, name := range candidates {
		if _, ok := analysis.Fields[name]; ok {
			return name
		}
	}
	return ""
}

// graphCode is the definition's capitals ("SecurityGroup" -> "SG"), or its
// first four letters when it has fewer than two capitals.
func graphCode(def string) string {
	var caps []rune
	for _, r := range def {
		if unicode.IsUpper(r) || unicode.IsDigit(r) {
			caps = append(caps, r)
		}
	}
	if len(caps) < 2 {
		caps = []rune(strings.ToUpper(def))
		if len(caps) > 4 {
			caps = caps[:4]
		}
	}
	if len(caps) > graphspec.MaxCodeLen {
		caps = caps[:graphspec.MaxCodeLen]
	}
	return string(caps)
}

// graphLabel splits a definition name into words: "SecurityGroup" ->
// "Security group", "SQLInstance" -> "SQL instance".
func graphLabel(def string) string {
	var words []string
	start := 0
	runes := []rune(def)
	for i := 1; i < len(runes); i++ {
		// A word starts at a capital after a lower-case letter, or at the
		// last capital of an acronym followed by lower case ("SQLInstance").
		acronymEnd := unicode.IsUpper(runes[i-1]) && i+1 < len(runes) && unicode.IsLower(runes[i+1])
		if unicode.IsUpper(runes[i]) && (!unicode.IsUpper(runes[i-1]) || acronymEnd) {
			words = append(words, string(runes[start:i]))
			start = i
		}
	}
	words = append(words, string(runes[start:]))
	for i := 1; i < len(words); i++ {
		if !isAcronym(words[i]) {
			words[i] = strings.ToLower(words[i])
		}
	}
	return strings.Join(words, " ")
}

func isAcronym(w string) bool {
	return len(w) > 1 && strings.ToUpper(w) == w
}

func graphClass(def string) string {
	lower := strings.ToLower(def)
	for _, h := range graphClassHints {
		if strings.Contains(lower, h.word) {
			return h.class
		}
	}
	return "compute"
}
