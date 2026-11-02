Every member of a set is observe-only. A set continues independent branches
after a member failure while blocking that member's dependents; a blocked
member records the binding it could not resolve.

An interrupted set starts no further member: the member in progress concludes
`cancelled`, the remaining members are recorded `cancelled`, and the set's
status is `cancelled`.

With `--detailed-exitcode`, a set exits 2 when any member found drift or a
failing fail-severity check. A failed set exits 2 only when every failed member
failed on its checks alone (members blocked or cancelled behind them do not
count as separate errors); otherwise it exits 1.
