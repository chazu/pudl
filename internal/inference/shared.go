package inference

import (
	"strings"
	"sync"

	"github.com/chazu/pudl/internal/validator"
)

// Shared returns the memoized inferrer for a schema path list.
//
// The CUE compile itself is already shared per path (validator.SharedLoader), so
// what this adds is everything built *on top* of it: the merged schema and
// metadata maps with their first-found-wins shadowing, and the inheritance graph
// derived from them. Recommendation 6 names all three — "schema loading, CUE
// compilation, inheritance graphs, and identity metadata are reconstructed
// repeatedly" — and the loader memo alone would leave the last two rebuilt on
// every call.
//
// Invalidated by the same fingerprint the loaders use, rather than a second
// signal that could disagree with theirs.
var (
	sharedInferrersMu sync.Mutex
	sharedInferrers   = map[string]*sharedInferrer{}
)

type sharedInferrer struct {
	fingerprint string
	inferrer    *SchemaInferrer
}

// Shared builds or returns the inferrer for these schema paths.
//
// Callers that need an isolated inferrer — one whose state cannot be observed by
// anyone else — keep using NewSchemaInferrer.
func Shared(schemaPaths ...string) (*SchemaInferrer, error) {
	if len(schemaPaths) == 0 {
		return NewSchemaInferrer(schemaPaths...)
	}

	key := strings.Join(schemaPaths, "\x00")
	fingerprint := pathsFingerprint(schemaPaths)

	sharedInferrersMu.Lock()
	defer sharedInferrersMu.Unlock()

	if cached, ok := sharedInferrers[key]; ok && cached.fingerprint == fingerprint {
		return cached.inferrer, nil
	}

	inferrer, err := NewSchemaInferrer(schemaPaths...)
	if err != nil {
		// Not cached: a failed load is where the caller may have fixed the problem
		// between attempts.
		return nil, err
	}
	sharedInferrers[key] = &sharedInferrer{fingerprint: fingerprint, inferrer: inferrer}
	return inferrer, nil
}

// ResetShared drops the memo. For tests that stage schema directories at the
// same path across cases.
func ResetShared() {
	sharedInferrersMu.Lock()
	defer sharedInferrersMu.Unlock()
	sharedInferrers = map[string]*sharedInferrer{}
}

// pathsFingerprint concatenates each path's loader fingerprint. A path whose
// fingerprint cannot be computed contributes a sentinel, so an unreadable
// directory always looks changed and never serves a stale inferrer.
func pathsFingerprint(schemaPaths []string) string {
	var b strings.Builder
	for _, path := range schemaPaths {
		fingerprint, err := validator.SharedLoader(path).Fingerprint()
		if err != nil {
			fingerprint = "\x01unreadable"
		}
		b.WriteString(fingerprint)
		b.WriteByte(0)
	}
	return b.String()
}
