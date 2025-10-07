package testutil

import (
	"fmt"
	"time"

	"pudl/internal/database"
)

// TestDataGenerator provides methods to generate realistic test data
type TestDataGenerator struct {
	baseTime time.Time
	counter  int
}

// NewTestDataGenerator creates a new test data generator
func NewTestDataGenerator() *TestDataGenerator {
	return &TestDataGenerator{
		baseTime: time.Now().Add(-24 * time.Hour), // Start 24 hours ago
		counter:  0,
	}
}

// GenerateAWSEntries creates realistic AWS catalog entries
func (g *TestDataGenerator) GenerateAWSEntries(count int) []database.CatalogEntry {
	entries := make([]database.CatalogEntry, count)
	
	awsSchemas := []string{
		"aws.#EC2Instance",
		"aws.#S3Bucket",
		"aws.#RDSInstance",
		"aws.#SecurityGroup",
		"aws.#VPC",
		"aws.#ELBLoadBalancer",
		"aws.#LambdaFunction",
		"aws.#IAMRole",
	}
	
	awsOrigins := []string{
		"aws-ec2-describe-instances",
		"aws-s3-list-buckets",
		"aws-rds-describe-db-instances",
		"aws-ec2-describe-security-groups",
		"aws-ec2-describe-vpcs",
		"aws-elb-describe-load-balancers",
		"aws-lambda-list-functions",
		"aws-iam-list-roles",
	}
	
	for i := 0; i < count; i++ {
		schemaIndex := i % len(awsSchemas)
		g.counter++
		
		entries[i] = database.CatalogEntry{
			ID:              fmt.Sprintf("aws-test-%06d", g.counter),
			StoredPath:      fmt.Sprintf("/test/raw/aws-test-%06d.json", g.counter),
			MetadataPath:    fmt.Sprintf("/test/metadata/aws-test-%06d.meta", g.counter),
			ImportTimestamp: g.baseTime.Add(time.Duration(i) * time.Minute),
			Format:          "json",
			Origin:          awsOrigins[schemaIndex],
			Schema:          awsSchemas[schemaIndex],
			Confidence:      0.85 + float64(i%15)/100.0, // 0.85-0.99
			RecordCount:     1 + i%5,                     // 1-5 records
			SizeBytes:       int64(500 + i*50),           // Varying sizes
			CollectionID:    nil,
			ItemIndex:       nil,
			CollectionType:  nil,
			ItemID:          nil,
		}
	}
	
	return entries
}

// GenerateK8sEntries creates realistic Kubernetes catalog entries
func (g *TestDataGenerator) GenerateK8sEntries(count int) []database.CatalogEntry {
	entries := make([]database.CatalogEntry, count)
	
	k8sSchemas := []string{
		"k8s.#Pod",
		"k8s.#Service",
		"k8s.#Deployment",
		"k8s.#ConfigMap",
		"k8s.#Secret",
		"k8s.#Ingress",
		"k8s.#PersistentVolume",
		"k8s.#Namespace",
	}
	
	k8sOrigins := []string{
		"k8s-get-pods",
		"k8s-get-services",
		"k8s-get-deployments",
		"k8s-get-configmaps",
		"k8s-get-secrets",
		"k8s-get-ingresses",
		"k8s-get-persistentvolumes",
		"k8s-get-namespaces",
	}
	
	namespaces := []string{"default", "kube-system", "production", "staging", "monitoring"}
	
	for i := 0; i < count; i++ {
		schemaIndex := i % len(k8sSchemas)
		namespace := namespaces[i%len(namespaces)]
		g.counter++
		
		entries[i] = database.CatalogEntry{
			ID:              fmt.Sprintf("k8s-%s-%06d", namespace, g.counter),
			StoredPath:      fmt.Sprintf("/test/raw/k8s-%s-%06d.yaml", namespace, g.counter),
			MetadataPath:    fmt.Sprintf("/test/metadata/k8s-%s-%06d.meta", namespace, g.counter),
			ImportTimestamp: g.baseTime.Add(time.Duration(i) * time.Minute * 2),
			Format:          "yaml",
			Origin:          k8sOrigins[schemaIndex],
			Schema:          k8sSchemas[schemaIndex],
			Confidence:      0.90 + float64(i%10)/100.0, // 0.90-0.99
			RecordCount:     1,                           // K8s resources are typically single objects
			SizeBytes:       int64(200 + i*25),           // Smaller than AWS resources
			CollectionID:    nil,
			ItemIndex:       nil,
			CollectionType:  nil,
			ItemID:          nil,
		}
	}
	
	return entries
}

// GenerateCollectionWithItems creates a collection entry with associated items
func (g *TestDataGenerator) GenerateCollectionWithItems(itemCount int) (database.CatalogEntry, []database.CatalogEntry) {
	g.counter++
	collectionID := fmt.Sprintf("collection-%06d", g.counter)
	
	// Create collection entry
	collectionType := "collection"
	collection := database.CatalogEntry{
		ID:              collectionID,
		StoredPath:      fmt.Sprintf("/test/raw/%s.json", collectionID),
		MetadataPath:    fmt.Sprintf("/test/metadata/%s.meta", collectionID),
		ImportTimestamp: g.baseTime.Add(time.Duration(g.counter) * time.Hour),
		Format:          "ndjson",
		Origin:          "test-collection",
		Schema:          "collections.#Collection",
		Confidence:      0.95,
		RecordCount:     itemCount,
		SizeBytes:       int64(itemCount * 100), // Estimate based on item count
		CollectionID:    nil,                    // Collections don't have parent collections
		ItemIndex:       nil,                    // Collections don't have item index
		CollectionType:  &collectionType,
		ItemID:          nil, // Collections don't have item IDs
	}
	
	// Create item entries
	items := make([]database.CatalogEntry, itemCount)
	itemType := "item"
	
	for i := 0; i < itemCount; i++ {
		itemID := fmt.Sprintf("%s-item-%03d", collectionID, i)
		itemIndex := i
		
		items[i] = database.CatalogEntry{
			ID:              itemID,
			StoredPath:      fmt.Sprintf("/test/raw/%s.json", itemID),
			MetadataPath:    fmt.Sprintf("/test/metadata/%s.meta", itemID),
			ImportTimestamp: collection.ImportTimestamp.Add(time.Duration(i) * time.Second),
			Format:          "json",
			Origin:          fmt.Sprintf("%s-item-%d", collectionID, i),
			Schema:          "collections.#CollectionItem",
			Confidence:      0.8,
			RecordCount:     1,
			SizeBytes:       100,
			CollectionID:    &collectionID,
			ItemIndex:       &itemIndex,
			CollectionType:  &itemType,
			ItemID:          &itemID,
		}
	}
	
	return collection, items
}

// GenerateMixedDataset creates a diverse dataset with different types of entries
func (g *TestDataGenerator) GenerateMixedDataset(totalCount int) []database.CatalogEntry {
	// Distribute entries across different types
	awsCount := totalCount * 40 / 100      // 40% AWS
	k8sCount := totalCount * 30 / 100      // 30% Kubernetes
	genericCount := totalCount - awsCount - k8sCount // Remaining generic
	
	var allEntries []database.CatalogEntry
	
	// Add AWS entries
	awsEntries := g.GenerateAWSEntries(awsCount)
	allEntries = append(allEntries, awsEntries...)
	
	// Add Kubernetes entries
	k8sEntries := g.GenerateK8sEntries(k8sCount)
	allEntries = append(allEntries, k8sEntries...)
	
	// Add generic entries
	genericEntries := g.GenerateGenericEntries(genericCount)
	allEntries = append(allEntries, genericEntries...)
	
	return allEntries
}

// GenerateGenericEntries creates generic catalog entries for testing
func (g *TestDataGenerator) GenerateGenericEntries(count int) []database.CatalogEntry {
	entries := make([]database.CatalogEntry, count)
	
	genericSchemas := []string{
		"unknown.#CatchAll",
		"generic.#JSONData",
		"generic.#CSVData",
		"generic.#TextData",
	}
	
	genericOrigins := []string{
		"unknown",
		"manual-import",
		"csv-import",
		"text-import",
	}
	
	formats := []string{"json", "yaml", "csv", "txt"}
	
	for i := 0; i < count; i++ {
		schemaIndex := i % len(genericSchemas)
		formatIndex := i % len(formats)
		g.counter++
		
		entries[i] = database.CatalogEntry{
			ID:              fmt.Sprintf("generic-%06d", g.counter),
			StoredPath:      fmt.Sprintf("/test/raw/generic-%06d.%s", g.counter, formats[formatIndex]),
			MetadataPath:    fmt.Sprintf("/test/metadata/generic-%06d.meta", g.counter),
			ImportTimestamp: g.baseTime.Add(time.Duration(i) * time.Minute * 3),
			Format:          formats[formatIndex],
			Origin:          genericOrigins[schemaIndex],
			Schema:          genericSchemas[schemaIndex],
			Confidence:      0.5 + float64(i%30)/100.0, // 0.5-0.79
			RecordCount:     1 + i%20,                   // 1-20 records
			SizeBytes:       int64(50 + i*15),           // Smaller files
			CollectionID:    nil,
			ItemIndex:       nil,
			CollectionType:  nil,
			ItemID:          nil,
		}
	}
	
	return entries
}

// GenerateTimeSeriesEntries creates entries with specific time patterns for testing date queries
func (g *TestDataGenerator) GenerateTimeSeriesEntries(count int, interval time.Duration) []database.CatalogEntry {
	entries := make([]database.CatalogEntry, count)
	startTime := time.Now().Add(-time.Duration(count) * interval)
	
	for i := 0; i < count; i++ {
		g.counter++
		
		entries[i] = database.CatalogEntry{
			ID:              fmt.Sprintf("timeseries-%06d", g.counter),
			StoredPath:      fmt.Sprintf("/test/raw/timeseries-%06d.json", g.counter),
			MetadataPath:    fmt.Sprintf("/test/metadata/timeseries-%06d.meta", g.counter),
			ImportTimestamp: startTime.Add(time.Duration(i) * interval),
			Format:          "json",
			Origin:          "timeseries-test",
			Schema:          "test.#TimeSeriesData",
			Confidence:      0.9,
			RecordCount:     1,
			SizeBytes:       int64(100 + i),
			CollectionID:    nil,
			ItemIndex:       nil,
			CollectionType:  nil,
			ItemID:          nil,
		}
	}
	
	return entries
}

// GenerateEntriesWithDuplicates creates entries with some duplicate characteristics for testing
func (g *TestDataGenerator) GenerateEntriesWithDuplicates(count int) []database.CatalogEntry {
	entries := make([]database.CatalogEntry, count)
	
	// Create some duplicate patterns
	duplicateSchemas := []string{"aws.#EC2Instance", "k8s.#Pod"}
	duplicateOrigins := []string{"aws-ec2-describe-instances", "k8s-get-pods"}
	
	for i := 0; i < count; i++ {
		g.counter++
		
		// Every 3rd entry uses duplicate schema/origin
		var schema, origin string
		if i%3 == 0 {
			schema = duplicateSchemas[i%len(duplicateSchemas)]
			origin = duplicateOrigins[i%len(duplicateOrigins)]
		} else {
			schema = fmt.Sprintf("unique.#Schema%d", i)
			origin = fmt.Sprintf("unique-origin-%d", i)
		}
		
		entries[i] = database.CatalogEntry{
			ID:              fmt.Sprintf("duplicate-test-%06d", g.counter),
			StoredPath:      fmt.Sprintf("/test/raw/duplicate-test-%06d.json", g.counter),
			MetadataPath:    fmt.Sprintf("/test/metadata/duplicate-test-%06d.meta", g.counter),
			ImportTimestamp: g.baseTime.Add(time.Duration(i) * time.Minute),
			Format:          "json",
			Origin:          origin,
			Schema:          schema,
			Confidence:      0.8,
			RecordCount:     1,
			SizeBytes:       int64(200 + i*10),
			CollectionID:    nil,
			ItemIndex:       nil,
			CollectionType:  nil,
			ItemID:          nil,
		}
	}
	
	return entries
}

// GenerateCorruptedEntries creates entries with various data issues for error testing
func (g *TestDataGenerator) GenerateCorruptedEntries(count int) []database.CatalogEntry {
	entries := make([]database.CatalogEntry, count)
	
	for i := 0; i < count; i++ {
		g.counter++
		
		entry := database.CatalogEntry{
			ID:              fmt.Sprintf("corrupted-%06d", g.counter),
			StoredPath:      fmt.Sprintf("/test/raw/corrupted-%06d.json", g.counter),
			MetadataPath:    fmt.Sprintf("/test/metadata/corrupted-%06d.meta", g.counter),
			ImportTimestamp: g.baseTime.Add(time.Duration(i) * time.Minute),
			Format:          "json",
			Origin:          "corrupted-test",
			Schema:          "test.#CorruptedData",
			Confidence:      0.1, // Very low confidence
			RecordCount:     1,
			SizeBytes:       int64(50 + i*5),
			CollectionID:    nil,
			ItemIndex:       nil,
			CollectionType:  nil,
			ItemID:          nil,
		}
		
		// Introduce various corruption patterns
		switch i % 4 {
		case 0:
			// Empty ID (should cause validation error)
			entry.ID = ""
		case 1:
			// Invalid confidence (outside 0-1 range)
			entry.Confidence = 1.5
		case 2:
			// Negative record count
			entry.RecordCount = -1
		case 3:
			// Zero timestamp
			entry.ImportTimestamp = time.Time{}
		}
		
		entries[i] = entry
	}
	
	return entries
}
