package clidocs

//go:generate go run ./gen

// ReferencePath is the generated reference, relative to the repository root.
const ReferencePath = "docs/cli-reference.md"

// NotesDir holds the hand-written per-command notes, relative to the
// repository root.
const NotesDir = "docs/cli/notes"
