package policy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/validator"
)

// awsTypes is a validated type (with a sensitive field), a custom subtype and
// a draft type.
const awsTypes = `package aws

#S3Bucket: {
	_pudl: {
		schema_type:      "base"
		resource_type:    "aws.s3.bucket"
		identity_fields:  ["Name"]
		sensitive_fields: ["Credentials.Secret"]
		review: {status: "validated", by: "sre-platform", at: "2026-11-01"}
	}
	Name:   string
	Region: string
	Credentials?: {Secret?: string, KeyId?: string}
	PublicAccessBlock?: {...}
	ServerSideEncryption?: {...}
	Versioning?: {...}
	Tags?: [...{Key: string, Value?: string}]
	Size?: int
	...
}

#S3BucketSpecial: {
	_pudl: {schema_type: "custom", resource_type: "aws.s3.bucket", base_schema: "aws.#S3Bucket"}
	Name: string
	...
}

#Disk: {
	_pudl: {schema_type: "base", resource_type: "aws.disk", identity_fields: ["Id"]}
	Id:        string
	Encrypted: bool
	...
}
`

// awsPolicies follows the design's authoring example. Policies are written
// standalone and linked by base_schema: embedding the type (#S3Bucket & {...})
// conflicts on _pudl.schema_type and does not load.
const awsPolicies = `package aws

#S3BucketBestPractice: {
	_pudl: {
		schema_type: "policy"
		base_schema: "aws.#S3Bucket"
		description: "Baseline S3 hardening"
	}
	PublicAccessBlock: {
		BlockPublicAcls:       true @pudl(rule="s3-block-public-acls", severity="high")
		RestrictPublicBuckets: true @pudl(rule="s3-restrict-public", severity="high", doc="Restrict public bucket policies")
	} @pudl(absent="violation")
	ServerSideEncryption: {
		Algorithm: "aws:kms" | "AES256" @pudl(rule="s3-encryption", severity="medium")
	}
	Region: "us-east-1" | "eu-west-1"
	Size?:  <100
}

#S3BucketPCI: {
	_pudl: {schema_type: "policy", base_schema: "aws.#S3BucketBestPractice"}
	Versioning: Status: "Enabled" @pudl(rule="s3-versioning", severity="critical")
	Tags: [...{Key: =~"^[a-z]+$" @pudl(rule="tag-key", severity="low")}]
	Credentials: KeyId: string
}

#DiskEncrypted: {
	_pudl: {schema_type: "policy", base_schema: "aws.#Disk"}
	Encrypted: true
}
`

// writeModule writes a CUE module with the given files under a fresh dir.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files["cue.mod/module.cue"] = "module: \"test.schemas\"\nlanguage: version: \"v0.14.0\"\n"
	for name, body := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	return dir
}

// loadDir loads the schemas in dir and the policy set over them.
func loadDir(t *testing.T, dir string) (*Set, []LoadError) {
	t.Helper()
	validator.ResetSharedLoaders()
	t.Cleanup(validator.ResetSharedLoaders)
	src, err := inference.NewSchemaInferrer(dir)
	require.NoError(t, err)
	require.Empty(t, src.LoadErrors())
	return Load(src)
}

// loadFixture loads the AWS fixture plus extra files and requires no policy
// load errors.
func loadFixture(t *testing.T) *Set {
	t.Helper()
	set, errs := loadDir(t, writeModule(t, map[string]string{
		"aws/types.cue":    awsTypes,
		"aws/policies.cue": awsPolicies,
	}))
	require.Empty(t, errs)
	return set
}

// compliantBucket satisfies both S3 policies.
func compliantBucket() map[string]any {
	return map[string]any{
		"Name":                 "logs",
		"Region":               "us-east-1",
		"PublicAccessBlock":    map[string]any{"BlockPublicAcls": true, "RestrictPublicBuckets": true},
		"ServerSideEncryption": map[string]any{"Algorithm": "aws:kms"},
		"Versioning":           map[string]any{"Status": "Enabled"},
		"Tags":                 []any{map[string]any{"Key": "team", "Value": "sre"}},
		"Credentials":          map[string]any{"KeyId": "k1", "Secret": "[REDACTED]"},
	}
}
