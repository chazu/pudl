package importer

import "fmt"

// identityTally counts records whose declared identity could not be extracted.
// The fallback (content-hash identity) is unchanged; the tally only makes it
// visible, because such records silently never form version chains.
type identityTally struct {
	count int
	first string
}

func (t *identityTally) record(schema string, err error) {
	t.count++
	if t.first == "" {
		t.first = fmt.Sprintf("%s: %v", schema, err)
	}
}

func (t *identityTally) apply(result *ImportResult) {
	result.IdentityUnresolved = t.count
	result.IdentityError = t.first
}
