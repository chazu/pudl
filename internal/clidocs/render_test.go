package clidocs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/cmd"
)

func testTree() *cobra.Command {
	root := &cobra.Command{Use: "pudl"}
	root.PersistentFlags().Bool("json", false, "Output JSON")

	run := &cobra.Command{Use: "run [<model>]", Short: "Run a model", Long: "Run a model.\n\nExamples:\n    pudl run m", RunE: noop}
	run.PersistentFlags().Duration("mu-timeout", 0, "limit each mu call")
	run.Flags().Int("max-iters", 5, "iteration cap")
	run.Flags().Bool("secret", false, "hidden")
	_ = run.Flags().MarkHidden("secret")

	set := &cobra.Command{Use: "set <model>...", Short: "Run a set", RunE: noop}
	set.Flags().String("approve", "", "wait for `pudl run resume <id>` | then <go>")

	hidden := &cobra.Command{Use: "internal", Short: "hidden", Hidden: true, RunE: noop}

	run.AddCommand(set)
	root.AddCommand(run, hidden)
	return root
}

func noop(*cobra.Command, []string) error { return nil }

func TestRenderSectionsFlagsAndNotes(t *testing.T) {
	notes := fstest.MapFS{
		IntroNote:    {Data: []byte("Intro text.")},
		"run_set.md": {Data: []byte("Set semantics.")},
	}
	out, err := Render(testTree(), notes)
	require.NoError(t, err)
	doc := string(out)

	require.True(t, strings.HasPrefix(doc, Header))
	require.Contains(t, doc, "Intro text.")
	require.Contains(t, doc, "## pudl run\n")
	require.Contains(t, doc, "## pudl run set\n")
	require.Contains(t, doc, "pudl run set <model>... [flags]")
	require.Contains(t, doc, "- [pudl run set](#pudl-run-set) — Run a set")

	// The global flag is documented once, not per command.
	require.Equal(t, 1, strings.Count(doc, "`--json`"))
	// A non-root persistent flag is inherited by subcommands.
	setSection := doc[strings.Index(doc, "## pudl run set\n"):]
	require.Contains(t, setSection, "`--mu-timeout`")
	require.Contains(t, doc, "| `--max-iters` | int | `5` | iteration cap |")

	// Hidden flags and commands are omitted.
	require.NotContains(t, doc, "--secret")
	require.NotContains(t, doc, "pudl internal")

	// Notes follow their command's generated section.
	require.Greater(t, strings.Index(doc, "Set semantics."), strings.Index(doc, "## pudl run set\n"))

	// Cells escape pipes everywhere and angle brackets only outside code spans.
	require.Contains(t, doc, "wait for `pudl run resume <id>` \\| then &lt;go&gt;")
}

func TestRenderRejectsStaleNotes(t *testing.T) {
	notes := fstest.MapFS{"run_gone.md": {Data: []byte("orphan")}}
	_, err := Render(testTree(), notes)
	require.ErrorContains(t, err, "run_gone.md")
}

func TestRenderIsDeterministic(t *testing.T) {
	notes := fstest.MapFS{}
	a, err := Render(testTree(), notes)
	require.NoError(t, err)
	b, err := Render(testTree(), notes)
	require.NoError(t, err)
	require.Equal(t, string(a), string(b))
}

// TestCLIReferenceInSync fails when docs/cli-reference.md no longer matches the
// command tree and notes; run `make docs` to regenerate it.
func TestCLIReferenceInSync(t *testing.T) {
	root := filepath.Join("..", "..")
	want, err := Render(cmd.RootCommand(), os.DirFS(filepath.Join(root, NotesDir)))
	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(root, ReferencePath))
	require.NoError(t, err)
	require.Equal(t, string(want), string(got), "docs/cli-reference.md is stale; run `make docs`")
}
