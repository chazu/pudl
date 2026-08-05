package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/systemmodel"
)

// redactSealedError keeps provider references operational inside generated mu
// configuration while preventing their paths from escaping through ordinary
// diagnostics or durable reports. The live approval review is the sole PUDL
// display that intentionally prints complete write destinations.
func redactSealedError(err error, model *systemmodel.SystemModel) error {
	if err == nil {
		return nil
	}
	return sealedRedactedError{cause: err, text: redactSealedText(err.Error(), model)}
}

type sealedRedactedError struct {
	cause error
	text  string
}

func (e sealedRedactedError) Error() string { return e.text }
func (e sealedRedactedError) Unwrap() error { return e.cause }

func redactSealedText(value string, model *systemmodel.SystemModel) string {
	if value == "" || model == nil {
		return value
	}
	refs := modelSealedReferences(model)
	// Replace longer refs first so one path cannot be partially exposed when a
	// provider namespace is also present in another declaration.
	sort.Slice(refs, func(i, j int) bool { return len(refs[i]) > len(refs[j]) })
	for _, ref := range refs {
		value = strings.ReplaceAll(value, ref, sealedReferenceLabel(ref))
	}
	return value
}

func modelSealedReferences(model *systemmodel.SystemModel) []string {
	seen := map[string]struct{}{}
	appendInputs := func(inputs map[string]systemmodel.SealedInput) {
		for _, input := range inputs {
			if input.Ref != "" {
				seen[input.Ref] = struct{}{}
			}
		}
	}
	appendOutputs := func(outputs map[string]systemmodel.SealedOutput) {
		for _, output := range outputs {
			if output.Ref != "" {
				seen[output.Ref] = struct{}{}
			}
		}
	}
	appendInputs(model.Populate.SealedInputs)
	if model.Converge != nil {
		appendInputs(model.Converge.SealedInputs)
		appendOutputs(model.Converge.SealedOutputs)
	}
	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	return refs
}

func sealedReferenceLabel(ref string) string {
	scheme := "sealed"
	if index := strings.IndexByte(ref, ':'); index > 0 {
		scheme = ref[:index]
	}
	digest := sha256.Sum256([]byte(ref))
	return fmt.Sprintf("<sealed-ref:%s:%s>", scheme, hex.EncodeToString(digest[:8]))
}
