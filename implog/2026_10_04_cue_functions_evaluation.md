# CUE functions experiment evaluation (2026-10-04)

## Summary

Research-only task: investigated the `@experiment(functions)` feature in
`cuelang.org/go v0.18.0-alpha.2` (proposal cue-lang/cue#4484) and how it could
simplify pudl. Full report:
[docs/research/cue-functions-experiment.md](../docs/research/cue-functions-experiment.md).

## Work done

- Read the experiment's definition (`internal/cueexperiment/file.go`), its
  evaluator test suite (`cue/testdata/functions/*.txtar`), the Go API tests,
  the proposal, and the release notes.
- Prototyped with the alpha `cue` CLI: typed datalog rule constructors,
  `_pudl` metadata helpers, git-family specialisation, `#Check` constructors,
  CUE-side identity keys, and a `#SystemModel` factory.
- Ran the rule prototype through pudl's unmodified `datalog.ParseRules`, built
  against the alpha. It parses correctly.
- Upgraded pudl to the alpha in a throwaway worktree. It builds unchanged. The
  stable `explicitopen` semantics break `#SystemModel` (22 tests) until
  `#SealedInputs...`/`#SealedExecution...` are used. 13 test files pin
  language `v0.14.0`, which is too old for the needed attribute. With both
  fixes, results match the v0.16 baseline.

## Public API

None. No production code, schemas, or `go.mod` were changed. The upgrade
experiments ran in a scratch worktree and were discarded.

## Key conclusions

- Functions help *authoring* (rules, checks, model factories, metadata) and
  catch rule arg-key typos at compile time.
- They do not replace Go engines: there is no Go call API and no Go bridge
  yet, and recursion is forbidden.
- Recommendations R1–R7 are in the report. R1–R3 are worthwhile independent
  of functions.
