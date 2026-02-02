package database

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// BenchmarkAddEntry benchmarks adding entries to the database
func BenchmarkAddEntry(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "bench-db-")
	defer os.RemoveAll(tmpDir)

	db, _ := NewCatalogDB(tmpDir)
	defer db.Close()

	entry := CatalogEntry{
		ID:              "test-001",
		StoredPath:      "/path/to/stored.json",
		MetadataPath:    "/path/to/metadata.json",
		ImportTimestamp: time.Now(),
		Format:          "json",
		Origin:          "test-origin",
		Schema:          "test.schema",
		Confidence:      0.95,
		RecordCount:     100,
		SizeBytes:       1024,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.ID = fmt.Sprintf("test-%06d", i)
		db.AddEntry(entry)
	}
}

// BenchmarkQueryEntries benchmarks querying entries from the database
func BenchmarkQueryEntries(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "bench-db-")
	defer os.RemoveAll(tmpDir)

	db, _ := NewCatalogDB(tmpDir)
	defer db.Close()

	// Add test data
	for i := 0; i < 1000; i++ {
		entry := CatalogEntry{
			ID:              fmt.Sprintf("test-%06d", i),
			StoredPath:      "/path/to/stored.json",
			MetadataPath:    "/path/to/metadata.json",
			ImportTimestamp: time.Now(),
			Format:          "json",
			Origin:          "test-origin",
			Schema:          "test.schema",
			Confidence:      0.95,
			RecordCount:     100,
			SizeBytes:       1024,
		}
		db.AddEntry(entry)
	}

	filters := FilterOptions{}
	opts := QueryOptions{Limit: 100, Offset: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.QueryEntries(filters, opts)
	}
}

// BenchmarkGetEntry benchmarks retrieving a single entry
func BenchmarkGetEntry(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "bench-db-")
	defer os.RemoveAll(tmpDir)

	db, _ := NewCatalogDB(tmpDir)
	defer db.Close()

	entry := CatalogEntry{
		ID:              "test-001",
		StoredPath:      "/path/to/stored.json",
		MetadataPath:    "/path/to/metadata.json",
		ImportTimestamp: time.Now(),
		Format:          "json",
		Origin:          "test-origin",
		Schema:          "test.schema",
		Confidence:      0.95,
		RecordCount:     100,
		SizeBytes:       1024,
	}
	db.AddEntry(entry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.GetEntry("test-001")
	}
}

// BenchmarkUpdateEntry benchmarks updating an entry
func BenchmarkUpdateEntry(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "bench-db-")
	defer os.RemoveAll(tmpDir)

	db, _ := NewCatalogDB(tmpDir)
	defer db.Close()

	entry := CatalogEntry{
		ID:              "test-001",
		StoredPath:      "/path/to/stored.json",
		MetadataPath:    "/path/to/metadata.json",
		ImportTimestamp: time.Now(),
		Format:          "json",
		Origin:          "test-origin",
		Schema:          "test.schema",
		Confidence:      0.95,
		RecordCount:     100,
		SizeBytes:       1024,
	}
	db.AddEntry(entry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.Confidence = 0.90 + float64(i%10)*0.01
		db.UpdateEntry(entry)
	}
}

// BenchmarkBatchOperations benchmarks batch add operations
func BenchmarkBatchOperations(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "bench-db-")
	defer os.RemoveAll(tmpDir)

	db, _ := NewCatalogDB(tmpDir)
	defer db.Close()

	entries := make([]CatalogEntry, 100)
	for i := 0; i < 100; i++ {
		entries[i] = CatalogEntry{
			ID:              fmt.Sprintf("test-%06d", i),
			StoredPath:      "/path/to/stored.json",
			MetadataPath:    "/path/to/metadata.json",
			ImportTimestamp: time.Now(),
			Format:          "json",
			Origin:          "test-origin",
			Schema:          "test.schema",
			Confidence:      0.95,
			RecordCount:     100,
			SizeBytes:       1024,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, entry := range entries {
			db.AddEntry(entry)
		}
	}
}

