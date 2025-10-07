package idgen

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ImporterIDManager manages ID generation for the importer
type ImporterIDManager struct {
	generator *IDGenerator
	config    IDConfig
}

// NewImporterIDManager creates a new ID manager for the importer
func NewImporterIDManager(config IDConfig) *ImporterIDManager {
	return &ImporterIDManager{
		generator: NewIDGenerator(config.Format, config.Prefix),
		config:    config,
	}
}

// NewImporterIDManagerFromOrigin creates an ID manager with format based on detected origin
func NewImporterIDManagerFromOrigin(origin string) *ImporterIDManager {
	config := selectConfigForOrigin(origin)
	return NewImporterIDManager(config)
}

// GenerateMainID creates an ID for a main catalog entry
func (m *ImporterIDManager) GenerateMainID(sourcePath, origin string) string {
	switch m.config.Format {
	case FormatLegacy:
		// Maintain backward compatibility
		timestamp := time.Now()
		timestampStr := timestamp.Format("20060102_150405")
		ext := filepath.Ext(sourcePath)
		filename := fmt.Sprintf("%s_%s%s", timestampStr, origin, ext)
		return strings.TrimSuffix(filename, ext)
		
	case FormatReadable:
		// Use origin as context for more meaningful names
		return m.generator.Generate(origin)
		
	case FormatCompact:
		// Include origin hint in compact format
		return m.generator.Generate(origin)
		
	default:
		return m.generator.Generate()
	}
}

// GenerateCollectionID creates an ID for a collection
func (m *ImporterIDManager) GenerateCollectionID(sourcePath, origin string) string {
	// Collections use the same logic as main IDs
	return m.GenerateMainID(sourcePath, origin)
}

// GenerateItemID creates an ID for a collection item
func (m *ImporterIDManager) GenerateItemID(collectionID string, index int, itemData interface{}) string {
	// Convert itemData to map if possible
	var dataMap map[string]interface{}
	if data, ok := itemData.(map[string]interface{}); ok {
		dataMap = data
	} else {
		dataMap = make(map[string]interface{})
	}
	
	return m.generator.GenerateCollectionItemID(collectionID, index, dataMap)
}

// selectConfigForOrigin chooses appropriate ID format based on data origin
func selectConfigForOrigin(origin string) IDConfig {
	origin = strings.ToLower(origin)
	
	// AWS-related origins
	if strings.Contains(origin, "aws") || strings.Contains(origin, "ec2") || 
	   strings.Contains(origin, "s3") || strings.Contains(origin, "rds") {
		return DefaultConfigs["aws"]
	}
	
	// Kubernetes-related origins
	if strings.Contains(origin, "k8s") || strings.Contains(origin, "kubernetes") ||
	   strings.Contains(origin, "kubectl") || strings.Contains(origin, "pod") {
		return DefaultConfigs["kubernetes"]
	}
	
	// Collection-related
	if strings.Contains(origin, "collection") || strings.Contains(origin, "ndjson") {
		return DefaultConfigs["collections"]
	}
	
	// Default to general format
	return DefaultConfigs["general"]
}

// IDMigrationHelper helps with migrating from old to new ID formats
type IDMigrationHelper struct {
	legacyGenerator *IDGenerator
	newGenerator    *IDGenerator
}

// NewIDMigrationHelper creates a helper for ID migration
func NewIDMigrationHelper(newConfig IDConfig) *IDMigrationHelper {
	return &IDMigrationHelper{
		legacyGenerator: NewIDGenerator(FormatLegacy, ""),
		newGenerator:    NewIDGenerator(newConfig.Format, newConfig.Prefix),
	}
}

// IsLegacyID checks if an ID follows the old format
func (h *IDMigrationHelper) IsLegacyID(id string) bool {
	// Legacy IDs typically have timestamp format: YYYYMMDD_HHMMSS_origin
	if len(id) < 15 {
		return false
	}
	
	// Check for timestamp pattern at the beginning
	timestampPart := id[:15]
	if len(timestampPart) == 15 && timestampPart[8] == '_' {
		// Check if first 8 chars are numeric (YYYYMMDD)
		for i := 0; i < 8; i++ {
			if timestampPart[i] < '0' || timestampPart[i] > '9' {
				return false
			}
		}
		// Check if chars 9-14 are numeric (HHMMSS)
		for i := 9; i < 15; i++ {
			if timestampPart[i] < '0' || timestampPart[i] > '9' {
				return false
			}
		}
		return true
	}
	
	return false
}

// GenerateNewID creates a new-format ID to replace a legacy ID
func (h *IDMigrationHelper) GenerateNewID(legacyID string) string {
	// Extract context from legacy ID if possible
	if h.IsLegacyID(legacyID) && len(legacyID) > 16 {
		// Extract origin part after timestamp
		originPart := legacyID[16:] // Skip "YYYYMMDD_HHMMSS_"
		return h.newGenerator.Generate(originPart)
	}
	
	return h.newGenerator.Generate()
}

// IDDisplayHelper provides utilities for displaying IDs in user interfaces
type IDDisplayHelper struct{}

// NewIDDisplayHelper creates a new display helper
func NewIDDisplayHelper() *IDDisplayHelper {
	return &IDDisplayHelper{}
}

// FormatForDisplay formats an ID for user-friendly display
func (h *IDDisplayHelper) FormatForDisplay(id string) string {
	// If it's a legacy ID, show a shortened version
	if h.isLegacyFormat(id) {
		return h.shortenLegacyID(id)
	}
	
	// For new formats, return as-is since they're already human-friendly
	return id
}

// GetIDType returns a human-readable description of the ID format
func (h *IDDisplayHelper) GetIDType(id string) string {
	if h.isLegacyFormat(id) {
		return "legacy"
	}

	if h.isProquint(id) {
		return "proquint"
	}

	if h.isShortCode(id) {
		return "short"
	}

	if h.isReadable(id) {
		return "readable"
	}

	if h.isCompact(id) {
		return "compact"
	}

	if h.isSequential(id) {
		return "sequential"
	}

	return "unknown"
}

// isLegacyFormat checks if ID follows legacy timestamp format
func (h *IDDisplayHelper) isLegacyFormat(id string) bool {
	return len(id) > 15 && strings.Contains(id[:16], "_")
}

// shortenLegacyID creates a shorter display version of legacy IDs
func (h *IDDisplayHelper) shortenLegacyID(id string) string {
	if len(id) <= 20 {
		return id
	}
	
	// Show first 8 chars (date) + "..." + last 8 chars
	return fmt.Sprintf("%s...%s", id[:8], id[len(id)-8:])
}

// isShortCode checks if ID is a short alphanumeric code
func (h *IDDisplayHelper) isShortCode(id string) bool {
	// Remove prefix if present
	parts := strings.Split(id, "-")
	lastPart := parts[len(parts)-1]
	
	return len(lastPart) == 6 && isAlphanumeric(lastPart)
}

// isReadable checks if ID follows readable format (adjective-noun-number)
func (h *IDDisplayHelper) isReadable(id string) bool {
	parts := strings.Split(id, "-")
	if len(parts) < 3 {
		return false
	}
	
	// Check if last part is a 2-digit number
	lastPart := parts[len(parts)-1]
	return len(lastPart) == 2 && isNumeric(lastPart)
}

// isCompact checks if ID follows compact format (MMDDYY-XXX)
func (h *IDDisplayHelper) isCompact(id string) bool {
	parts := strings.Split(id, "-")
	if len(parts) < 2 {
		return false
	}

	// Look for date pattern (6 digits) followed by 3-char code
	for i := 0; i < len(parts)-1; i++ {
		if len(parts[i]) == 6 && isNumeric(parts[i]) &&
		   len(parts[i+1]) == 3 && isAlphanumeric(parts[i+1]) {
			return true
		}
	}

	return false
}

// isSequential checks if ID follows sequential format
func (h *IDDisplayHelper) isSequential(id string) bool {
	parts := strings.Split(id, "-")
	lastPart := parts[len(parts)-1]

	// Check if last part is a 3-digit number
	return len(lastPart) == 3 && isNumeric(lastPart)
}

// isProquint checks if ID follows proquint format
func (h *IDDisplayHelper) isProquint(id string) bool {
	// Remove prefix if present
	parts := strings.Split(id, "-")

	// Proquint should have exactly 2 quintuplets (after removing prefix)
	if len(parts) < 2 {
		return false
	}

	// Check if last two parts look like proquint quintuplets
	quintuplets := parts[len(parts)-2:]
	if len(quintuplets) != 2 {
		return false
	}

	for _, q := range quintuplets {
		if !isValidQuintuplet(q) {
			return false
		}
	}

	return true
}

// isValidQuintuplet checks if a string is a valid proquint quintuplet
func isValidQuintuplet(s string) bool {
	if len(s) != 5 {
		return false
	}

	consonants := "bdfghjklmnpqrstvz"
	vowels := "aiou"

	// Pattern: consonant-vowel-consonant-vowel-consonant
	return strings.Contains(consonants, string(s[0])) &&
		   strings.Contains(vowels, string(s[1])) &&
		   strings.Contains(consonants, string(s[2])) &&
		   strings.Contains(vowels, string(s[3])) &&
		   strings.Contains(consonants, string(s[4]))
}

// Helper functions for character validation
func isAlphanumeric(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return len(s) > 0
}

func isNumeric(s string) bool {
	for _, r := range s {
		if !(r >= '0' && r <= '9') {
			return false
		}
	}
	return len(s) > 0
}
