Observe-only sets continue independent branches after a member failure while
blocking its dependents. Mutating sets complete read-only preflight for every
member before the first mutation and stop new mutations after the first apply
failure. Non-sealed sets can pause with `--require-approval`; resume revalidates
the immutable request and plan digest before execution.

Mu's version-2 `build --plan --json` output includes each action's complete
execution identity, resolved plugin identities, and sealed input/output claims.
PUDL validates those claims under strict routing before persisting an exact
approval. Unused declarations, undeclared claims, mismatched refs/modes, and
ambiguous output writers fail before mutation or provider traffic. Any run-set
that can write a sealed output is approval-gated automatically;
`--require-approval` adds the same gate to other mutating sets. Resume rebuilds
and compares the normalized exact plan; immediately before every apply, mu
compares the raw same-workspace digest before provider access and executes that
same in-memory graph.

Each mutating member concludes as a standalone converge does: its checks run,
its verdict is written to the model's status row, and a verified clean promotes
its `converging` resources. A failing fail-severity check fails the member, which
stops the set's later mutations.

An interrupted set starts no further member: the member in progress concludes
`cancelled` (with needs-verification if it was applying), the remaining members
are recorded `cancelled`, and the set's status is `cancelled`.

With `--detailed-exitcode`, an observe-only set exits 2 when any member found
drift, pending changes or a failing fail-severity check. A converging set exits
0 once every member converged clean. A failed set exits 2 only when every
failed member failed on its checks alone (members blocked or cancelled behind
them do not count as separate errors); otherwise it exits 1.
