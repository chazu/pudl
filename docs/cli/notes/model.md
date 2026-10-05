A model is a registered `#SystemModel` describing a `desired` state and the
sources that populate observed state. Catalog rows carry a `target` — the mu
target or run target that produced them (e.g. `//models/<name>`, a populate
phase `//models/<name>:populate`, or a standalone observe like `home/odroid`);
status is recorded per target. Drift detection is a phase of `pudl run`, not a
standalone command.
