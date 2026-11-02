# Vision

pudl is an evidence-backed inventory and governance tool for SREs and the
coding agents that work beside them. It answers three questions about
infrastructure, and every answer points back to the evidence behind it:

- **What exists?** Collectors run the read-only CLIs teams already trust and
  record what they print as scoped snapshots. Each snapshot records whether it
  saw everything in its scope.
- **What is it?** CUE types classify each record and give each resource a
  stable identity, so the catalog keeps every resource's history.
- **What is wrong with it?** Policies describe good resources, Datalog checks
  describe good relationships, and reports track each finding over time with
  its owner and any waiver.

## Principles

- **Observe, never change.** pudl reads and reports. Terraform, `kubectl`,
  consoles and people change infrastructure; pudl's next check verifies the
  result.
- **Evidence over assertion.** Incomplete evidence makes a result unknown. It
  never makes a resource look absent and never resolves a finding.
- **Knowledge lives in CUE.** Vendor knowledge (types, policies, collectors,
  rules) ships as CUE content packs, not Go code. A team can read, review and
  version it like any other code.
- **Local and explicit.** One binary, one SQLite catalog per repository, and
  commands that run exactly what you name.
- **Agent-ready.** Every command has `--json` output and stable IDs. pudl
  ships with agent instructions (`pudl prime`, the bundled skill).

## Out of scope

- Changing infrastructure: applying, converging, remediating or approving
  changes. Convergence and the mu integration were removed for this reason;
  see [the removal design](design/2026-11-01-remove-convergence-and-mu.md).
- Holding secrets. Sensitive fields are redacted before storage, and
  `pudl doctor` warns about likely secrets.

## Direction

- More content packs, such as Kubernetes, Azure and Terraform state.
- Correlation across providers, for example AWS and Kubernetes.
- Catalog summaries: schema coverage, counts, outliers and sprawl.

[plan.md](plan.md) records what has been built and what comes next.
