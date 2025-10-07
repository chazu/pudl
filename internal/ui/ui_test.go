package ui

import (
	"testing"
	"time"

	"pudl/internal/lister"
)

func TestListItem(t *testing.T) {
	// Create a test entry
	collectionType := "collection"
	entry := lister.ListEntry{
		ID:              "test-id-123",
		Schema:          "aws.#EC2Instance",
		Origin:          "aws-ec2",
		Format:          "json",
		RecordCount:     100,
		SizeBytes:       1024,
		CollectionType:  &collectionType,
		ImportTimestamp: "2025-01-01T00:00:00Z",
		ParsedTimestamp: time.Now(),
	}

	item := ListItem{
		Entry: entry,
		Index: 1,
	}

	// Test FilterValue
	filterValue := item.FilterValue()
	expectedSubstrings := []string{"test-id-123", "aws.#EC2Instance", "aws-ec2", "json"}
	for _, substr := range expectedSubstrings {
		if !contains(filterValue, substr) {
			t.Errorf("FilterValue() should contain %q, got %q", substr, filterValue)
		}
	}

	// Test Title
	title := item.Title()
	if !contains(title, "test-id-123") {
		t.Errorf("Title() should contain ID, got %q", title)
	}
	if !contains(title, "aws.#EC2Instance") {
		t.Errorf("Title() should contain schema, got %q", title)
	}
	if !contains(title, "📦") {
		t.Errorf("Title() should contain collection indicator, got %q", title)
	}

	// Test Description
	description := item.Description()
	if !contains(description, "aws-ec2") {
		t.Errorf("Description() should contain origin, got %q", description)
	}
	if !contains(description, "json") {
		t.Errorf("Description() should contain format, got %q", description)
	}
	if !contains(description, "100") {
		t.Errorf("Description() should contain record count, got %q", description)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, test := range tests {
		result := formatBytes(test.input)
		if result != test.expected {
			t.Errorf("formatBytes(%d) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestFormatInt(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{123, "123"},
		{1000, "1,000"},
		{1234567, "1,234,567"},
		{-1000, "-1,000"},
	}

	for _, test := range tests {
		result := formatInt(test.input)
		if result != test.expected {
			t.Errorf("formatInt(%d) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestNewModel(t *testing.T) {
	// Create test entries
	entries := []lister.ListEntry{
		{
			ID:              "test-1",
			Schema:          "aws.#EC2Instance",
			Origin:          "aws-ec2",
			Format:          "json",
			RecordCount:     50,
			SizeBytes:       2048,
			ImportTimestamp: "2025-01-01T00:00:00Z",
			ParsedTimestamp: time.Now(),
		},
		{
			ID:              "test-2",
			Schema:          "k8s.#Pod",
			Origin:          "k8s-pods",
			Format:          "yaml",
			RecordCount:     25,
			SizeBytes:       1024,
			ImportTimestamp: "2025-01-01T01:00:00Z",
			ParsedTimestamp: time.Now(),
		},
	}

	// Create model
	model := NewModel(entries, false)

	// Verify model was created correctly
	if len(model.entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(model.entries))
	}

	if model.verbose != false {
		t.Errorf("Expected verbose=false, got %t", model.verbose)
	}

	// Verify list was initialized
	if model.list.FilteringEnabled() != true {
		t.Error("Expected filtering to be enabled")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) != -1
}

// Simple substring search
func findSubstring(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	if len(substr) > len(s) {
		return -1
	}
	
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
