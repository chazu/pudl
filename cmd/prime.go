package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var primeCmd = &cobra.Command{
	Use:   "prime",
	Short: "Output agent prompt describing how to use pudl",
	Long: `Print a structured prompt that teaches AI agents how to use pudl.

Include a line like this in your CLAUDE.md or similar agent config:

    Run 'pudl prime' to learn how to use pudl.

The agent will then know to execute the command and read the output
to understand pudl's capabilities and conventions.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprint(outw(), primeText)
	},
}

func init() {
	rootCmd.AddCommand(primeCmd)
}

const primeText = `# pudl: agent briefing

pudl discovers, catalogs and governs cloud infrastructure. It runs collectors
(plain gcloud/aws/kubectl commands that print JSON) or imports files, stores
every observation as a snapshot in a local SQLite catalog (.pudl/ in this
repository), classifies each record against a CUE type schema, evaluates
compliance policies per resource, and runs Datalog checks. It observes and
reports; it never changes infrastructure. Hand fixes to the tool that owns
the resource.

## The loop

1. Set up:       pudl init
2. Observe:      pudl example install gcp      (or aws; or write a collector:
                 pudl model new NAME --populate 'command:<cmd printing JSON>')
3. Check:        pudl check COLLECTOR... CHECK-FAMILY --json
4. Read:         pudl report --json; pudl policy list; pudl policy show POLICY;
                 pudl show ENTRY-ID (type, compliance) [--history]
5. Fix or waive: fix the resource with its owning tool, or add a waiver
                 (owner, reason, created, expires) to the model
6. Re-check:     the same pudl check, with --fail-on new --detailed-exitcode
                 in CI; findings show as new, persisting or resolved

Offline first run, with the GCP pack's fixtures:

  pudl init
  pudl example install gcp
  pudl check gcp-fixture-firewalls gcp-fixture-addresses gcp-fixture-routers \
    gcp-fixture-network-hygiene-checks --json --detailed-exitcode
  pudl policy show gcp.#FirewallBestPractice

## Rules that matter

- pudl check runs exactly the named models, producers first by depends_on.
  Name the collectors a check family reads; nothing else runs.
- A check is pass, fail, unknown, error or waived. Unknown (missing,
  incomplete or stale evidence) is not pass. A failed collector input makes
  the snapshot incomplete; it never makes resources look absent.
- Findings carry an owner and a change state: new, persisting, resolved,
  unknown. Only a complete observation resolves a finding.
- Preview before writing: import --dry-run, doctor --fix --dry-run,
  policy evaluate --what-if FILE.
- Check a new import source with import --dry-run --explain first: a record
  that matches no type still imports, as pudl/core.#Item. Fix records on
  pudl/core.#Item or an unexpected type (--schema, --set, a type) first.
- Query projected facts (pudl query) rather than parsing payloads.

## JSON contracts

- pudl check ... --json and pudl report [ID] --json: one versioned report
  (report_version 3): run_set_id, status, summary, members[].report.
- pudl report ID --since ID2 --json: findings classified per check.
- pudl doctor --json (report_version 3); pudl policy list|show|evaluate --json
  (report_version 1); list, show, import, query, model and snapshot take --json.
- pudl help --json is the command tree; trust it over any document.
- Exit codes: 0 ok, 1 error or failed check. With --detailed-exitcode, 2 means
  drift or a failing fail-severity check. --fail-on new fails only on new
  findings.

## Everyday commands

  pudl import FILE [--schema TYPE] [--set path=value] [--dry-run]
  pudl list --json / pudl show ID [--history] / pudl export --id ID
  pudl model list|show|validate|new|deps
  pudl check MODEL... / pudl report [ID] [--since ID] / pudl status
  pudl policy list|show|evaluate
  pudl schema list|show|new|add / pudl rule new|add
  pudl query RELATION key=value / pudl query --list
  pudl snapshot list|show|current|retain|prune
  pudl doctor [--fix [--only NAME] [--dry-run]]

Retired; do not use: run, run set, run report, converge, approvals, mu,
schema reinfer/migrate, reclassify, migrate identity, facts reproject
(docs/retired-commands.md).

## More

  pudl guide              topics: observe, schemas, policies, checks, query,
                          doctor, packs, agents
  pudl help CMD --json    one command's flags
  docs/getting-started.md (Git walkthrough, GCP governance tour),
  docs/cli-reference.md, docs/schema-authoring.md
`
