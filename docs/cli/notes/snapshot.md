Each observation is one snapshot with a scope and a completeness flag (see
[observation](observation-scope.md)). `snapshot show NEW --compare OLD`
compares two snapshots of one scope: a resource missing from NEW is `removed`
only when NEW is complete, and `not-observed` otherwise. A snapshot can be
pinned by reports (for 30 days), by `snapshot retain`, or by legacy
approvals; `prune` removes none of them, nor a model's latest successful
observation. See [evidence lifetime](evidence.md#reports-and-evidence-lifetime).
