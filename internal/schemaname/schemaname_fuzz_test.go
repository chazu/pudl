package schemaname

import "testing"

// FuzzNormalizeIdempotent checks Normalize never panics and that normalizing a
// canonical name leaves it unchanged, so stored names and freshly normalized
// names always compare equal.
func FuzzNormalizeIdempotent(f *testing.F) {
	for _, seed := range []string{
		"pudl.schemas/aws/ec2@v0:#Instance",
		"pudl.schemas/aws/ec2:#Instance",
		"aws/ec2:#Instance",
		"aws/ec2.#Instance",
		"core.#Item",
		"core.Item",
		"mu/aws@v1#EC2Instance",
		"a:b:c",
		"a.#b.c",
		"x.",
		".",
		":",
		"",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		once := Normalize(name)
		if twice := Normalize(once); twice != once {
			t.Fatalf("Normalize not idempotent: %q -> %q -> %q", name, once, twice)
		}
	})
}
