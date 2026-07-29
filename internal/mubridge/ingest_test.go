package mubridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/validator"
)

func setupIngestTestDB(t *testing.T) (*database.CatalogDB, string) {
	t.Helper()
	tmpDir := t.TempDir()
	dbDir := filepath.Join(tmpDir, "db")
	db, err := database.NewCatalogDB(dbDir)
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	dataDir := filepath.Join(tmpDir, "data")
	return db, dataDir
}

func TestIngestObserveResults_Basic(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	// mu observe --json output: array of ObserveResult with current.records
	input := `[
		{
			"target": "//home/odroid",
			"current": {
				"records": [
					{"_schema": "linux.host", "hostname": "renge", "kernel": "5.10.0", "arch": "aarch64", "os": {"id": "debian", "version": "10", "name": "Debian"}, "uptime_seconds": 12114},
					{"_schema": "linux.package", "host": "renge", "name": "acl", "version": "2.2.53-4", "status": "ii "},
					{"_schema": "linux.package", "host": "renge", "name": "adduser", "version": "3.118", "status": "ii "},
					{"_schema": "linux.service", "host": "renge", "unit": "ssh.service", "active": "active", "sub": "running"}
				]
			}
		}
	]`

	countResult, err := IngestObserve(db, ObserveIngest{Reader: strings.NewReader(input), Origin: "mu-observe", DataDir: dataDir, Graph: nil})
	count := countResult.Records
	if err != nil {
		t.Fatalf("IngestObserveResults failed: %v", err)
	}
	if count != 4 {
		t.Errorf("expected 4 ingested records, got %d", count)
	}

	// All records should be stored as observe entries for target "home/odroid"
	entry, err := db.GetLatestObserve("home/odroid")
	if err != nil {
		t.Fatalf("GetLatestObserve failed: %v", err)
	}
	if entry == nil {
		t.Fatal("expected observe entry for home/odroid")
	}
	if entry.EntryType == nil || *entry.EntryType != "observe" {
		t.Errorf("expected entry_type 'observe', got %v", entry.EntryType)
	}
	if entry.Origin != "mu-observe" {
		t.Errorf("expected origin 'mu-observe', got %s", entry.Origin)
	}

	// Records should be members of a snapshot collection
	if entry.CollectionID == nil {
		t.Fatal("expected record to be a member of a collection")
	}
	if entry.CollectionType == nil || *entry.CollectionType != "item" {
		t.Errorf("expected collection_type 'item', got %v", entry.CollectionType)
	}

	// The snapshot collection should exist
	snapshot, err := db.GetCollectionByID(*entry.CollectionID)
	if err != nil {
		t.Fatalf("GetCollectionByID failed: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected snapshot collection entry")
	}
	if snapshot.Schema != "pudl/mu.#ObserveSnapshot" {
		t.Errorf("expected snapshot schema, got %s", snapshot.Schema)
	}
	if snapshot.RecordCount != 4 {
		t.Errorf("expected snapshot record_count 4, got %d", snapshot.RecordCount)
	}

	// Snapshot should have 4 items
	items, err := db.GetCollectionItems(*entry.CollectionID)
	if err != nil {
		t.Fatalf("GetCollectionItems failed: %v", err)
	}
	if len(items) != 4 {
		t.Errorf("expected 4 collection items, got %d", len(items))
	}
}

func TestIngestObserveResults_SchemaRouting(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()
	graph := inference.BuildInheritanceGraph(map[string]validator.SchemaMetadata{
		"pudl/linux.#Host":    {ResourceType: "linux.host"},
		"pudl/linux.#Service": {ResourceType: "linux.service"},
	})

	input := `[
		{
			"target": "//home/renge",
			"current": {
				"records": [
					{"_schema": "linux.host", "hostname": "renge", "kernel": "5.10.0", "arch": "aarch64", "os": {"id": "debian", "version": "10", "name": "Debian"}, "uptime_seconds": 100},
					{"_schema": "linux.service", "host": "renge", "unit": "cron.service", "active": "active", "sub": "running"}
				]
			}
		}
	]`

	countResult, err := IngestObserve(db, ObserveIngest{Reader: strings.NewReader(input), Origin: "mu-observe", DataDir: dataDir, Graph: graph})
	count := countResult.Records
	if err != nil {
		t.Fatalf("IngestObserveResults failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 records, got %d", count)
	}

	// Query observe item entries (not the snapshot collection) and check schema routing
	entries, err := db.QueryEntries(
		database.FilterOptions{EntryTypes: []string{"observe"}, CollectionType: "item"},
		database.QueryOptions{Limit: 100},
	)
	if err != nil {
		t.Fatalf("QueryEntries failed: %v", err)
	}
	if entries.FilteredCount != 2 {
		t.Fatalf("expected 2 item entries, got %d", entries.FilteredCount)
	}

	schemas := map[string]bool{}
	for _, e := range entries.Entries {
		schemas[e.Schema] = true
	}
	if !schemas["pudl/linux.#Host"] {
		t.Error("expected pudl/linux.#Host schema in results")
	}
	if !schemas["pudl/linux.#Service"] {
		t.Error("expected pudl/linux.#Service schema in results")
	}
}

func TestIngestObserveResultsWithSnapshotRunID_AttachesAuditIdentity(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	input := `[{"target":"//app","current":{"records":[{"_schema":"linux.host","hostname":"box"}]}}]`
	result, err := IngestObserve(db, ObserveIngest{
		Reader: strings.NewReader(input), Origin: "mu-observe", DataDir: dataDir,
		RunID: "run_test", SnapshotID: "snap_test",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Records)
	snapshotID := result.SnapshotID
	assert.Equal(t, "snap_test", snapshotID, "the pre-allocated id is what lands")

	entry, err := db.GetLatestObserve("app")
	require.NoError(t, err)
	require.NotNil(t, entry)
	require.NotNil(t, entry.RunID)
	assert.Equal(t, "run_test", *entry.RunID)

	snapshot, err := db.GetCollectionByID(snapshotID)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.RunID)
	assert.Equal(t, "run_test", *snapshot.RunID)

	// A second run observing the same thing must NOT steal the record. The
	// association is first-writer-wins: rewriting run_id here made it
	// last-writer-wins, so a query for run_test under-reported what run_test saw
	// and the entry's run disagreed with its snapshot membership about the same
	// fact (invariant 3).
	next, err := IngestObserve(db, ObserveIngest{
		Reader: strings.NewReader(input), Origin: "mu-observe", DataDir: dataDir,
		RunID: "run_next",
	})
	require.NoError(t, err)
	nextSnapshotID := next.SnapshotID
	entry, err = db.GetLatestObserve("app")
	require.NoError(t, err)
	require.NotNil(t, entry.RunID)
	assert.Equal(t, "run_test", *entry.RunID, "the record keeps the run that first observed it")

	// The second run's sighting is not lost — it is snapshot membership, which is
	// the relationship that is legitimately many-to-many.
	require.NotEqual(t, snapshotID, nextSnapshotID, "each run gets its own snapshot")
	nextSnapshot, err := db.GetCollectionByID(nextSnapshotID)
	require.NoError(t, err)
	require.NotNil(t, nextSnapshot.RunID)
	assert.Equal(t, "run_next", *nextSnapshot.RunID)

	memberships, err := db.ItemMembershipCount(entry.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, memberships, "the shared record belongs to both runs' snapshots")
}

func TestIngestObserveResults_Dedup(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	input := `[{"target":"//app","current":{"records":[{"_schema":"linux.host","hostname":"box","kernel":"6.0","arch":"x86_64","os":{"id":"ubuntu","version":"22.04","name":"Ubuntu"},"uptime_seconds":1}]}}]`

	count1Result, err := IngestObserve(db, ObserveIngest{Reader: strings.NewReader(input), Origin: "mu-observe", DataDir: dataDir, Graph: nil})
	count1 := count1Result.Records
	if err != nil {
		t.Fatalf("first ingest failed: %v", err)
	}
	if count1 != 1 {
		t.Errorf("expected 1 on first ingest, got %d", count1)
	}

	// Same data again — should deduplicate
	count2Result, err := IngestObserve(db, ObserveIngest{Reader: strings.NewReader(input), Origin: "mu-observe", DataDir: dataDir, Graph: nil})
	count2 := count2Result.Records
	if err != nil {
		t.Fatalf("second ingest failed: %v", err)
	}
	if count2 != 0 {
		t.Errorf("expected 0 on duplicate ingest, got %d", count2)
	}
}

func TestIngestObserveResults_EmptyInput(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	countResult, err := IngestObserve(db, ObserveIngest{Reader: strings.NewReader(""), Origin: "mu-observe", DataDir: dataDir, Graph: nil})
	count := countResult.Records
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 for empty input, got %d", count)
	}
}

func TestIngestObserveResults_TargetError(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	// Targets with errors should be skipped, not ingested
	input := `[
		{"target":"//broken","error":"plugin crashed"},
		{"target":"//ok","current":{"records":[{"_schema":"linux.host","hostname":"good","kernel":"6.0","arch":"x86_64","os":{"id":"ubuntu","version":"22.04","name":"Ubuntu"},"uptime_seconds":1}]}}
	]`

	countResult, err := IngestObserve(db, ObserveIngest{Reader: strings.NewReader(input), Origin: "mu-observe", DataDir: dataDir, Graph: nil})
	count := countResult.Records
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 (skipping errored target), got %d", count)
	}
}

func TestIngestObserveResults_NoRecordsKey(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	// current without records key — treat whole current as single record
	input := `[{"target":"//simple","current":{"status":"healthy","uptime":42}}]`

	countResult, err := IngestObserve(db, ObserveIngest{Reader: strings.NewReader(input), Origin: "mu-observe", DataDir: dataDir, Graph: nil})
	count := countResult.Records
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 record, got %d", count)
	}

	entry, err := db.GetLatestObserve("simple")
	if err != nil {
		t.Fatalf("GetLatestObserve failed: %v", err)
	}
	if entry == nil {
		t.Fatal("expected observe entry for simple")
	}
	// No _schema field, should fall back to generic observe result
	if entry.Schema != "pudl/mu.#ObserveResult" {
		t.Errorf("expected fallback schema, got %s", entry.Schema)
	}
}

func TestIngestObserveResults_MultipleTargets(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	input := `[
		{"target":"//host/a","current":{"records":[{"_schema":"linux.host","hostname":"a","kernel":"6.0","arch":"x86_64","os":{"id":"ubuntu","version":"22.04","name":"Ubuntu"},"uptime_seconds":1}]}},
		{"target":"//host/b","current":{"records":[{"_schema":"linux.host","hostname":"b","kernel":"6.0","arch":"x86_64","os":{"id":"ubuntu","version":"22.04","name":"Ubuntu"},"uptime_seconds":2}]}}
	]`

	countResult, err := IngestObserve(db, ObserveIngest{Reader: strings.NewReader(input), Origin: "mu-observe", DataDir: dataDir, Graph: nil})
	count := countResult.Records
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 records (one per target), got %d", count)
	}

	// Both targets should have entries
	e1, _ := db.GetLatestObserve("host/a")
	e2, _ := db.GetLatestObserve("host/b")
	if e1 == nil {
		t.Error("expected observe entry for host/a")
	}
	if e2 == nil {
		t.Error("expected observe entry for host/b")
	}
}

func TestIngestObserveResults_CustomOrigin(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	input := `[{"target":"//app","current":{"records":[{"_schema":"linux.host","hostname":"x","kernel":"6.0","arch":"x86_64","os":{"id":"ubuntu","version":"22.04","name":"Ubuntu"},"uptime_seconds":1}]}}]`

	countResult, err := IngestObserve(db, ObserveIngest{Reader: strings.NewReader(input), Origin: "custom-origin", DataDir: dataDir, Graph: nil})
	count := countResult.Records
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}

	entry, _ := db.GetLatestObserve("app")
	if entry == nil {
		t.Fatal("expected entry")
	}
	if entry.Origin != "custom-origin" {
		t.Errorf("expected origin 'custom-origin', got %s", entry.Origin)
	}
}

func TestResourceTypeToSchema(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"linux.host", "pudl/linux.#Host"},
		{"linux.package", "pudl/linux.#Package"},
		{"linux.network_interface", "pudl/linux.#NetworkInterface"},
		{"linux.service", "pudl/linux.#Service"},
		{"linux.filesystem", "pudl/linux.#Filesystem"},
		{"linux.user", "pudl/linux.#User"},
		{"aws.ec2.instance", "pudl/aws.#Instance"},
		{"aws.ec2.vpc", "pudl/aws.#VPC"},
		{"aws.ec2.nat_gateway", "pudl/aws.#NATGateway"},
		{"aws.ec2.network_acl", "pudl/aws.#NetworkACL"},
		{"unknown", "pudl/mu.#ObserveResult"}, // no dot separator
	}

	for _, tt := range tests {
		got := resourceTypeToSchema(tt.input)
		if got != tt.expected {
			t.Errorf("resourceTypeToSchema(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestResolveObserveSchemaRequiresLoadedSchema(t *testing.T) {
	graph := inference.BuildInheritanceGraph(map[string]validator.SchemaMetadata{
		"pudl/linux.#Host": {ResourceType: "linux.host"},
	})

	assert.Equal(t, "pudl/linux.#Host", resolveObserveSchema(
		map[string]any{"_schema": "linux.host"}, graph, nil,
	))
	assert.Equal(t, genericObserveSchema, resolveObserveSchema(
		map[string]any{"_schema": "aws.ec2.vpc"}, graph, nil,
	))
	assert.Equal(t, genericObserveSchema, resolveObserveSchema(
		map[string]any{"_schema": "not-a-resource"}, graph, nil,
	))
	assert.Equal(t, genericObserveSchema, resolveObserveSchema(
		map[string]any{"_schema": "linux.host"}, nil, nil,
	), "a missing schema graph must not create a dangling ref")
}

func TestIngestObserveResults_KubernetesInventorySchema(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	graph := inference.BuildInheritanceGraph(map[string]validator.SchemaMetadata{
		"pudl/k8s.#Resource": {ResourceType: "k8s.resource"},
	})
	input := `[{"target":"//cluster","current":{"records":[{
		"_schema":"k8s.resource",
		"apiVersion":"v1",
		"kind":"Pod",
		"metadata":{"name":"web","namespace":"default"},
		"spec":{"containers":[{"name":"web","image":"example/web:1"}]}
	}]}}]`

	result, err := IngestObserve(db, ObserveIngest{
		Reader: strings.NewReader(input), Origin: "mu-k8s-inventory", DataDir: dataDir, Graph: graph,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Records)

	entries, err := db.QueryEntries(
		database.FilterOptions{EntryTypes: []string{"observe"}, CollectionType: "item"},
		database.QueryOptions{Limit: 10},
	)
	require.NoError(t, err)
	require.Len(t, entries.Entries, 1)
	assert.Equal(t, "pudl/k8s.#Resource", entries.Entries[0].Schema)
}

func TestIngestObserveResults_AWSInstanceSchema(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	graph := inference.BuildInheritanceGraph(map[string]validator.SchemaMetadata{
		"pudl/aws.#Instance": {ResourceType: "aws.ec2.instance"},
	})
	input := `[{"target":"//aws","current":{"records":[{
		"_schema":"aws.ec2.instance",
		"instance_id":"i-123",
		"instance_type":"t3.micro",
		"state":"running",
		"tags":[],
		"security_groups":[]
	}]}}]`

	result, err := IngestObserve(db, ObserveIngest{
		Reader: strings.NewReader(input), Origin: "mu-aws", DataDir: dataDir, Graph: graph,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Records)

	entries, err := db.QueryEntries(
		database.FilterOptions{EntryTypes: []string{"observe"}, CollectionType: "item"},
		database.QueryOptions{Limit: 10},
	)
	require.NoError(t, err)
	require.Len(t, entries.Entries, 1)
	assert.Equal(t, "pudl/aws.#Instance", entries.Entries[0].Schema)
}

func TestIngestObserveResults_ExplicitPluginMappingOverridesNamingConvention(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()

	graph := inference.BuildInheritanceGraph(map[string]validator.SchemaMetadata{
		"pudl/aws.#VPC":           {ResourceType: "aws.ec2.vpc"},
		"pudl/custom.#AwsNetwork": {ResourceType: "aws.ec2.vpc"},
	})
	input := `[{"target":"//aws","current":{"records":[{"_schema":"aws.ec2.vpc","vpc_id":"vpc-123","cidr_block":"10.0.0.0/16","state":"available","is_default":false,"tags":[],"instance_tenancy":"default"}]}}]`
	result, err := IngestObserve(db, ObserveIngest{
		Reader: strings.NewReader(input), Origin: "mu-aws", DataDir: dataDir, Graph: graph,
		SchemaMappings: map[string]string{"aws.ec2.vpc": "pudl/custom.#AwsNetwork"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Records)

	entries, err := db.QueryEntries(database.FilterOptions{EntryTypes: []string{"observe"}, CollectionType: "item"}, database.QueryOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, entries.Entries, 1)
	assert.Equal(t, "pudl/custom.#AwsNetwork", entries.Entries[0].Schema)
}

func TestResolveObserveSchemaNormalizesExplicitPluginMapping(t *testing.T) {
	graph := inference.BuildInheritanceGraph(map[string]validator.SchemaMetadata{
		"pudl/custom.#AwsNetwork": {ResourceType: "aws.ec2.vpc"},
	})

	got := resolveObserveSchemaWithMappings(
		map[string]any{"_schema": "aws.ec2.vpc"},
		graph,
		nil,
		map[string]string{"aws.ec2.vpc": "pudl.schemas/pudl/custom@v0:#AwsNetwork"},
	)
	assert.Equal(t, "pudl/custom.#AwsNetwork", got)
}

func TestResolveObserveSchemaUsesInferenceForUnknownResourceType(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cue.mod"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cue.mod", "module.cue"), []byte("module: \"test.schemas\"\nlanguage: version: \"v0.14.0\"\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schemas"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas", "custom.cue"), []byte(`package schemas

#Thing: {
	_pudl: {
		schema_type: "base"
		resource_type: "provider.custom"
		identity_fields: ["name"]
	}
	name: string
	...
}
`), 0o644))

	inferrer, err := inference.NewSchemaInferrer(dir)
	require.NoError(t, err)
	got := resolveObserveSchema(map[string]any{
		"_schema": "provider.custom",
		"name":    "thing-1",
	}, inferrer.GetInheritanceGraph(), inferrer)
	assert.Equal(t, "schemas.#Thing", got)
}
