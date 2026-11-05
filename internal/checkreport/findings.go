package checkreport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	"github.com/chazu/pudl/internal/datalog"
)

// FindingLimit bounds how many finding identities one check records. A check
// with more failing matches records the first FindingLimit by key and marks
// its findings truncated, which keeps it from establishing that anything was
// resolved.
const FindingLimit = 1000

// Finding states. A current failing match is new or persisting; an earlier
// one that no longer matches is resolved, or unknown when this evaluation
// cannot establish its absence.
//
// A waived match keeps its key and is classified like any other (its state
// says whether it is new); waived is a separate mark, so waiving never
// resolves a finding.
const (
	FindingNew        = "new"
	FindingPersisting = "persisting"
	FindingResolved   = "resolved"
	FindingUnknown    = "unknown"
)

// Finding is one failing match of a check, identified stably across
// observations. Arguments are recorded when the match is one of a report's
// witnesses; Resource and Owner are recorded for every finding. FirstSeen is
// when a comparable report first recorded it. Waived marks a current finding
// an active waiver excuses: it keeps its state, but is not a failure (an
// expired waiver excuses nothing).
type Finding struct {
	Key       string         `json:"key"`
	State     string         `json:"state,omitempty"`
	FirstSeen *time.Time     `json:"first_seen,omitempty"`
	Waived    bool           `json:"waived,omitempty"`
	Resource  string         `json:"resource,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
	// Owner is the owner of the resource the finding references (see
	// AttributeOwners); "" when none was resolved.
	Owner string `json:"owner"`
	// ResourceID, Account, Type and Name describe the cataloged resource one
	// of the match's argument values identifies (see AttributeResources): its
	// resource ID, the account or project that owns it, its type and its
	// name. Empty when no value identifies one.
	ResourceID string `json:"resource_id,omitempty"`
	Account    string `json:"account,omitempty"`
	Type       string `json:"type,omitempty"`
	// Name is that resource's display name (its graph name).
	Name string `json:"name,omitempty"`
	// refs are the match's string arguments, kept so that a finding whose
	// arguments are not recorded can still be attributed to its resource and
	// owner. They are not stored.
	refs map[string]any
}

// evidenceFields name the observation a match came from, not what failed: a
// resource re-observed under a new entry, snapshot or run is the same
// finding.
var evidenceFields = map[string]bool{
	"entry_id": true, "evidence_id": true, "observation_id": true, "snapshot_id": true, "run_id": true,
}

// FindingKey is the stable identity of one failing match: a digest of its
// projected head arguments without evidence identifiers. Row order and
// re-observation do not change it; a head that projects only evidence
// identifiers is identified by them.
func FindingKey(args map[string]any) string {
	identity := make(map[string]any, len(args))
	for name, value := range args {
		if !evidenceFields[name] {
			identity[name] = value
		}
	}
	if len(identity) == 0 {
		identity = args
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		encoded = []byte("<unencodable>")
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:12])
}

// MatchFindings records the identity and resource of every failing match of a
// check, with the arguments of those that are also witnesses and whether
// waivedKeys says an active waiver excuses it. A failing `expect: nonempty` check is one
// finding with no arguments. truncated reports that more matched than
// FindingLimit.
func MatchFindings(tuples []datalog.Tuple, expect, outcome string, witnesses []Witness, waivedKeys map[string]bool) (findings []Finding, truncated bool) {
	if outcome != "fail" && outcome != OutcomeWaived {
		return nil, false
	}
	if expect != "empty" {
		return []Finding{{Key: FindingKey(map[string]any{}), Arguments: map[string]any{}}}, false
	}
	known := make(map[string]map[string]any, len(witnesses))
	for _, witness := range witnesses {
		known[witness.Key] = witness.Arguments
	}
	seen := map[string]bool{}
	for _, tuple := range tuples {
		key := FindingKey(tuple.Args)
		if seen[key] {
			continue
		}
		seen[key] = true
		findings = append(findings, Finding{Key: key, Waived: waivedKeys[key], Arguments: known[key],
			Resource: resourceOf(tuple.Args), refs: stringArgs(tuple.Args)})
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].Key < findings[j].Key })
	if len(findings) > FindingLimit {
		return findings[:FindingLimit], true
	}
	return findings, false
}
