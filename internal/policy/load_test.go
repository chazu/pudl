package policy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_PolicyShape(t *testing.T) {
	set := loadFixture(t)
	require.Len(t, set.Policies(), 3)

	p, ok := set.Policy(pci)
	require.True(t, ok)
	assert.Equal(t, bucket, p.Type)
	assert.Equal(t, bestPractice, p.Extends)
	assert.Equal(t, []string{pci, bestPractice, bucket}, p.Chain)
	assert.True(t, strings.HasPrefix(p.Digest, "sha256:"))

	declared := map[string]string{}
	for _, r := range p.Rules {
		declared[r.Name] = r.DeclaredBy
	}
	assert.Equal(t, map[string]string{
		"s3-block-public-acls": bestPractice,
		"s3-encryption":        bestPractice,
		"s3-restrict-public":   bestPractice,
		"s3-versioning":        pci,
		"tag-key":              pci,
	}, declared, "an extending policy lists inherited rules under their declarer")

	bp, _ := set.Policy(bestPractice)
	assert.Equal(t, "", bp.Extends)
	for _, r := range bp.Rules {
		if r.Name == "s3-restrict-public" {
			assert.Equal(t, "Restrict public bucket policies", r.Doc)
			assert.Equal(t, []string{"PublicAccessBlock.RestrictPublicBuckets"}, r.Paths)
		}
	}
}

func TestLoad_Errors(t *testing.T) {
	policy := func(name, body string) string {
		return "#" + name + `: {
	_pudl: {schema_type: "policy", base_schema: "aws.#S3Bucket"}
	` + body + `
}
`
	}
	bad := "package aws\n\n" +
		policy("BadSeverity", `Name: string @pudl(rule="r1", severity="urgent")`) +
		policy("UnknownKey", `Name: string @pudl(rule="r2", sevrity="high")`) +
		policy("BadAbsent", `Name: string @pudl(absent="maybe")`) +
		policy("SeverityWithoutRule", `Name: string @pudl(severity="high")`) +
		policy("RuleRedeclared", `Name: string @pudl(rule="dup", severity="low")
	Region: string @pudl(rule="dup", severity="high")`) +
		policy("Sensitive", `Credentials: Secret: =~"^AKIA"`) +
		policy("SensitiveAnnotated", `Credentials: Secret: _ @pudl(rule="no-secret")`) +
		`#Floating: {_pudl: {schema_type: "policy"}, Name: string}
#Orphan: {_pudl: {schema_type: "policy", base_schema: "aws.#Floating"}, Name: string}
`
	set, errs := loadDir(t, writeModule(t, map[string]string{
		"aws/types.cue":    awsTypes,
		"aws/policies.cue": awsPolicies,
		"aws/bad.cue":      bad,
	}))
	got := map[string]string{}
	for _, e := range errs {
		got[e.Policy] = e.Error()
	}
	want := map[string]string{
		"aws.#BadSeverity":         `invalid severity "urgent"`,
		"aws.#UnknownKey":          `unknown @pudl key "sevrity"`,
		"aws.#BadAbsent":           `@pudl(absent="maybe")`,
		"aws.#SeverityWithoutRule": "need a rule name",
		"aws.#RuleRedeclared":      `rule "dup" redeclared`,
		"aws.#Sensitive":           "constrains Credentials.Secret, which is declared in sensitive_fields",
		"aws.#SensitiveAnnotated":  "constrains Credentials.Secret",
		"aws.#Floating":            "must declare _pudl.base_schema",
		"aws.#Orphan":              "does not reach a type schema",
	}
	require.Len(t, got, len(want), "errors: %v", got)
	for name, fragment := range want {
		assert.Contains(t, got[name], fragment, name)
		_, loaded := set.Policy(name)
		assert.False(t, loaded, "%s must not load", name)
	}
	assert.Len(t, set.Policies(), 3, "valid policies still load beside broken ones")
}

func TestDigest(t *testing.T) {
	digestOfPolicies := func(policies string) map[string]string {
		set, errs := loadDir(t, writeModule(t, map[string]string{
			"aws/types.cue":    awsTypes,
			"aws/policies.cue": policies,
		}))
		require.Empty(t, errs)
		out := map[string]string{}
		for _, p := range set.Policies() {
			out[p.Name] = p.Digest
		}
		return out
	}
	base := digestOfPolicies(awsPolicies)
	assert.Equal(t, base, digestOfPolicies(awsPolicies), "the same source digests the same")

	cosmetic := strings.Replace(awsPolicies, `description: "Baseline S3 hardening"`, `description: "Reworded"`, 1)
	cosmetic = strings.Replace(cosmetic, "#S3BucketPCI: {", "// PCI DSS controls\n#S3BucketPCI: {", 1)
	assert.Equal(t, base, digestOfPolicies(cosmetic), "comments and _pudl descriptions do not change the digest")

	stricter := strings.Replace(awsPolicies, `Size?:  <100`, `Size?:  <50`, 1)
	changed := digestOfPolicies(stricter)
	assert.NotEqual(t, base[bestPractice], changed[bestPractice])
	assert.NotEqual(t, base[pci], changed[pci], "an extending policy's digest covers its base policy")
	assert.Equal(t, base["aws.#DiskEncrypted"], changed["aws.#DiskEncrypted"])

	severity := strings.Replace(awsPolicies, `rule="s3-versioning", severity="critical"`, `rule="s3-versioning", severity="high"`, 1)
	assert.NotEqual(t, base[pci], digestOfPolicies(severity)[pci], "rule attributes are part of the digest")
}
