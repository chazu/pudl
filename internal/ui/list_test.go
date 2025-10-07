package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"pudl/internal/lister"
)

func TestDetailViewToggle(t *testing.T) {
	// Create a test entry
	entries := []lister.ListEntry{
		{
			ID:              "test-id",
			StoredPath:      "/tmp/test.json",
			MetadataPath:    "/tmp/test.meta.json",
			ImportTimestamp: "2025-10-07T10:00:00Z",
			Format:          "json",
			Origin:          "test",
			Schema:          "test-schema",
			Confidence:      0.95,
			RecordCount:     10,
			SizeBytes:       1024,
		},
	}

	// Create model
	model := NewModel(entries, false)

	// Initially should not be showing detail
	if model.showingDetail {
		t.Error("Model should not be showing detail initially")
	}

	// Simulate pressing enter
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	model = updatedModel.(Model)

	// Should now be showing detail
	if !model.showingDetail {
		t.Error("Model should be showing detail after enter key")
	}

	// Detail content should be populated
	if model.detailContent == "" {
		t.Error("Detail content should be populated")
	}

	// View should return detail content
	view := model.View()
	if view != model.detailContent {
		t.Error("View should return detail content when showing detail")
	}

	// Simulate pressing any other key to return to list
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	model = updatedModel.(Model)

	// Should no longer be showing detail
	if model.showingDetail {
		t.Error("Model should not be showing detail after pressing another key")
	}

	// Detail content should be cleared
	if model.detailContent != "" {
		t.Error("Detail content should be cleared")
	}
}

func TestQuitFromDetailView(t *testing.T) {
	// Create a test entry
	entries := []lister.ListEntry{
		{
			ID:              "test-id",
			StoredPath:      "/tmp/test.json",
			MetadataPath:    "/tmp/test.meta.json",
			ImportTimestamp: "2025-10-07T10:00:00Z",
			Format:          "json",
			Origin:          "test",
			Schema:          "test-schema",
			Confidence:      0.95,
			RecordCount:     10,
			SizeBytes:       1024,
		},
	}

	// Create model and enter detail view
	model := NewModel(entries, false)
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	model = updatedModel.(Model)

	// Should be showing detail
	if !model.showingDetail {
		t.Error("Model should be showing detail")
	}

	// Simulate pressing 'q' to quit
	updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	model = updatedModel.(Model)

	// Should be quitting
	if !model.quitting {
		t.Error("Model should be quitting after pressing 'q' in detail view")
	}

	// Command should be tea.Quit
	if cmd == nil {
		t.Error("Command should not be nil")
	}
}
