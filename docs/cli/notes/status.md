`unknown` means there is no verified terminal status: the model has not been
observed, its last run could not establish a verdict (an incomplete inventory or
a check without evidence), or the row still holds the `converging` value written
by the retired convergence workflow. The run row (`runs`) carries each run's
real verdict and a note.

`clean` is the single in-sync state (drift == ∅), written only off a live
observation, never off a `--from-catalog` replay. `failed` appears only on rows
written by the retired convergence workflow; the next run replaces it.
