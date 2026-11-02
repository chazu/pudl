# Check waivers with owner, reason and expiry (pudl-xf5.4)

A model can now accept a check finding until a date. Waivers cover Datalog
check witnesses and compliance-policy violations (keyed on policy, rule and
resource, as the ticket notes require for pudl-xf5.8).

## Where waivers are declared: on the model

`waivers: [...#Waiver]` is a field of `#SystemModel`, next to `checks`. This
is the leanest option:

- It reuses the model's load path: the embedded schema validates it, the
  file is reviewed and versioned with the checks, and no new file discovery,
  global/repository shadowing or package is needed.
- Check names are unique only within a model (every GCP family has a
  `policy-high`), so a check waiver needs no model qualifier.
- Staleness is well defined: a model's waivers are matched exactly when its
  checks run, so "matched nothing" means nothing in that run.

A workspace-level waivers file would make marking waivers in `pudl policy
show` and `pudl show` cheap, but it needs its own loader and a model key on
every check waiver, and its stale waivers could only be judged for models
that happened to run. That marking is deferred (see Follow-ups).

## CUE surface

```cue
#Waiver: {
	check?:    string & !=""      // a check of this model (expect: "empty")
	witness?:  {[string]: _}      // values a witness must carry (all of them)
	policy?:   string & !=""      // or: a policy violation
	rule?:     string & !=""
	resource?: string & !=""      // resource ID (pudl policy show lists them)
	owner:     string & !=""
	reason:    string & !=""
	created:   #Date              // "YYYY-MM-DD"
	expires:   #Date
}
```

Load fails, naming the waiver (`waivers[N]`) and the field, when: a required
field is missing (validated concretely before decode); a date is not a real
`YYYY-MM-DD` date (`2026-02-30`); `expires` is before `created`; there is no
target, both targets, an empty witness selector or a partial policy target;
the check is not a check of the model or expects `nonempty` (no witnesses);
or the model declares no checks.

## Semantics

- **Check waiver.** Matches a witness of its check that carries every
  selector value (values compared as canonical JSON, so CUE `22` equals a
  Datalog `int64`/`float64` 22).
- **Policy waiver.** Applies to any check of the model whose witnesses carry
  `resource` and `policy` arguments (`policy_noncompliant` and rules over it).
  A witness stands for that resource's current violations of the policy,
  narrowed by any `rule`, `severity`, `path` or `kind` argument it carries.
  It is waived only when it stands for at least one violation and every one
  has an active waiver. Violations are read from the catalog's current policy
  facts (`policyfacts.ReadStored`), looked up once per resource and policy.
- **Waived outcome.** Every finding (all tuples, not only the 20 kept
  witnesses) is matched. A failed `expect: "empty"` check whose every finding
  is waived gets outcome `waived`; `passed` stays false. One unwaived finding
  keeps it `fail`, with `waived: N` counting the rest. Witnesses keep their
  waiver (`witnesses[].waiver{index, owner, reason, expires, state}`);
  unwaived witnesses sort first, so truncation hides waived ones first.
- **Expired.** A waiver applies through its `expires` date (UTC). After that
  the finding counts again, the witness carries the waiver with
  `state: "expired"`, and the report flags it.
- **Stale.** An unexpired waiver that matched no finding.
- **Report.** The model report's `waivers` lists every declared waiver with
  `state` (`active`, `expired`, `stale`) and `matches`. Human output marks a
  waived check `~ name [sev] waived`, each waived witness `waived by
  waivers[N] (owner, until DATE): reason`, and prints a `### waivers` section.
- **Summary and exit.** `summary.checks` gains `waived` (precedence unknown >
  fail > waived > pass) and `waived_checks` counts them apart from
  `failed_checks`. Waived checks do not fail the model, do not demote its
  verdict and are not findings, so `--detailed-exitcode` exits 0 for them;
  an expired waiver's finding fails as before (exit 2).
- **GCP pack.** `gcp_policy_noncompliant_high` heads carry `severity`
  (constant `high`/`critical`), so a waiver of a high violation is not
  defeated by an unwaived low violation of the same resource and policy.
  Fixture counts are unchanged.

## Public API

`internal/waiver` (pure; imports only `schemaname`):
- `Waiver{Check, Witness, Policy, Rule, Resource, Owner, Reason, Created,
  Expires}`, `(Waiver).IsPolicy()`, `Target()`, `Expired(now)`, `DateLayout`.
- `Validate(waivers []Waiver, checks map[string]string) error`.
- `NewMatcher(waivers, now, ViolationLookup) *Matcher`;
  `(*Matcher).Match(check, args) (*Ref, error)` — **the "is this finding
  waived" helper**: it returns the active waiver, an expired one (flag only),
  or nil; `(*Matcher).Outcomes() []Outcome`.
- `Ref{Index, Owner, Reason, Expires, State}`, `Outcome{Waiver, Index, State,
  Matches}`, `State` (`Active`, `Expired`, `Stale`), `Violation`,
  `ViolationLookup`.

`internal/checkreport`:
- `Witness.Waiver *waiver.Ref`, `(Witness).Waived() bool`.
- `CheckResult.Waived int`, `OutcomeWaived`, `(CheckResult).IsWaived()`,
  `(CheckResult).Failing()` (not passed and not waived).
- `(*CheckResult).SetFindings(tuples, MatchFunc) error`, `MatchFunc`,
  `WaivedWitnesses(tuples, expect, match)`; `Witnesses` is unchanged.
- `ModelReport.Waivers []waiver.Outcome`.

`internal/acute`: `VerdictSummary.WaivedChecks`; `Checks` may be `waived`.

`internal/systemmodel`: `SystemModel.Waivers`; `#Waiver`, `#Date` in the
schema; `DecodeValue` validates waivers.

`internal/operation`: `checkContext.waivers` (a matcher built per model run
in `finalize`); `runChecks` keeps its signature.

## Deferred

`pudl policy show` and the `pudl show` compliance section do not mark waived
violations. Waivers belong to a model's checks; marking them there would load
and evaluate every model's CUE on each `show`, and a violation can be waived
in one model's checks and not another's.

## Line deltas against `sre-focus`

`git diff --numstat sre-focus HEAD`, Go files, after merging `sre-focus`:

| | Added | Removed | Net |
| --- | --- | --- | --- |
| Production | +652 | −49 | +603 |
| Tests | +436 | −0 | +436 |

Plus 35 lines of CUE in `internal/systemmodel/schema.cue` and 4 changed in
the GCP pack rules. Production: `internal/waiver` 377 (validation, matching,
outcomes, with docs), `checkreport` +124, `operation` +66, `systemmodel` +31,
`acute` +5.

## Tests

- `internal/waiver`: expiry boundary (applies on the date, not the next day),
  selector matching with numeric canonicalization, active over expired,
  expired and stale outcomes, policy waivers covering every violation with
  severity narrowing, partial expiry flagged, no current violation stays
  stale, one lookup per pair, lookup errors propagate.
- `internal/systemmodel`: both targets decode; twelve load rejections.
- `internal/checkreport`: all tuples matched, unwaived first under
  truncation, waived only when every finding is; summary counts.
- `cmd` `TestCheckWaivers` over the GCP fixture with a checks-only model
  holding a Datalog check (`nat-ip-not-in-use`) and a policy check
  (`policy-high`): stale, active and expired waivers, each in JSON and human
  output and with `--detailed-exitcode` (2, 0, 2).

## Verification

`CGO_ENABLED=0 go test ./...` passes; `CGO_ENABLED=0 mise exec -- make lint`
reports 0 issues; `mise exec -- make check-docs check-skills` passes.
