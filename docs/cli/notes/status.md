Status is recorded per model target by `pudl check`. `clean` means a live
observation matched `desired` and no fail-severity check failed; it is never
written by a `--from-catalog` replay. `drifted` means drift or a failing
fail-severity check. `unknown` means no verdict: the model has no `desired`
state, has not been observed, or its last check could not establish one (an
incomplete inventory, a check without evidence). `failed` and the value
`converging` appear only on rows written by retired commands; `converging`
reads as `unknown`, and the next check replaces both.
