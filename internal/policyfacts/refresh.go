package policyfacts

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/policy"
	"github.com/chazu/pudl/internal/projection"
)

// Options select what Refresh evaluates.
type Options struct {
	// Policies names the policies to evaluate; empty means every loaded
	// policy, and then results of policies no longer loaded are closed too.
	Policies []string
	// DryRun computes every change and rolls it back.
	DryRun bool
}

// Select resolves policy names against the loaded set; no names selects
// every loaded policy.
func (e *Evaluator) Select(names []string) ([]*policy.Policy, error) {
	var loaded []*policy.Policy
	if e.set != nil {
		loaded = e.set.Policies()
	}
	if len(names) == 0 {
		return loaded, nil
	}
	var out []*policy.Policy
	for _, name := range names {
		if p, ok := e.lookup(name); ok {
			out = append(out, p)
			continue
		}
		for _, le := range e.loadErrors {
			if le.Policy == name {
				return nil, le
			}
		}
		var list []string
		for _, p := range loaded {
			list = append(list, p.Name)
		}
		if list == nil {
			list = []string{"none"}
		}
		return nil, fmt.Errorf("policy %q is not loaded (loaded: %s)", name, strings.Join(list, ", "))
	}
	return out, nil
}

// lookup returns a loaded policy by name.
func (e *Evaluator) lookup(name string) (*policy.Policy, bool) {
	if e.set == nil {
		return nil, false
	}
	return e.set.Policy(name)
}

var errDryRun = errors.New("policy evaluation dry run")

// Refresh re-evaluates every stored current resource (see
// database.CurrentResourceEntries) from its retained payload, in one
// transaction. Changed results replace the current facts as an observation,
// so history keeps the previous result and its policy digest. Results whose
// entry was deleted or re-identified are retracted as corrections.
func (e *Evaluator) Refresh(ctx context.Context, db *database.CatalogDB, load projection.LoadFunc, opts Options) (Report, error) {
	selected, err := e.Select(opts.Policies)
	if err != nil {
		return Report{}, err
	}
	var scope []string
	if len(opts.Policies) > 0 {
		for _, p := range selected {
			scope = append(scope, p.Name)
		}
	}
	t := newTally(selected)
	err = db.WithCatalogTxContext(ctx, func(tx *database.CatalogTx) error {
		orphans, err := tx.PolicyOrphans()
		if err != nil {
			return err
		}
		probe := &Prepared{Scope: scope}
		for _, s := range orphans {
			if !probe.covers(s.Policy) {
				continue
			}
			if err := closeResult(tx, s.ResourceID, s.Policy, database.ProjectionCorrection); err != nil {
				return err
			}
			t.report.Orphaned++
		}
		entries, err := tx.CurrentResourceEntries()
		if err != nil {
			return err
		}
		err = e.evaluateStored(ctx, entries, selected, scope, load, t, func(p *Prepared) error {
			change, err := Observe(tx, p, database.ProjectionObservation)
			t.report.add(change)
			return err
		})
		if err == nil && opts.DryRun {
			return errDryRun
		}
		return err
	})
	if errors.Is(err, errDryRun) {
		err = nil
	}
	return t.finish(e.loadErrors), err
}

// Preview evaluates policies against every stored current resource without
// writing anything: a what-if evaluation of a draft policy.
func (e *Evaluator) Preview(ctx context.Context, db *database.CatalogDB, load projection.LoadFunc, policies []*policy.Policy) (Report, error) {
	entries, err := db.CurrentResourceEntries()
	if err != nil {
		return Report{}, err
	}
	t := newTally(policies)
	if err := e.evaluateStored(ctx, entries, policies, nil, load, t, func(*Prepared) error { return nil }); err != nil {
		return Report{}, err
	}
	return t.finish(e.loadErrors), nil
}

// evaluateStored evaluates the selected policies that apply to each stored
// entry, tallies the results and passes them to observe. The payload is read
// only when a policy applies; an unreadable payload is reported and the
// entry's results are left as they were.
func (e *Evaluator) evaluateStored(ctx context.Context, entries []database.CatalogEntry, selected []*policy.Policy, scope []string, load projection.LoadFunc, t *tally, observe func(*Prepared) error) error {
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		applicable := intersect(e.Applicable(entry.Schema), selected)
		var record any
		if len(applicable) > 0 {
			var err error
			if record, err = load(entry); err != nil {
				t.report.Errors = append(t.report.Errors, fmt.Sprintf("%s: %v", entry.ID, err))
				continue
			}
		}
		p := e.prepare(applicable, scope, entry.Schema, entry.ID, *entry.ResourceID, record)
		t.note(p)
		if err := observe(p); err != nil {
			return err
		}
	}
	return nil
}

// intersect keeps the applicable policies (sorted by name) that are selected.
func intersect(applicable, selected []*policy.Policy) []*policy.Policy {
	return slices.DeleteFunc(applicable, func(p *policy.Policy) bool {
		return !slices.ContainsFunc(selected, func(s *policy.Policy) bool { return s.Name == p.Name })
	})
}
