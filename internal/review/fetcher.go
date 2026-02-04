package review

import (
	"encoding/json"
	"fmt"
	"os"

	"pudl/internal/database"
	"pudl/internal/errors"
)

// ReviewItemFetcher handles fetching and preparing data items for review
type ReviewItemFetcher struct {
	catalogDB *database.CatalogDB
	dataPath  string
}

// NewReviewItemFetcher creates a new review item fetcher
func NewReviewItemFetcher(catalogDB *database.CatalogDB, dataPath string) *ReviewItemFetcher {
	return &ReviewItemFetcher{
		catalogDB: catalogDB,
		dataPath:  dataPath,
	}
}

// FetchItemsForReview fetches catalog entries and loads their data for review
func (f *ReviewItemFetcher) FetchItemsForReview(filter SessionFilter) ([]ReviewItem, error) {
	// Convert session filter to database filter
	dbFilter := database.FilterOptions{
		Schema: filter.Schema,
		Origin: filter.Origin,
		Format: filter.Format,
	}

	// Apply special filters
	if filter.OnlyUnknown {
		dbFilter.Schema = "core.#Item"
	}

	// Set up query options
	queryOpts := database.QueryOptions{
		Limit:   filter.MaxItems,
		Offset:  0,
		SortBy:  "import_timestamp",
		Reverse: true, // Most recent first
	}

	// Query the catalog
	queryResult, err := f.catalogDB.QueryEntries(dbFilter, queryOpts)
	if err != nil {
		return nil, errors.WrapError(
			errors.ErrCodeDatabaseError,
			"Failed to query catalog for review items",
			err,
		)
	}

	if len(queryResult.Entries) == 0 {
		return nil, errors.NewInputError(
			"No items found matching the specified criteria",
			"Try adjusting your filter criteria or import more data",
		)
	}

	// Convert catalog entries to review items
	var reviewItems []ReviewItem
	for _, entry := range queryResult.Entries {
		reviewItem, err := f.createReviewItem(entry)
		if err != nil {
			// Log error but continue with other items
			fmt.Printf("Warning: Failed to load data for item %s: %v\n", entry.ID, err)
			continue
		}
		reviewItems = append(reviewItems, *reviewItem)
	}

	if len(reviewItems) == 0 {
		return nil, errors.NewSystemError(
			"No items could be loaded for review",
			nil,
		)
	}

	return reviewItems, nil
}

// createReviewItem creates a ReviewItem from a catalog entry by loading the actual data
func (f *ReviewItemFetcher) createReviewItem(entry database.CatalogEntry) (*ReviewItem, error) {
	// Load the actual data from the stored file
	data, err := f.loadDataFromFile(entry.StoredPath)
	if err != nil {
		return nil, err
	}

	// Load metadata for additional context
	metadata, err := f.loadMetadata(entry.MetadataPath)
	if err != nil {
		// Metadata is optional, continue without it
		metadata = make(map[string]interface{})
	}

	// Create suggested schema based on patterns or confidence
	suggestedSchema := f.suggestBetterSchema(entry, data)

	reviewItem := &ReviewItem{
		EntryID:         entry.ID,
		Data:            data,
		CurrentSchema:   entry.Schema,
		SuggestedSchema: suggestedSchema,
		Status:          StatusPending,
		Metadata: map[string]interface{}{
			"catalog_entry":      entry,
			"import_timestamp":   entry.ImportTimestamp,
			"origin":            entry.Origin,
			"format":            entry.Format,
			"record_count":      entry.RecordCount,
			"size_bytes":        entry.SizeBytes,
			"confidence":        entry.Confidence,
			"collection_id":     entry.CollectionID,
			"item_index":        entry.ItemIndex,
			"collection_type":   entry.CollectionType,
			"stored_metadata":   metadata,
		},
	}

	return reviewItem, nil
}

// loadDataFromFile loads the actual data from a stored file
func (f *ReviewItemFetcher) loadDataFromFile(storedPath string) (interface{}, error) {
	// Read the file
	data, err := os.ReadFile(storedPath)
	if err != nil {
		return nil, errors.NewFileNotFoundError(storedPath)
	}

	// Parse as JSON (most common format)
	var jsonData interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		// If JSON parsing fails, return as string
		return string(data), nil
	}

	return jsonData, nil
}

// loadMetadata loads metadata from the metadata file
func (f *ReviewItemFetcher) loadMetadata(metadataPath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, err
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return metadata, nil
}

// suggestBetterSchema suggests a better schema based on data patterns
func (f *ReviewItemFetcher) suggestBetterSchema(entry database.CatalogEntry, data interface{}) string {
	// If confidence is low, suggest a review
	if entry.Confidence < 0.7 {
		return f.inferSchemaFromData(data, entry.Origin)
	}

	// If it's unknown schema, try to infer a better one
	if entry.Schema == "core.#Item" {
		return f.inferSchemaFromData(data, entry.Origin)
	}

	// Otherwise, no suggestion
	return ""
}

// inferSchemaFromData attempts to infer a better schema from data patterns
func (f *ReviewItemFetcher) inferSchemaFromData(data interface{}, origin string) string {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return ""
	}

	// AWS patterns
	if f.hasAWSFields(dataMap) {
		if f.hasEC2Fields(dataMap) {
			return "aws.#EC2Instance"
		}
		if f.hasS3Fields(dataMap) {
			return "aws.#S3Resource"
		}
		return "aws.#Resource"
	}

	// Kubernetes patterns
	if f.hasK8sFields(dataMap) {
		if kind, ok := dataMap["kind"].(string); ok {
			switch kind {
			case "Pod":
				return "k8s.#Pod"
			case "Service":
				return "k8s.#Service"
			case "Deployment":
				return "k8s.#Deployment"
			default:
				return "k8s.#Resource"
			}
		}
		return "k8s.#Resource"
	}

	// Origin-based inference
	if f.isAWSOrigin(origin) {
		return "aws.#Resource"
	}
	if f.isK8sOrigin(origin) {
		return "k8s.#Resource"
	}

	return ""
}

// hasAWSFields checks if data contains AWS-specific fields
func (f *ReviewItemFetcher) hasAWSFields(data map[string]interface{}) bool {
	awsFields := []string{"arn", "awsRegion", "resourceType", "accountId", "instanceId", "bucketName"}
	for _, field := range awsFields {
		if _, exists := data[field]; exists {
			return true
		}
	}
	return false
}

// hasEC2Fields checks for EC2-specific fields
func (f *ReviewItemFetcher) hasEC2Fields(data map[string]interface{}) bool {
	ec2Fields := []string{"instanceId", "instanceType", "state", "privateIpAddress", "publicIpAddress"}
	count := 0
	for _, field := range ec2Fields {
		if _, exists := data[field]; exists {
			count++
		}
	}
	return count >= 2 // Need at least 2 EC2-specific fields
}

// hasS3Fields checks for S3-specific fields
func (f *ReviewItemFetcher) hasS3Fields(data map[string]interface{}) bool {
	s3Fields := []string{"bucketName", "key", "size", "lastModified", "etag"}
	count := 0
	for _, field := range s3Fields {
		if _, exists := data[field]; exists {
			count++
		}
	}
	return count >= 2 // Need at least 2 S3-specific fields
}

// hasK8sFields checks if data contains Kubernetes-specific fields
func (f *ReviewItemFetcher) hasK8sFields(data map[string]interface{}) bool {
	k8sFields := []string{"apiVersion", "kind", "metadata"}
	for _, field := range k8sFields {
		if _, exists := data[field]; exists {
			return true
		}
	}
	return false
}

// isAWSOrigin checks if origin suggests AWS data
func (f *ReviewItemFetcher) isAWSOrigin(origin string) bool {
	awsOrigins := []string{"aws-", "ec2-", "s3-", "lambda-", "rds-", "iam-", "cloudformation-"}
	for _, prefix := range awsOrigins {
		if len(origin) >= len(prefix) && origin[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// isK8sOrigin checks if origin suggests Kubernetes data
func (f *ReviewItemFetcher) isK8sOrigin(origin string) bool {
	k8sOrigins := []string{"k8s-", "kubectl-", "kubernetes-"}
	for _, prefix := range k8sOrigins {
		if len(origin) >= len(prefix) && origin[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// GetReviewStats returns statistics about items available for review
func (f *ReviewItemFetcher) GetReviewStats(filter SessionFilter) (*ReviewStats, error) {
	// Get total count with filter
	dbFilter := database.FilterOptions{
		Schema: filter.Schema,
		Origin: filter.Origin,
		Format: filter.Format,
	}

	if filter.OnlyUnknown {
		dbFilter.Schema = "core.#Item"
	}

	queryOpts := database.QueryOptions{
		Limit:  0, // No limit for counting
		Offset: 0,
	}

	queryResult, err := f.catalogDB.QueryEntries(dbFilter, queryOpts)
	if err != nil {
		return nil, err
	}

	// Get breakdown by schema
	schemaBreakdown := make(map[string]int)
	for _, entry := range queryResult.Entries {
		schemaBreakdown[entry.Schema]++
	}

	stats := &ReviewStats{
		TotalItems:       queryResult.FilteredCount,
		UnknownItems:     schemaBreakdown["core.#Item"],
		SchemaBreakdown:  schemaBreakdown,
		FilterCriteria:   filter,
	}

	return stats, nil
}

// ReviewStats contains statistics about items available for review
type ReviewStats struct {
	TotalItems      int                `json:"total_items"`
	UnknownItems    int                `json:"unknown_items"`
	SchemaBreakdown map[string]int     `json:"schema_breakdown"`
	FilterCriteria  SessionFilter      `json:"filter_criteria"`
}
