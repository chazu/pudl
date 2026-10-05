`unknown` means there is no verified terminal status, including when an
external apply completed but PUDL could not persist its manifest receipt, and
when the only clean observation came from an `--only` run that covered part of
the model. The run row (`runs`) distinguishes these: it carries the run's real
verdict and a note.

Lifecycle: `drifted → converging` (apply, via `ingest-manifest`) `→ clean`
(verified ∅ by the drift re-check) `| failed`. `clean` is the single in-sync
state (drift == ∅), written only off an actual observation with successful
receipt persistence — never a bare apply.
