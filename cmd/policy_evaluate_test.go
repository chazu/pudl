package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
)

const policyTypes = `package aws

#Bucket: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "aws.bucket"
		identity_fields: ["Name"]
	}
	Name: string
	...
}
`

// policyBP follows the design's authoring form; %s is the encryption rule's
// severity.
const policyBP = `package aws

#BucketBestPractice: {
	_pudl: {schema_type: "policy", base_schema: "pudl/aws.#Bucket"}
	PublicAccessBlock: {
		BlockPublicAcls: true @pudl(rule="block-public-acls", severity="high")
	} @pudl(absent="violation")
	Encryption: Algorithm: "aws:kms" @pudl(rule="encryption", severity="%s")
}
`

// policyBuckets: compliant, a conflict, an absence that is a violation, and
// an absence that is only unknown.
const policyBuckets = `[
	{"Name":"good","PublicAccessBlock":{"BlockPublicAcls":true},"Encryption":{"Algorithm":"aws:kms"}},
	{"Name":"open","PublicAccessBlock":{"BlockPublicAcls":false},"Encryption":{"Algorithm":"aws:kms"}},
	{"Name":"bare","Encryption":{"Algorithm":"aws:kms"}},
	{"Name":"plain","PublicAccessBlock":{"BlockPublicAcls":true}}
]`

// factCounts is every fact version and the latest write sequence: unchanged
// counts mean nothing was written.
func factCounts(t *testing.T, pudlDir string) [2]int64 {
	t.Helper()
	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	defer db.Close()
	var n, seq int64
	require.NoError(t, db.DB().QueryRow(`SELECT COUNT(*), COALESCE(MAX(tx_seq), 0) FROM facts`).Scan(&n, &seq))
	return [2]int64{n, seq}
}

func TestPolicyResultsPersistAtImportAndReevaluate(t *testing.T) {
	pudlDir := cliWorkspace(t)
	awsDir := filepath.Join(pudlDir, "schema", "pudl", "aws")
	writeFile(t, filepath.Join(awsDir, "bucket.cue"), policyTypes)
	writeFile(t, filepath.Join(awsDir, "policies.cue"), strings.Replace(policyBP, "%s", "medium", 1))
	writeFile(t, "buckets.json", policyBuckets)

	r := runCLI(t, "import", "buckets.json", "--schema", "pudl/aws.#Bucket")
	require.NoError(t, r.Err, r.Stderr)

	r = runCLI(t, "query", "policy_status", "status=noncompliant")
	require.NoError(t, r.Err, r.Stderr)
	assert.Equal(t, 2, strings.Count(r.Stdout, `"status":"noncompliant"`), r.Stdout)
	r = runCLI(t, "query", "policy_status", "status=unknown")
	require.NoError(t, r.Err, r.Stderr)
	assert.Equal(t, 1, strings.Count(r.Stdout, `"status":"unknown"`), r.Stdout)
	r = runCLI(t, "query", "policy_violation", "kind=absent")
	require.NoError(t, r.Err, r.Stderr)
	assert.Contains(t, r.Stdout, `"path":"PublicAccessBlock.BlockPublicAcls"`)
	assert.Contains(t, r.Stdout, `"declared_by":"pudl/aws.#BucketBestPractice"`)
	r = runCLI(t, "query", "policy_noncompliant", "severity=high")
	require.NoError(t, r.Err, r.Stderr)
	assert.Equal(t, 2, strings.Count(r.Stdout, `"severity":"high"`), "the built-in rule ships with the bootstrap schemas: "+r.Stdout+r.Stderr)

	r = runCLI(t, "doctor", "--only", "policy-results")
	assert.Contains(t, r.Stdout, "Repair policy-results: ok", r.Stderr)

	// A what-if draft writes nothing.
	before := factCounts(t, pudlDir)
	writeFile(t, "strict.cue", `package aws

#BucketStrict: {
	_pudl: {schema_type: "policy", base_schema: "pudl/aws.#Bucket"}
	Encryption: {
		Algorithm: "aws:kms" @pudl(rule="encryption-required", severity="critical")
	} @pudl(absent="violation")
}
`)
	r = runCLI(t, "policy", "evaluate", "--what-if", "strict.cue", "--json")
	require.NoError(t, r.Err, r.Stderr)
	var whatIf policyEvaluateOutput
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &whatIf), r.Stdout)
	assert.Equal(t, "what-if", whatIf.Mode)
	assert.Equal(t, 1, whatIf.ReportVersion)
	require.Len(t, whatIf.Policies, 1)
	assert.Equal(t, "aws.#BucketStrict", whatIf.Policies[0].Policy)
	assert.Equal(t, 3, whatIf.Policies[0].Compliant)
	assert.Equal(t, 1, whatIf.Policies[0].Noncompliant)
	require.NotEmpty(t, whatIf.TopViolations)
	assert.Equal(t, "encryption-required", whatIf.TopViolations[0].Rule)
	assert.Equal(t, before, factCounts(t, pudlDir), "--what-if never writes facts")

	// The policy gets stricter: stored results are stale until re-evaluated.
	writeFile(t, filepath.Join(awsDir, "policies.cue"), strings.Replace(policyBP, "%s", "critical", 1))
	reloadSchemas()
	r = runCLI(t, "doctor", "--only", "policy-results")
	assert.Contains(t, r.Stdout, "4 policy results are stale", r.Stderr)

	before = factCounts(t, pudlDir)
	r = runCLI(t, "policy", "evaluate", "--dry-run")
	require.NoError(t, r.Err, r.Stderr)
	assert.Contains(t, r.Stdout, "Results: 4 would update")
	assert.Equal(t, before, factCounts(t, pudlDir), "--dry-run never writes facts")

	r = runCLI(t, "policy", "evaluate")
	require.NoError(t, r.Err, r.Stderr)
	assert.Contains(t, r.Stdout, "pudl/aws.#BucketBestPractice (pudl/aws.#Bucket): 1 compliant, 2 noncompliant, 1 unknown")
	assert.Contains(t, r.Stdout, "Results: 4 updated")
	r = runCLI(t, "doctor", "--only", "policy-results")
	assert.Contains(t, r.Stdout, "Repair policy-results: ok", r.Stderr)

	// Current facts carry the new digest; history keeps the old one.
	db, err := database.NewCatalogDB(pudlDir)
	require.NoError(t, err)
	defer db.Close()
	current, err := db.QueryFacts(database.FactFilter{Relation: database.PolicyStatusRelation})
	require.NoError(t, err)
	history, err := db.FactHistory(database.PolicyStatusRelation)
	require.NoError(t, err)
	assert.Len(t, current, 4)
	assert.Greater(t, len(history), len(current), "re-evaluation invalidates the previous results instead of deleting them")
	digests := map[string]bool{}
	for _, f := range history {
		var args map[string]string
		require.NoError(t, json.Unmarshal([]byte(f.Args), &args))
		digests[args["policy_digest"]] = true
	}
	assert.Len(t, digests, 2)
}

func TestPolicyFactsAreReservedForEvaluation(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "facts", "add", "--relation", "policy_status", "--args", `{"resource":"r","policy":"p","status":"compliant"}`)
	require.Error(t, r.Err)
	assert.Contains(t, r.Stderr, `relation "policy_status" is reserved`)
}
