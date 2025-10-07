package review

import (
	"fmt"
	"time"

	"pudl/internal/database"
	"pudl/internal/errors"
)

// CatalogUpdater handles updating catalog entries when schemas change during review
type CatalogUpdater struct {
	catalogDB *database.CatalogDB
}

// NewCatalogUpdater creates a new catalog updater
func NewCatalogUpdater(catalogDB *database.CatalogDB) *CatalogUpdater {
	return &CatalogUpdater{
		catalogDB: catalogDB,
	}
}

// ApplySessionChanges applies all schema changes from a review session to the catalog
func (cu *CatalogUpdater) ApplySessionChanges(session *ReviewSession) (*UpdateSummary, error) {
	summary := &UpdateSummary{
		SessionID:        session.SessionID,
		TotalChanges:     len(session.Changes),
		SuccessfulUpdates: 0,
		FailedUpdates:    0,
		Errors:          []string{},
		UpdatedEntries:   []string{},
	}

	for _, change := range session.Changes {
		if err := cu.applySchemaChange(change); err != nil {
			summary.FailedUpdates++
			summary.Errors = append(summary.Errors, 
				fmt.Sprintf("Failed to update %s: %v", change.EntryID, err))
		} else {
			summary.SuccessfulUpdates++
			summary.UpdatedEntries = append(summary.UpdatedEntries, change.EntryID)
		}
	}

	return summary, nil
}

// applySchemaChange applies a single schema change to the catalog
func (cu *CatalogUpdater) applySchemaChange(change SchemaChange) error {
	// Get the current catalog entry
	entry, err := cu.catalogDB.GetEntry(change.EntryID)
	if err != nil {
		return errors.WrapError(
			errors.ErrCodeNotFound,
			fmt.Sprintf("Catalog entry not found: %s", change.EntryID),
			err,
		)
	}

	// Update the schema field
	entry.Schema = change.NewSchema
	
	// Update confidence based on change type
	switch change.ChangeType {
	case "accept":
		// User accepted the schema, increase confidence
		entry.Confidence = 1.0
	case "reassign":
		// User manually reassigned, high confidence
		entry.Confidence = 0.95
	case "create":
		// User created new schema, very high confidence
		entry.Confidence = 1.0
	default:
		// Keep existing confidence
	}

	// Update the entry in the database
	if err := cu.catalogDB.UpdateEntry(*entry); err != nil {
		return errors.WrapError(
			errors.ErrCodeDatabaseError,
			"Failed to update catalog entry",
			err,
		)
	}

	return nil
}

// UpdateSingleEntry updates a single catalog entry with new schema information
func (cu *CatalogUpdater) UpdateSingleEntry(entryID, newSchema string, confidence float64) error {
	// Get the current catalog entry
	entry, err := cu.catalogDB.GetEntry(entryID)
	if err != nil {
		return err
	}

	// Update schema and confidence
	entry.Schema = newSchema
	entry.Confidence = confidence

	// Update the entry in the database
	return cu.catalogDB.UpdateEntry(*entry)
}

// GetEntryHistory returns the update history for a catalog entry
func (cu *CatalogUpdater) GetEntryHistory(entryID string) (*EntryHistory, error) {
	// Get current entry
	entry, err := cu.catalogDB.GetEntry(entryID)
	if err != nil {
		return nil, err
	}

	// For now, we don't store full history in the database
	// This could be enhanced to track schema change history
	history := &EntryHistory{
		EntryID:        entryID,
		CurrentSchema:  entry.Schema,
		CurrentConfidence: entry.Confidence,
		LastUpdated:    entry.UpdatedAt,
		Changes:        []HistoryEntry{
			{
				Timestamp: entry.UpdatedAt,
				Schema:    entry.Schema,
				Confidence: entry.Confidence,
				ChangeType: "current",
				Source:     "catalog",
			},
		},
	}

	return history, nil
}

// ValidateChanges validates that all changes in a session can be applied
func (cu *CatalogUpdater) ValidateChanges(session *ReviewSession) *ValidationSummary {
	summary := &ValidationSummary{
		SessionID:     session.SessionID,
		TotalChanges:  len(session.Changes),
		ValidChanges:  0,
		InvalidChanges: 0,
		Errors:       []string{},
	}

	for _, change := range session.Changes {
		if err := cu.validateSchemaChange(change); err != nil {
			summary.InvalidChanges++
			summary.Errors = append(summary.Errors, 
				fmt.Sprintf("Invalid change for %s: %v", change.EntryID, err))
		} else {
			summary.ValidChanges++
		}
	}

	return summary
}

// validateSchemaChange validates that a schema change can be applied
func (cu *CatalogUpdater) validateSchemaChange(change SchemaChange) error {
	// Check that the entry exists
	_, err := cu.catalogDB.GetEntry(change.EntryID)
	if err != nil {
		return errors.WrapError(
			errors.ErrCodeNotFound,
			fmt.Sprintf("Catalog entry not found: %s", change.EntryID),
			err,
		)
	}

	// Validate schema name format
	if change.NewSchema == "" {
		return errors.NewInputError("New schema cannot be empty")
	}

	// TODO: Add more validation:
	// - Check that the new schema exists
	// - Validate schema name format
	// - Check permissions/constraints

	return nil
}

// RollbackChanges rolls back schema changes for a specific session
func (cu *CatalogUpdater) RollbackChanges(session *ReviewSession) (*UpdateSummary, error) {
	summary := &UpdateSummary{
		SessionID:        session.SessionID,
		TotalChanges:     len(session.Changes),
		SuccessfulUpdates: 0,
		FailedUpdates:    0,
		Errors:          []string{},
		UpdatedEntries:   []string{},
	}

	// Reverse the changes by applying the old schema
	for _, change := range session.Changes {
		rollbackChange := SchemaChange{
			EntryID:    change.EntryID,
			OldSchema:  change.NewSchema, // Swap old and new
			NewSchema:  change.OldSchema,
			ChangeType: "rollback",
			Timestamp:  time.Now(),
		}

		if err := cu.applySchemaChange(rollbackChange); err != nil {
			summary.FailedUpdates++
			summary.Errors = append(summary.Errors, 
				fmt.Sprintf("Failed to rollback %s: %v", change.EntryID, err))
		} else {
			summary.SuccessfulUpdates++
			summary.UpdatedEntries = append(summary.UpdatedEntries, change.EntryID)
		}
	}

	return summary, nil
}

// GetUpdateStatistics returns statistics about catalog updates
func (cu *CatalogUpdater) GetUpdateStatistics() (*UpdateStatistics, error) {
	// Query for basic statistics
	// This is a simplified version - could be enhanced with more detailed queries
	
	queryResult, err := cu.catalogDB.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	if err != nil {
		return nil, err
	}

	stats := &UpdateStatistics{
		TotalEntries:    queryResult.TotalCount,
		LastUpdateTime:  time.Now(), // Placeholder
		SchemaBreakdown: make(map[string]int),
		ConfidenceStats: ConfidenceStatistics{},
	}

	// Calculate schema breakdown and confidence statistics
	var totalConfidence float64
	var highConfidenceCount int
	var lowConfidenceCount int

	for _, entry := range queryResult.Entries {
		stats.SchemaBreakdown[entry.Schema]++
		totalConfidence += entry.Confidence
		
		if entry.Confidence >= 0.8 {
			highConfidenceCount++
		} else if entry.Confidence < 0.5 {
			lowConfidenceCount++
		}
	}

	if len(queryResult.Entries) > 0 {
		stats.ConfidenceStats.AverageConfidence = totalConfidence / float64(len(queryResult.Entries))
		stats.ConfidenceStats.HighConfidenceCount = highConfidenceCount
		stats.ConfidenceStats.LowConfidenceCount = lowConfidenceCount
	}

	return stats, nil
}

// UpdateSummary contains the results of applying session changes
type UpdateSummary struct {
	SessionID         string    `json:"session_id"`
	TotalChanges      int       `json:"total_changes"`
	SuccessfulUpdates int       `json:"successful_updates"`
	FailedUpdates     int       `json:"failed_updates"`
	Errors           []string  `json:"errors"`
	UpdatedEntries   []string  `json:"updated_entries"`
}

// ValidationSummary contains the results of validating session changes
type ValidationSummary struct {
	SessionID      string   `json:"session_id"`
	TotalChanges   int      `json:"total_changes"`
	ValidChanges   int      `json:"valid_changes"`
	InvalidChanges int      `json:"invalid_changes"`
	Errors        []string `json:"errors"`
}

// EntryHistory contains the update history for a catalog entry
type EntryHistory struct {
	EntryID           string         `json:"entry_id"`
	CurrentSchema     string         `json:"current_schema"`
	CurrentConfidence float64        `json:"current_confidence"`
	LastUpdated       time.Time      `json:"last_updated"`
	Changes          []HistoryEntry `json:"changes"`
}

// HistoryEntry represents a single change in an entry's history
type HistoryEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	Schema     string    `json:"schema"`
	Confidence float64   `json:"confidence"`
	ChangeType string    `json:"change_type"`
	Source     string    `json:"source"`
}

// UpdateStatistics contains statistics about catalog updates
type UpdateStatistics struct {
	TotalEntries     int                    `json:"total_entries"`
	LastUpdateTime   time.Time              `json:"last_update_time"`
	SchemaBreakdown  map[string]int         `json:"schema_breakdown"`
	ConfidenceStats  ConfidenceStatistics   `json:"confidence_stats"`
}

// ConfidenceStatistics contains statistics about schema assignment confidence
type ConfidenceStatistics struct {
	AverageConfidence    float64 `json:"average_confidence"`
	HighConfidenceCount  int     `json:"high_confidence_count"`  // >= 0.8
	LowConfidenceCount   int     `json:"low_confidence_count"`   // < 0.5
}
