package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	bestPractice = "aws.#S3BucketBestPractice"
	pci          = "aws.#S3BucketPCI"
	bucket       = "aws.#S3Bucket"
)

func TestEvaluate(t *testing.T) {
	set := loadFixture(t)
	tests := []struct {
		name       string
		policy     string
		typeName   string
		mutate     func(r map[string]any)
		status     Status
		violations []Violation
		unknown    []Violation
		highest    Severity
	}{
		{
			name: "compliant", policy: pci, typeName: bucket,
			mutate: func(map[string]any) {},
			status: StatusCompliant,
		},
		{
			name: "extra fields never violate", policy: pci, typeName: bucket,
			mutate: func(r map[string]any) {
				r["Unmodelled"] = map[string]any{"deep": 1}
				r["PublicAccessBlock"].(map[string]any)["IgnorePublicAcls"] = false
				r["Tags"].([]any)[0].(map[string]any)["Extra"] = "x"
				r["Versioning"].(map[string]any)["MFADelete"] = "Disabled"
			},
			status: StatusCompliant,
		},
		{
			name: "conflict on an annotated rule", policy: bestPractice, typeName: bucket,
			mutate: func(r map[string]any) {
				r["PublicAccessBlock"].(map[string]any)["BlockPublicAcls"] = false
			},
			status: StatusNoncompliant,
			violations: []Violation{{Rule: "s3-block-public-acls", Severity: SeverityHigh,
				Path: "PublicAccessBlock.BlockPublicAcls", Kind: KindConflict, DeclaredBy: bestPractice}},
			highest: SeverityHigh,
		},
		{
			// The type already rejects a non-struct, so the type is the declarer.
			name: "conflict on a struct reports the field path", policy: bestPractice, typeName: bucket,
			mutate: func(r map[string]any) { r["ServerSideEncryption"] = "on" },
			status: StatusNoncompliant,
			violations: []Violation{{Rule: "ServerSideEncryption", Severity: DefaultSeverity,
				Path: "ServerSideEncryption", Kind: KindConflict, DeclaredBy: bucket}},
			highest: DefaultSeverity,
		},
		{
			name: "unannotated constraint uses its path as the rule", policy: bestPractice, typeName: bucket,
			mutate: func(r map[string]any) { r["Region"] = "ap-south-1"; r["Size"] = 500 },
			status: StatusNoncompliant,
			violations: []Violation{
				{Rule: "Region", Severity: DefaultSeverity, Path: "Region", Kind: KindConflict, DeclaredBy: bestPractice},
				{Rule: "Size", Severity: DefaultSeverity, Path: "Size", Kind: KindConflict, DeclaredBy: bestPractice},
			},
			highest: DefaultSeverity,
		},
		{
			name: "conflict with the type is attributed to the type", policy: bestPractice, typeName: bucket,
			mutate:     func(r map[string]any) { r["Size"] = "big" },
			status:     StatusNoncompliant,
			violations: []Violation{{Rule: "Size", Severity: DefaultSeverity, Path: "Size", Kind: KindConflict, DeclaredBy: bucket}},
			highest:    DefaultSeverity,
		},
		{
			name: "absent without opt-in is unknown", policy: bestPractice, typeName: bucket,
			mutate: func(r map[string]any) { delete(r, "ServerSideEncryption") },
			status: StatusUnknown,
			unknown: []Violation{{Rule: "s3-encryption", Severity: SeverityMedium,
				Path: "ServerSideEncryption.Algorithm", Kind: KindAbsent, DeclaredBy: bestPractice}},
		},
		{
			name: "absent nested field without opt-in is unknown", policy: bestPractice, typeName: bucket,
			mutate: func(r map[string]any) { r["ServerSideEncryption"] = map[string]any{} },
			status: StatusUnknown,
			unknown: []Violation{{Rule: "s3-encryption", Severity: SeverityMedium,
				Path: "ServerSideEncryption.Algorithm", Kind: KindAbsent, DeclaredBy: bestPractice}},
		},
		{
			name: "absent with opt-in is a violation per rule", policy: bestPractice, typeName: bucket,
			mutate: func(r map[string]any) { delete(r, "PublicAccessBlock") },
			status: StatusNoncompliant,
			violations: []Violation{
				{Rule: "s3-block-public-acls", Severity: SeverityHigh, Path: "PublicAccessBlock.BlockPublicAcls", Kind: KindAbsent, DeclaredBy: bestPractice},
				{Rule: "s3-restrict-public", Severity: SeverityHigh, Path: "PublicAccessBlock.RestrictPublicBuckets", Kind: KindAbsent, DeclaredBy: bestPractice},
			},
			highest: SeverityHigh,
		},
		{
			name: "absent opt-in inherited by a nested field", policy: bestPractice, typeName: bucket,
			mutate: func(r map[string]any) {
				delete(r["PublicAccessBlock"].(map[string]any), "RestrictPublicBuckets")
			},
			status: StatusNoncompliant,
			violations: []Violation{{Rule: "s3-restrict-public", Severity: SeverityHigh,
				Path: "PublicAccessBlock.RestrictPublicBuckets", Kind: KindAbsent, DeclaredBy: bestPractice}},
			highest: SeverityHigh,
		},
		{
			name: "required in a validated type is a violation", policy: bestPractice, typeName: bucket,
			mutate:     func(r map[string]any) { delete(r, "Region") },
			status:     StatusNoncompliant,
			violations: []Violation{{Rule: "Region", Severity: DefaultSeverity, Path: "Region", Kind: KindAbsent, DeclaredBy: bucket}},
			highest:    DefaultSeverity,
		},
		{
			name: "required in a validated ancestor of a custom type", policy: bestPractice, typeName: "aws.#S3BucketSpecial",
			mutate:     func(r map[string]any) { delete(r, "Region") },
			status:     StatusNoncompliant,
			violations: []Violation{{Rule: "Region", Severity: DefaultSeverity, Path: "Region", Kind: KindAbsent, DeclaredBy: bucket}},
			highest:    DefaultSeverity,
		},
		{
			name: "required in a draft type is unknown", policy: "aws.#DiskEncrypted", typeName: "aws.#Disk",
			mutate: func(r map[string]any) {
				for k := range r {
					delete(r, k)
				}
				r["Id"] = "d1"
			},
			status:  StatusUnknown,
			unknown: []Violation{{Rule: "Encrypted", Severity: DefaultSeverity, Path: "Encrypted", Kind: KindAbsent, DeclaredBy: "aws.#Disk"}},
		},
		{
			name: "extending policy reports inherited rules with their declarer", policy: pci, typeName: bucket,
			mutate: func(r map[string]any) {
				r["PublicAccessBlock"].(map[string]any)["BlockPublicAcls"] = false
				r["Versioning"] = map[string]any{"Status": "Suspended"}
				r["Tags"] = []any{map[string]any{"Key": "ok"}, map[string]any{"Key": "Bad Key"}}
			},
			status: StatusNoncompliant,
			violations: []Violation{
				{Rule: "s3-block-public-acls", Severity: SeverityHigh, Path: "PublicAccessBlock.BlockPublicAcls", Kind: KindConflict, DeclaredBy: bestPractice},
				{Rule: "tag-key", Severity: SeverityLow, Path: "Tags[*].Key", Kind: KindConflict, DeclaredBy: pci},
				{Rule: "s3-versioning", Severity: SeverityCritical, Path: "Versioning.Status", Kind: KindConflict, DeclaredBy: pci},
			},
			highest: SeverityCritical,
		},
		{
			// Key is required in the validated type's element shape.
			name: "absent field inside a list element", policy: pci, typeName: bucket,
			mutate:     func(r map[string]any) { r["Tags"] = []any{map[string]any{"Value": "v"}} },
			status:     StatusNoncompliant,
			violations: []Violation{{Rule: "tag-key", Severity: SeverityLow, Path: "Tags[*].Key", Kind: KindAbsent, DeclaredBy: pci}},
			highest:    SeverityLow,
		},
		{
			name: "violation and unknown together is noncompliant", policy: bestPractice, typeName: bucket,
			mutate:     func(r map[string]any) { r["Region"] = "nowhere"; delete(r, "ServerSideEncryption") },
			status:     StatusNoncompliant,
			violations: []Violation{{Rule: "Region", Severity: DefaultSeverity, Path: "Region", Kind: KindConflict, DeclaredBy: bestPractice}},
			unknown: []Violation{{Rule: "s3-encryption", Severity: SeverityMedium,
				Path: "ServerSideEncryption.Algorithm", Kind: KindAbsent, DeclaredBy: bestPractice}},
			highest: DefaultSeverity,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := set.Policy(tt.policy)
			require.True(t, ok)
			record := compliantBucket()
			tt.mutate(record)
			res, err := set.Evaluate(p, tt.typeName, record)
			require.NoError(t, err)
			assert.Equal(t, tt.status, res.Status)
			assert.Equal(t, tt.violations, res.Violations)
			assert.Equal(t, tt.unknown, res.Unknown)
			assert.Equal(t, tt.highest, res.HighestSeverity())
			assert.Equal(t, p.Digest, res.Digest)
		})
	}
}

func TestEvaluate_Provisional(t *testing.T) {
	set := loadFixture(t)
	disk, _ := set.Policy("aws.#DiskEncrypted")
	res, err := set.Evaluate(disk, "aws.#Disk", map[string]any{"Id": "d", "Encrypted": true})
	require.NoError(t, err)
	assert.Equal(t, StatusCompliant, res.Status)
	assert.True(t, res.Provisional, "the draft type makes the result provisional")

	bp, _ := set.Policy(bestPractice)
	res, err = set.Evaluate(bp, bucket, compliantBucket())
	require.NoError(t, err)
	assert.False(t, res.Provisional, "a validated type is not provisional")
}

func TestEvaluate_ViolationsNeverCarryValues(t *testing.T) {
	set := loadFixture(t)
	p, _ := set.Policy(pci)
	record := compliantBucket()
	record["Region"] = "secret-region-value"
	res, err := set.Evaluate(p, bucket, record)
	require.NoError(t, err)
	require.NotEmpty(t, res.Violations)
	for _, v := range res.Violations {
		assert.NotContains(t, v.Rule+v.Path+string(v.Severity)+v.DeclaredBy, "secret-region-value")
	}
}

func TestEvaluate_RejectsUnrelatedType(t *testing.T) {
	set := loadFixture(t)
	p, _ := set.Policy(bestPractice)
	_, err := set.Evaluate(p, "aws.#Disk", map[string]any{})
	assert.ErrorContains(t, err, "does not apply")
	_, err = set.Evaluate(p, "aws.#Nope", map[string]any{})
	assert.ErrorContains(t, err, "not loaded")
}

func TestApplicable(t *testing.T) {
	set := loadFixture(t)
	names := func(ps []*Policy) []string {
		var out []string
		for _, p := range ps {
			out = append(out, p.Name)
		}
		return out
	}
	assert.Equal(t, []string{bestPractice, pci}, names(set.Applicable(bucket)))
	assert.Equal(t, []string{bestPractice, pci}, names(set.Applicable("aws.#S3BucketSpecial")),
		"a policy on a type applies to the type's descendants")
	assert.Equal(t, []string{"aws.#DiskEncrypted"}, names(set.Applicable("aws.#Disk")))
	assert.Empty(t, set.Applicable(pci), "policies never apply to policies")
}
