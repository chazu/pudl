package cmd

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// jsonContractExempt lists runnable commands the JSON output contract test
// does not run, with the reason. Every other runnable command must appear in
// TestJSONOutputContract, so a new command is classified deliberately.
var jsonContractExempt = map[string]string{
	"pudl facts":                 "command group",
	"pudl model":                 "command group",
	"pudl mu":                    "command group",
	"pudl rule":                  "command group",
	"pudl schema":                "command group",
	"pudl snapshot":              "command group",
	"pudl guide":                 "prints reference documentation",
	"pudl prime":                 "prints reference documentation",
	"pudl setup":                 "edits shell configuration; text only",
	"pudl schema edit":           "opens an interactive editor",
	"pudl schema status":         "text-only Git wrapper",
	"pudl schema commit":         "text-only Git wrapper",
	"pudl schema log":            "text-only Git wrapper",
	"pudl module add":            "text-only CUE tooling wrapper",
	"pudl module info":           "text-only CUE tooling wrapper",
	"pudl module list":           "text-only CUE tooling wrapper",
	"pudl module tidy":           "text-only CUE tooling wrapper",
	"pudl schema migrate":        "maintenance command with a text report",
	"pudl schema reinfer":        "maintenance command with a text report",
	"pudl migrate identity":      "maintenance command with a text report",
	"pudl reclassify":            "maintenance command with a text report",
	"pudl schema add":            "schema authoring; text report",
	"pudl schema new":            "schema authoring; covered by schema_new tests",
	"pudl model new":             "scaffold authoring; needs a plugin",
	"pudl model populator add":   "scaffold authoring; needs a populator",
	"pudl model populator new":   "scaffold authoring",
	"pudl rule add":              "covered by the datalog rule tests",
	"pudl rule new":              "covered by the datalog rule tests",
	"pudl run set":               "observes live through mu",
	"pudl run resume":            "needs a pending approval",
	"pudl run reject":            "needs a pending approval",
	"pudl snapshot current":      "needs a live observation recorded for a model",
	"pudl mu ingest-manifest":    "needs a mu build manifest",
	"pudl completion":            "generates shell scripts",
	"pudl completion bash":       "generates shell scripts",
	"pudl completion fish":       "generates shell scripts",
	"pudl completion powershell": "generates shell scripts",
	"pudl completion zsh":        "generates shell scripts",
}

// TestJSONOutputContract runs each JSON-capable command with --json against a
// seeded workspace and requires stdout to be exactly one JSON document: no
// banners, progress or warnings mixed in, and no top-level null.
func TestJSONOutputContract(t *testing.T) {
	cliWorkspace(t)
	require.NoError(t, os.WriteFile("widget.json", []byte(`{"kind":"Widget","name":"w1"}`), 0o644))

	run := func(args ...string) []byte {
		t.Helper()
		r := runCLI(t, append(args, "--json")...)
		require.NoError(t, r.Err, "pudl %s: stderr=%s", strings.Join(args, " "), r.Stderr)
		out := strings.TrimSpace(r.Stdout)
		require.True(t, json.Valid([]byte(out)), "pudl %s --json wrote non-JSON stdout:\n%s", strings.Join(args, " "), r.Stdout)
		require.NotEqual(t, "null", out, "pudl %s --json wrote null", strings.Join(args, " "))
		return []byte(out)
	}
	field := func(doc []byte, path ...string) string {
		t.Helper()
		var v any
		require.NoError(t, json.Unmarshal(doc, &v))
		for _, p := range path {
			switch node := v.(type) {
			case map[string]any:
				v = node[p]
			case []any:
				require.NotEmpty(t, node)
				v = node[0].(map[string]any)[p]
			}
		}
		s, _ := v.(string)
		require.NotEmpty(t, s, "missing %v in %s", path, doc)
		return s
	}

	run("version")
	run("init")
	run("example", "install", "git-inventory")
	run("mu", "ingest-observe", "--path", ".pudl/populators/git-inventory/baseline-observe.json", "--origin", "git-baseline")
	run("run", "git-inventory", "--from-catalog", "--catalog-scope", "git-baseline")
	run("run", "report")
	run("status")
	run("model", "list")
	run("model", "show", "git-inventory")
	run("model", "validate", "git-inventory")
	run("model", "deps")
	run("schema", "list")
	run("schema", "show", "pudl/git.#GitRepository")
	run("doctor")
	run("config")
	run("config", "set", "version", "1.0")
	run("config", "reset")

	snapshotID := field(run("snapshot", "list"), "SnapshotID")
	run("snapshot", "show", snapshotID)
	run("snapshot", "retain", snapshotID)
	run("snapshot", "prune", "--dry-run")

	imported := run("import", "--path", "widget.json")
	require.Equal(t, importStatusImported, field(imported, "status"))
	entryID := field(imported, "id")
	run("list")
	run("list", "--origin", "git-baseline", "--items-only")
	proquint := field(run("show", entryID), "entry", "proquint")
	run("show", proquint, "--raw", "--metadata")
	run("export", "--id", proquint)

	factID := field(run("facts", "add", "--relation", "host", "--args", `{"name":"h1"}`), "id")
	run("facts", "list", "--relation", "host")
	run("facts", "list", "--relation", "nothing-here")
	run("facts", "show", factID)
	run("facts", "search", "h1")
	run("facts", "stats")
	run("facts", "reproject")
	run("facts", "reproject", "--dry-run")
	run("query", "host")
	run("query", "nothing-here")
	run("facts", "invalidate", factID)
	otherID := field(run("facts", "add", "--relation", "host", "--args", `{"name":"h2"}`), "id")
	run("facts", "retract", otherID)

	run("delete", proquint)
}

// TestJSONOutputContractClassifiesEveryCommand fails when a runnable command is
// neither exercised by TestJSONOutputContract nor exempted with a reason.
func TestJSONOutputContractClassifiesEveryCommand(t *testing.T) {
	src, err := os.ReadFile("cli_json_contract_test.go")
	require.NoError(t, err)
	body := string(src)
	start := strings.Index(body, "func TestJSONOutputContract(")
	end := strings.Index(body, "func TestJSONOutputContractClassifiesEveryCommand(")
	exercised := body[start:end]

	var unclassified, stale []string
	seen := map[string]bool{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.Hidden || sub.Name() == "help" {
				continue
			}
			path := sub.CommandPath()
			seen[path] = true
			if sub.Runnable() && jsonContractExempt[path] == "" && !exercisesCommand(exercised, path) {
				unclassified = append(unclassified, path)
			}
			walk(sub)
		}
	}
	walk(rootCmd)
	for path := range jsonContractExempt {
		if !seen[path] {
			stale = append(stale, path)
		}
	}
	sort.Strings(unclassified)
	sort.Strings(stale)
	require.Empty(t, unclassified, "exercise these commands in TestJSONOutputContract or exempt them in jsonContractExempt")
	require.Empty(t, stale, "jsonContractExempt names commands that no longer exist")
}

// exercisesCommand reports whether the contract test body runs the command at
// path, e.g. `run("facts", "add"` for "pudl facts add".
func exercisesCommand(body, path string) bool {
	words := strings.Fields(strings.TrimPrefix(path, "pudl "))
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = `"` + w + `"`
	}
	return strings.Contains(body, "run("+strings.Join(quoted, ", ")+",") ||
		strings.Contains(body, "run("+strings.Join(quoted, ", ")+")")
}
