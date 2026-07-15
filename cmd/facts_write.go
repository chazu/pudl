package cmd

import (
	"encoding/json"
	"fmt"
	"os/user"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/validator"
)

var (
	factsAddRelation   string
	factsAddArgs       string
	factsAddSource     string
	factsAddSchema     string
	factsAddNoValidate bool

	factsPromoteTo     string
	factsPromoteRule   string
	factsPromoteSource string

	factsSearchRelation string
	factsSearchLimit    int
)

// defaultFactSource returns the current OS username, or "human" if unavailable.
// Shared default for agent-facing fact writes (observe, add, promote).
func defaultFactSource() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return "human"
}

// bootstrapRelationSchemas maps a built-in agent relation to the embedded CUE
// definition its facts must satisfy. Used for automatic strict validation of the
// known agent-memory relations on write.
var bootstrapRelationSchemas = map[string]struct{ file, def string }{
	"observation": {"pudl/nous/nous.cue", "#Observation"},
	"feedback":    {"pudl/nous/nous.cue", "#Feedback"},
}

// validateKnownRelation validates obj against the built-in definition for the
// given relation, if one is registered. Relations with no built-in schema pass
// (facts are general-purpose). Returns a descriptive error on mismatch.
func validateKnownRelation(relation string, obj map[string]interface{}) error {
	rs, ok := bootstrapRelationSchemas[relation]
	if !ok {
		return nil
	}
	if err := importer.ValidateAgainstBootstrapDef(rs.file, rs.def, obj); err != nil {
		return fmt.Errorf("does not satisfy %s: %w", rs.def, err)
	}
	return nil
}

var factsAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a fact to the bitemporal store (the canonical write)",
	Long: `Add a fact under any relation. This is the one low-level door for writing
facts — feedback, playbook bullets, diary entries, and any future relation all
go through it, so there is exactly one way to assert a fact.

Args must be a JSON object. With --schema, the args are validated against a CUE
schema before the fact is stored; without it, the args are stored as-is (facts
are general-purpose and many relations have no schema).

For the common 'observation' relation, prefer the sugar 'pudl facts observe'.

Examples:
    pudl facts add --relation feedback \
        --args '{"target":"abc123","verdict":"helpful","source":"claude"}'
    pudl facts add --relation depends --args '{"from":"a","to":"b"}'
    pudl facts add --relation feedback --args '{...}' --schema nous.#Feedback`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if factsAddRelation == "" {
			return fmt.Errorf("--relation is required")
		}
		if database.IsReservedRelation(factsAddRelation) {
			return fmt.Errorf("relation %q is reserved and cannot be written directly", factsAddRelation)
		}

		// Args must parse to a JSON object.
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(factsAddArgs), &obj); err != nil {
			return fmt.Errorf("--args must be a JSON object: %w", err)
		}

		// Schema validation. An explicit --schema validates against an on-disk
		// schema via the cascade validator. Otherwise, known agent relations are
		// validated against their built-in (embedded) definition, unless
		// --no-validate is given.
		switch {
		case factsAddSchema != "":
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config for validation: %w", err)
			}
			vs, err := validator.NewValidationService(effectiveSchemaPaths(cfg)...)
			if err != nil {
				return fmt.Errorf("failed to initialize validation: %w", err)
			}
			result := vs.ValidateDataAgainstSchema(obj, factsAddSchema)
			if !result.Valid {
				return fmt.Errorf("args do not satisfy %s:\n%s", factsAddSchema, vs.GetValidationSummary(result))
			}
		case !factsAddNoValidate:
			if err := validateKnownRelation(factsAddRelation, obj); err != nil {
				return fmt.Errorf("args %w", err)
			}
		}

		configDir := config.GetPudlDir()
		db, err := database.NewCatalogDB(configDir)
		if err != nil {
			return fmt.Errorf("failed to open catalog: %w", err)
		}
		defer db.Close()

		f, err := db.AddFact(database.Fact{
			Relation: factsAddRelation,
			Args:     factsAddArgs,
			Source:   factsAddSource,
		})
		if err != nil {
			return fmt.Errorf("failed to add fact: %w", err)
		}

		fmt.Printf("Added fact %s\n", f.ID[:12])
		if jsonOutput {
			out, _ := json.MarshalIndent(f, "", "  ")
			fmt.Println(string(out))
		}
		return nil
	},
}

// statusTransitions maps an observation's current status to the set of statuses
// it may legally transition to. Promoted and rejected are terminal.
var statusTransitions = map[string][]string{
	"raw":      {"reviewed", "rejected"},
	"reviewed": {"promoted", "rejected"},
}

func transitionAllowed(from, to string) bool {
	for _, s := range statusTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

var factsPromoteCmd = &cobra.Command{
	Use:   "promote <id>",
	Short: "Advance a fact's maturity status (raw → reviewed → promoted|rejected)",
	Long: `Move a fact through its maturity lifecycle by appending a new version with an
updated status. The transition is validated and applied atomically (read-check-
write under the fact-store write lock), so concurrent promotions cannot race.

This command only changes the status flag and records an optional promotedTo
pointer — it does not decide whether a fact deserves promotion and does not
synthesize a rule. That judgment belongs to the caller.

Legal transitions:
    raw      → reviewed | rejected
    reviewed → promoted | rejected

Examples:
    pudl facts promote abc123 --to reviewed
    pudl facts promote abc123 --to promoted --rule pudl/rules/no-cycles
    pudl facts promote abc123 --to rejected`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		idArg := args[0]
		if factsPromoteTo == "" {
			return fmt.Errorf("--to is required (reviewed, promoted, or rejected)")
		}
		if factsPromoteRule != "" && factsPromoteTo != "promoted" {
			return fmt.Errorf("--rule is only valid with --to promoted")
		}

		configDir := config.GetPudlDir()
		db, err := database.NewCatalogDB(configDir)
		if err != nil {
			return fmt.Errorf("failed to open catalog: %w", err)
		}
		defer db.Close()

		oldID, newID, err := promoteFact(db, idArg, factsPromoteTo, factsPromoteRule)
		if err != nil {
			return err
		}
		fmt.Printf("Promoted %s → %s (new version %s)\n", oldID[:12], factsPromoteTo, newID[:12])
		return nil
	},
}

// promoteFact advances a fact's maturity status to `to` (optionally recording a
// promotedTo rule reference), atomically under the fact-store write lock. It
// resolves idArg (full ID or unique prefix), validates the transition against the
// current status, then invalidates the prior version and appends the updated one.
// Returns the resolved old ID and the new version's ID. Shared by `facts promote`
// and the curator.
func promoteFact(db *database.CatalogDB, idArg, to, rule string) (oldID, newID string, err error) {
	target, err := db.GetFactByPrefix(idArg)
	if err != nil {
		if exact, e2 := db.GetFact(idArg); e2 == nil {
			target = exact
		} else {
			return "", "", fmt.Errorf("fact not found: %s", idArg)
		}
	}

	err = db.WithFactTx(func(tx *database.FactTx) error {
		// Re-read current facts for this relation under the write lock.
		current, qerr := tx.QueryFacts(database.FactFilter{Relation: target.Relation})
		if qerr != nil {
			return fmt.Errorf("failed to read current facts: %w", qerr)
		}
		var f *database.Fact
		for i := range current {
			if current[i].ID == target.ID {
				f = &current[i]
				break
			}
		}
		if f == nil {
			return fmt.Errorf("fact %s is not currently valid (already retracted, invalidated, or promoted)", target.ID[:12])
		}

		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(f.Args), &obj); err != nil {
			return fmt.Errorf("fact args are not a JSON object: %w", err)
		}
		from, _ := obj["status"].(string)
		if from == "" {
			from = "raw"
		}
		if from == to {
			return fmt.Errorf("fact is already %s", from)
		}
		if !transitionAllowed(from, to) {
			return fmt.Errorf("illegal transition %s → %s", from, to)
		}

		obj["status"] = to
		obj["prevVersion"] = f.ID // lineage pointer so feedback on prior versions stays reachable
		if rule != "" {
			obj["promotedTo"] = rule
		}
		newArgs, merr := json.Marshal(obj)
		if merr != nil {
			return fmt.Errorf("failed to marshal updated args: %w", merr)
		}

		// Invalidate the old version, append the new one. Same relation and source
		// preserve provenance; the differing args yield a new ID.
		if err := tx.InvalidateFact(f.ID); err != nil {
			return fmt.Errorf("failed to invalidate prior version: %w", err)
		}
		nf, aerr := tx.AddFact(database.Fact{
			Relation: f.Relation,
			Args:     string(newArgs),
			Source:   f.Source,
		})
		if aerr != nil {
			return fmt.Errorf("failed to write new version: %w", aerr)
		}
		newID = nf.ID
		return nil
	})
	if err != nil {
		return "", "", err
	}
	return target.ID, newID, nil
}

var factsSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Full-text search over currently-valid facts",
	Long: `Keyword search over the indexed text of currently-valid facts, best matches
first. The index covers the values of each fact's args (not the JSON keys).

The query uses SQLite FTS5 syntax: bare terms are ANDed, "quoted phrases" match
in order, a trailing * is a prefix match, and AND/OR/NOT combine terms.

Examples:
    pudl facts search "rate limiting"
    pudl facts search "circular dependency" --relation observation
    pudl facts search "auth*" --limit 10`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		configDir := config.GetPudlDir()
		db, err := database.NewCatalogDB(configDir)
		if err != nil {
			return fmt.Errorf("failed to open catalog: %w", err)
		}
		defer db.Close()

		facts, err := db.SearchCurrentFacts(args[0], factsSearchRelation, factsSearchLimit)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		if jsonOutput {
			out, _ := json.MarshalIndent(facts, "", "  ")
			fmt.Println(string(out))
			return nil
		}
		if len(facts) == 0 {
			fmt.Println("No matching facts.")
			return nil
		}
		for _, f := range facts {
			printFact(f, false)
		}
		fmt.Printf("\n%d match(es)\n", len(facts))
		return nil
	},
}

func init() {
	factsCmd.AddCommand(factsAddCmd)
	factsCmd.AddCommand(factsPromoteCmd)
	factsCmd.AddCommand(factsSearchCmd)

	factsSearchCmd.Flags().StringVar(&factsSearchRelation, "relation", "", "Limit search to a relation")
	factsSearchCmd.Flags().IntVar(&factsSearchLimit, "limit", 20, "Maximum results (0 = no limit)")
	factsSearchCmd.RegisterFlagCompletionFunc("relation", completeRelations)

	factsAddCmd.Flags().StringVar(&factsAddRelation, "relation", "", "Relation to write (required)")
	factsAddCmd.Flags().StringVar(&factsAddArgs, "args", "", "Fact body as a JSON object (required)")
	factsAddCmd.Flags().StringVar(&factsAddSource, "source", defaultFactSource(), "Source of the fact (agent name or username)")
	factsAddCmd.Flags().StringVar(&factsAddSchema, "schema", "", "Validate args against a named on-disk CUE schema (e.g. nous.#Feedback)")
	factsAddCmd.Flags().BoolVar(&factsAddNoValidate, "no-validate", false, "Skip automatic validation of known agent relations")
	factsAddCmd.MarkFlagRequired("relation")
	factsAddCmd.MarkFlagRequired("args")
	factsAddCmd.RegisterFlagCompletionFunc("relation", completeRelations)

	factsPromoteCmd.Flags().StringVar(&factsPromoteTo, "to", "", "Target status: reviewed, promoted, or rejected (required)")
	factsPromoteCmd.Flags().StringVar(&factsPromoteRule, "rule", "", "Rule/convention reference (only with --to promoted)")
	factsPromoteCmd.ValidArgsFunction = completeFactIDs
}
