// PUDL workspace configuration.
// This file marks the root of a per-repo PUDL workspace.

// Workspace name — used as the origin for catalog entries in this repo's
// .pudl/data/sqlite/catalog.db.
name: "pudl"

// Optional: override toolchain mappings for this workspace.
// These take priority over global config and built-in defaults.
// toolchain_mappings: [
//     {prefix: "myapp", toolchain: "shell"},
// ]

// Optional: restrict provider references that sealed outputs may write.
// Omit for mu compatibility mode; set [] for explicit deny-all.
// secrets: writable_refs: ["pass:myproject/*"]
