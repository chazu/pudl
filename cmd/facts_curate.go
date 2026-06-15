package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
)

var (
	curatePromoteHelpful int
	curateRejectHarmful  int
	curateDryRun         bool
)

var factsCurateCmd = &cobra.Command{
	Use:   "curate",
	Short: "Deterministically advance observation maturity from feedback",
	Long: `Apply deterministic maturity transitions to observations based on the feedback
recorded against them. This is the Curator: no LLM, no judgment — just counting
helpful/harmful feedback and applying the same transition rules every time.

Rules (per observation, against accumulated feedback across its version lineage):
    harmful >= --reject-harmful                         -> rejected
    status raw,      helpful >= --promote-helpful, harmful 0 -> reviewed
    status reviewed, helpful >= --promote-helpful, harmful 0 -> promoted

Maturity advances one step per run (raw -> reviewed -> promoted), so a fact with
enough support reaches "promoted" over successive runs. Feedback targets a fact
ID; because a transition mints a new version ID, the curator follows each
observation's prevVersion chain so feedback on earlier versions still counts.

Use --dry-run to preview without writing.

Examples:
    pudl facts curate
    pudl facts curate --dry-run
    pudl facts curate --promote-helpful 5 --reject-harmful 1`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configDir := config.GetPudlDir()
		db, err := database.NewCatalogDB(configDir)
		if err != nil {
			return fmt.Errorf("failed to open catalog: %w", err)
		}
		defer db.Close()

		// Tally current feedback by target fact ID.
		feedback, err := db.QueryCurrentFacts("feedback")
		if err != nil {
			return fmt.Errorf("failed to read feedback: %w", err)
		}
		helpfulBy := map[string]int{}
		harmfulBy := map[string]int{}
		for _, fb := range feedback {
			var a map[string]interface{}
			if json.Unmarshal([]byte(fb.Args), &a) != nil {
				continue
			}
			target, _ := a["target"].(string)
			if target == "" {
				continue
			}
			switch v, _ := a["verdict"].(string); v {
			case "helpful":
				helpfulBy[target]++
			case "harmful":
				harmfulBy[target]++
			}
		}

		observations, err := db.QueryCurrentFacts("observation")
		if err != nil {
			return fmt.Errorf("failed to read observations: %w", err)
		}

		type plan struct {
			id, from, to     string
			helpful, harmful int
		}
		var plans []plan
		for _, o := range observations {
			var a map[string]interface{}
			if json.Unmarshal([]byte(o.Args), &a) != nil {
				continue
			}
			status, _ := a["status"].(string)
			if status == "" {
				status = "raw"
			}
			if status == "promoted" || status == "rejected" {
				continue // terminal
			}

			helpful, harmful := 0, 0
			for _, id := range observationLineage(db, o) {
				helpful += helpfulBy[id]
				harmful += harmfulBy[id]
			}

			to := ""
			switch {
			case harmful >= curateRejectHarmful:
				to = "rejected"
			case harmful == 0 && helpful >= curatePromoteHelpful && status == "raw":
				to = "reviewed"
			case harmful == 0 && helpful >= curatePromoteHelpful && status == "reviewed":
				to = "promoted"
			}
			if to == "" {
				continue
			}
			plans = append(plans, plan{id: o.ID, from: status, to: to, helpful: helpful, harmful: harmful})
		}

		if len(plans) == 0 {
			fmt.Println("No observations to curate.")
			return nil
		}

		applied := 0
		for _, p := range plans {
			if curateDryRun {
				fmt.Printf("[dry-run] %s  %s → %s  (helpful=%d harmful=%d)\n", p.id[:12], p.from, p.to, p.helpful, p.harmful)
				continue
			}
			_, newID, err := promoteFact(db, p.id, p.to, "")
			if err != nil {
				fmt.Printf("skip %s: %v\n", p.id[:12], err)
				continue
			}
			fmt.Printf("%s  %s → %s  (helpful=%d harmful=%d) → %s\n", p.id[:12], p.from, p.to, p.helpful, p.harmful, newID[:12])
			applied++
		}

		if !curateDryRun {
			fmt.Printf("\nCurated %d observation(s).\n", applied)
		}
		return nil
	},
}

// observationLineage returns the fact ID of o followed by the IDs of its prior
// maturity versions, walked via the prevVersion pointer. Bounded to guard against
// a malformed cycle.
func observationLineage(db *database.CatalogDB, o database.Fact) []string {
	ids := []string{o.ID}
	cur := o
	for i := 0; i < 50; i++ {
		var a map[string]interface{}
		if json.Unmarshal([]byte(cur.Args), &a) != nil {
			break
		}
		prev, _ := a["prevVersion"].(string)
		if prev == "" {
			break
		}
		pf, err := db.GetFact(prev)
		if err != nil {
			break
		}
		ids = append(ids, pf.ID)
		cur = *pf
	}
	return ids
}

func init() {
	factsCmd.AddCommand(factsCurateCmd)
	factsCurateCmd.Flags().IntVar(&curatePromoteHelpful, "promote-helpful", 3, "Helpful feedback needed to advance maturity (with zero harmful)")
	factsCurateCmd.Flags().IntVar(&curateRejectHarmful, "reject-harmful", 2, "Harmful feedback that triggers rejection")
	factsCurateCmd.Flags().BoolVar(&curateDryRun, "dry-run", false, "Preview transitions without writing")
}
