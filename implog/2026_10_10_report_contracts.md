# Report contracts and check evaluation service

Version-2 standalone and set reports expose execution, conformity, check and
verification axes, scope, counts, evidence and next actions. Set members include
summaries directly. Convergence reports retain their final differential
observation; an inventory check cannot mistake a set's preflight snapshot for
post-apply evidence. Early --json failures produce one structured error document.
--progress-json emits stderr events without contaminating the result document.

Moved check evaluation to internal/checks: explicit catalog, schema registry,
rule paths, payload reader, evidence selection, redaction and progress callback.
Cobra retains presentation and bounded witness rendering. This is a focused
extraction, not a rewrite of the remaining run adapters.

Validation: command and acute package tests passed, including early error JSON,
unknown evidence, aggregate member results, replay/uncertain mutation summaries,
and progress/result separation. Version-1 report readers remain supported.
