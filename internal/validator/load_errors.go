package validator

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrNoInstances reports a schema path that contains no CUE packages at all.
// It is a property of the path, not a broken package, so tolerant consumers
// treat it as "nothing to load" rather than as a load error.
var ErrNoInstances = errors.New("no CUE instances found in schema module")

// SchemaLoadError records one schema package that could not be loaded.
//
// One broken package used to fail its whole schema path, and consumers skipped
// the failed path silently, so every import fell back to the catch-all schema
// with nothing said. Loading is now per package: the good packages load, and
// each broken one is reported here so callers can surface it.
type SchemaLoadError struct {
	// Path is the schema root the package was loaded from.
	Path string `json:"path"`
	// Package is the schema-root-relative package directory (for example
	// "pudl/k8s"). Empty when the whole path failed to load.
	Package string `json:"package,omitempty"`
	// Err is the underlying load, build or integrity error.
	Err error `json:"-"`
}

// Error implements error. The text names the package and schema root so a
// one-line report is enough to find the broken file.
func (e SchemaLoadError) Error() string {
	if e.Package == "" {
		return fmt.Sprintf("schema path %s: %v", e.Path, e.Err)
	}
	return fmt.Sprintf("schema package %s (in %s): %v", e.Package, e.Path, e.Err)
}

// Unwrap exposes the underlying error to errors.Is and errors.As.
func (e SchemaLoadError) Unwrap() error { return e.Err }

// FindBaseSchemaCycles returns every base_schema cycle in metadata, each as the
// schema names walked from the cycle's smallest member back to itself
// (A → B → A). Cycles are returned in a deterministic order.
//
// A cycle makes "walk to the family root" undefined: the chain validator would
// loop forever and the inheritance graph would pick an arbitrary root.
func FindBaseSchemaCycles(metadata map[string]SchemaMetadata) [][]string {
	reported := map[string]bool{}
	var cycles [][]string

	names := make([]string, 0, len(metadata))
	for name := range metadata {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, start := range names {
		// Walk the parent chain from start, remembering the position of each
		// schema; revisiting one means the walk entered a cycle there.
		position := map[string]int{}
		var walk []string
		current := start
		for {
			if at, seen := position[current]; seen {
				cycle := canonicalCycle(walk[at:])
				key := strings.Join(cycle, "\x00")
				if !reported[key] {
					reported[key] = true
					cycles = append(cycles, cycle)
				}
				break
			}
			meta, exists := metadata[current]
			if !exists || meta.BaseSchema == "" {
				break
			}
			position[current] = len(walk)
			walk = append(walk, current)
			current = meta.BaseSchema
		}
	}

	sort.Slice(cycles, func(i, j int) bool { return cycles[i][0] < cycles[j][0] })
	return cycles
}

// canonicalCycle rotates a cycle's members so it starts at its smallest name
// and closes back on it, giving each cycle exactly one spelling.
func canonicalCycle(members []string) []string {
	smallest := 0
	for i, name := range members {
		if name < members[smallest] {
			smallest = i
		}
	}
	cycle := make([]string, 0, len(members)+1)
	cycle = append(cycle, members[smallest:]...)
	cycle = append(cycle, members[:smallest]...)
	return append(cycle, members[smallest])
}

// FormatCycle renders a cycle as "A → B → A".
func FormatCycle(cycle []string) string {
	return strings.Join(cycle, " → ")
}
