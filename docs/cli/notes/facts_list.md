`--as-of-valid` asks what was true at a moment; `--as-of-tx` asks what the store
believed at a moment, in whole seconds, after every write during or before that
second. `--as-of-tx-seq` selects an exact write in the store's transaction
sequence, which also shows facts added and retracted within one second. See
[facts](facts.md) for the bitemporal model.
