package pithdriver

import (
	"github.com/chazu/pith"

	"github.com/chazu/pudl/internal/drift"
)

// driftWords returns the drift driver vocabulary.
func driftWords() map[string]pith.Word {
	return map[string]pith.Word{
		"diff": wordDriftDiff(),
	}
}

// drift/diff ( declared live -- [diffs] )
// Compares two maps and returns field-level differences.
// Each diff has path, type ("changed"/"added"/"removed"), declared, and live.
func wordDriftDiff() pith.Word {
	return func(vm *pith.VM) error {
		live, err := popMap(vm)
		if err != nil {
			return err
		}
		declared, err := popMap(vm)
		if err != nil {
			return err
		}
		diffs := drift.Compare(declared, live)
		items, err := structsToMaps(diffs)
		if err != nil {
			return err
		}
		vm.Push(items)
		return nil
	}
}
