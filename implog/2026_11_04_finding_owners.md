# Every check finding carries its resource and owner (pudl-upc)

**Date:** 2026-11-04

## Root cause

`checkreport.MatchFindings` recorded a finding's arguments only when the match
was also a witness, and `AttributeOwners` resolved a finding's owner from
those recorded arguments. Witnesses are capped at `WitnessLimit` (20), so
findings past the cap got `owner: ""`. Waived matches are sorted after
unwaived ones in the witness list, so they are the first to fall past the cap,
which is why waived findings lost their owner too.

## Work done

- `MatchFindings` now records, for every finding, its `resource` (the
  `resource`, `target` or `name` argument, the same rule witnesses use) and
  keeps the match's resource and catalog-entry references (`resource_id`,
  `resource`, `target`, `name`, `entry_id`, `evidence_id`, `observation_id`)
  in an unexported, unserialized field.
- `AttributeOwners` resolves each finding made in this evaluation from those
  references, independent of the witness cap and of waiver state. Findings
  from earlier reports (resolved/unknown) still use their recorded arguments,
  and keep their recorded owner through classification as before.
- The witness cap is unchanged and arguments are still recorded for witnesses
  only. Recording them for all findings (up to `FindingLimit` = 1000, each
  value up to 4 KB) would grow stored reports a lot and change the human
  rendering, which lists findings with arguments; the issue's minimum
  (resource and owner for every finding) is what is kept.
- `newWitness` and `MatchFindings` share a new `resourceOf` helper.
- `docs/governance.md` lists `resource` on findings.

## Public API

- Check report JSON: `members[].report.checks[].findings[]` gains
  `resource` (omitted when the match names none). `owner` is now set for every
  finding whose match references an owned resource or entry.
- No Go API changes outside `internal/`.

## Tests

- `internal/checkreport/owner_test.go`:
  `TestEveryFindingCarriesResourceAndOwner` covers 23 findings (3 past the
  witness limit, one waived) and checks that each has resource and owner, that
  only witnesses have arguments, that `refs` is not serialized, and that
  `ForOwner` keeps a finding past the limit. It fails before the fix.
