package idgen

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// IDFormat represents different ID generation formats
type IDFormat string

const (
	// Short alphanumeric codes (e.g., "abc123", "xyz789")
	FormatShortCode IDFormat = "short_code"

	// Readable words + numbers (e.g., "blue-cat-42", "fast-tree-91")
	FormatReadable IDFormat = "readable"

	// Compact timestamp + short suffix (e.g., "1207-a1b", "1208-x9z")
	FormatCompact IDFormat = "compact"

	// Sequential with prefix (e.g., "data-001", "aws-042")
	FormatSequential IDFormat = "sequential"

	// Proquints - pronounceable quintuplets (e.g., "lusab-babad", "gutih-tugad")
	FormatProquint IDFormat = "proquint"

	// Legacy format (current long format for compatibility)
	FormatLegacy IDFormat = "legacy"
)

// IDGenerator provides configurable ID generation
type IDGenerator struct {
	format     IDFormat
	prefix     string
	counter    int64
	useCounter bool
}

// NewIDGenerator creates a new ID generator with the specified format
func NewIDGenerator(format IDFormat, prefix string) *IDGenerator {
	return &IDGenerator{
		format:     format,
		prefix:     prefix,
		counter:    1,
		useCounter: format == FormatSequential,
	}
}

// Generate creates a new ID based on the configured format
func (g *IDGenerator) Generate(context ...string) string {
	switch g.format {
	case FormatShortCode:
		return g.generateShortCode()
	case FormatReadable:
		return g.generateReadable()
	case FormatCompact:
		return g.generateCompact(context...)
	case FormatSequential:
		return g.generateSequential()
	case FormatProquint:
		return g.generateProquint()
	case FormatLegacy:
		return g.generateLegacy(context...)
	default:
		return g.generateShortCode() // Default fallback
	}
}

// generateShortCode creates short alphanumeric IDs (6-8 characters)
func (g *IDGenerator) generateShortCode() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	const length = 6
	
	result := make([]byte, length)
	for i := range result {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[num.Int64()]
	}
	
	if g.prefix != "" {
		return fmt.Sprintf("%s-%s", g.prefix, string(result))
	}
	return string(result)
}

// generateReadable creates human-readable IDs with adjective-noun-number format
func (g *IDGenerator) generateReadable() string {
	adjectives := []string{
		"blue", "red", "green", "fast", "slow", "big", "small", "bright", "dark", "cool",
		"warm", "fresh", "old", "new", "clean", "sharp", "soft", "hard", "light", "heavy",
	}
	
	nouns := []string{
		"cat", "dog", "tree", "rock", "star", "moon", "sun", "wave", "fire", "wind",
		"bird", "fish", "bear", "lion", "wolf", "fox", "deer", "owl", "hawk", "dove",
	}
	
	adjIdx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(adjectives))))
	nounIdx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(nouns))))
	num, _ := rand.Int(rand.Reader, big.NewInt(99))
	
	adj := adjectives[adjIdx.Int64()]
	noun := nouns[nounIdx.Int64()]
	number := num.Int64() + 1
	
	if g.prefix != "" {
		return fmt.Sprintf("%s-%s-%s-%02d", g.prefix, adj, noun, number)
	}
	return fmt.Sprintf("%s-%s-%02d", adj, noun, number)
}

// generateCompact creates compact timestamp-based IDs
func (g *IDGenerator) generateCompact(context ...string) string {
	now := time.Now()

	// Use MMDDYY format for date (6 chars)
	dateStr := now.Format("010206")

	// Generate 3-character suffix
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	suffix := make([]byte, 3)
	for i := range suffix {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		suffix[i] = charset[num.Int64()]
	}

	base := fmt.Sprintf("%s-%s", dateStr, string(suffix))

	if g.prefix != "" {
		return fmt.Sprintf("%s-%s", g.prefix, base)
	}
	return base
}

// generateSequential creates sequential IDs with zero-padding
func (g *IDGenerator) generateSequential() string {
	id := g.counter
	g.counter++

	if g.prefix != "" {
		return fmt.Sprintf("%s-%03d", g.prefix, id)
	}
	return fmt.Sprintf("%03d", id)
}

// generateProquint creates pronounceable quintuplet IDs
func (g *IDGenerator) generateProquint() string {
	// Generate a random 32-bit number
	num, _ := rand.Int(rand.Reader, big.NewInt(0xFFFFFFFF))
	proquint := numberToProquint(uint32(num.Int64()))

	if g.prefix != "" {
		return fmt.Sprintf("%s-%s", g.prefix, proquint)
	}
	return proquint
}

// generateLegacy creates IDs in the current long format for compatibility
func (g *IDGenerator) generateLegacy(context ...string) string {
	timestamp := time.Now()
	timestampStr := timestamp.Format("20060102_150405")
	
	if len(context) > 0 && context[0] != "" {
		return fmt.Sprintf("%s_%s", timestampStr, context[0])
	}
	
	if g.prefix != "" {
		return fmt.Sprintf("%s_%s", timestampStr, g.prefix)
	}
	return timestampStr
}

// GenerateCollectionItemID creates IDs for collection items
func (g *IDGenerator) GenerateCollectionItemID(collectionID string, index int, itemData map[string]interface{}) string {
	switch g.format {
	case FormatShortCode:
		// Try to extract meaningful ID from data
		if id := extractDataID(itemData); id != "" {
			return fmt.Sprintf("%s-%s", collectionID, sanitizeID(id, 8))
		}
		return fmt.Sprintf("%s-%s", collectionID, g.generateShortCode())

	case FormatReadable:
		return fmt.Sprintf("%s-item-%02d", collectionID, index)

	case FormatCompact:
		return fmt.Sprintf("%s-%02d", collectionID, index)

	case FormatSequential:
		return fmt.Sprintf("%s-%03d", collectionID, index)

	case FormatProquint:
		// Try to extract meaningful ID from data
		if id := extractDataID(itemData); id != "" {
			return fmt.Sprintf("%s-%s", collectionID, sanitizeID(id, 10))
		}
		// Generate proquint based on index for consistency
		proquint := numberToProquint(uint32(index + 1000)) // Add offset to avoid very short proquints
		return fmt.Sprintf("%s-%s", collectionID, proquint)

	case FormatLegacy:
		// Use existing logic for backward compatibility
		if id := extractDataID(itemData); id != "" {
			return fmt.Sprintf("%s_%s", collectionID, id)
		}
		return fmt.Sprintf("%s_item_%d", collectionID, index)

	default:
		return fmt.Sprintf("%s-%02d", collectionID, index)
	}
}

// extractDataID tries to extract a meaningful ID from item data
func extractDataID(itemData map[string]interface{}) string {
	// Try common ID fields
	idFields := []string{"id", "ID", "identifier", "name", "externalId", "external_id"}
	
	for _, field := range idFields {
		if value, exists := itemData[field]; exists {
			if str, ok := value.(string); ok && str != "" {
				return str
			}
		}
	}
	
	return ""
}

// sanitizeID cleans and truncates an ID to make it suitable for use
func sanitizeID(id string, maxLength int) string {
	// Replace problematic characters
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, "_", "-")
	id = strings.ToLower(id)
	
	// Remove non-alphanumeric characters except hyphens
	var result strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	
	cleaned := result.String()
	
	// Truncate if too long
	if len(cleaned) > maxLength {
		cleaned = cleaned[:maxLength]
	}
	
	// Remove trailing hyphens
	cleaned = strings.TrimRight(cleaned, "-")
	
	return cleaned
}

// IDConfig holds configuration for ID generation
type IDConfig struct {
	Format IDFormat `json:"format"`
	Prefix string   `json:"prefix,omitempty"`
}

// DefaultConfigs provides sensible defaults for different use cases
var DefaultConfigs = map[string]IDConfig{
	"general":     {Format: FormatProquint, Prefix: ""},
	"aws":         {Format: FormatCompact, Prefix: "aws"},
	"kubernetes":  {Format: FormatCompact, Prefix: "k8s"},
	"collections": {Format: FormatProquint, Prefix: "col"},
	"legacy":      {Format: FormatLegacy, Prefix: ""},
}

// Proquint implementation - converts numbers to pronounceable quintuplets
// Based on the proquint specification: https://arxiv.org/html/0901.4016

// Consonants and vowels for proquint encoding
var (
	consonants = "bdfghjklmnpqrstvz"
	vowels     = "aiou"
)

// numberToProquint converts a 32-bit number to a proquint string
func numberToProquint(n uint32) string {
	// Split 32-bit number into 4 bytes
	bytes := []byte{
		byte((n >> 24) & 0xFF),
		byte((n >> 16) & 0xFF),
		byte((n >> 8) & 0xFF),
		byte(n & 0xFF),
	}

	// Convert each pair of bytes to a quintuplet
	q1 := bytesToQuintuplet(bytes[0], bytes[1])
	q2 := bytesToQuintuplet(bytes[2], bytes[3])

	return q1 + "-" + q2
}

// bytesToQuintuplet converts two bytes to a 5-character quintuplet
func bytesToQuintuplet(high, low byte) string {
	// Combine bytes into 16-bit value
	val := uint16(high)<<8 | uint16(low)

	// Extract 5 components (4 bits each for consonants, 2 bits each for vowels)
	c1 := (val >> 12) & 0x0F  // bits 15-12
	v1 := (val >> 10) & 0x03  // bits 11-10
	c2 := (val >> 6) & 0x0F   // bits 9-6
	v2 := (val >> 4) & 0x03   // bits 5-4
	c3 := val & 0x0F          // bits 3-0

	return string([]byte{
		consonants[c1],
		vowels[v1],
		consonants[c2],
		vowels[v2],
		consonants[c3],
	})
}

// proquintToNumber converts a proquint string back to a 32-bit number
func proquintToNumber(proquint string) (uint32, error) {
	// Remove hyphens and validate length
	clean := strings.ReplaceAll(proquint, "-", "")
	if len(clean) != 10 {
		return 0, fmt.Errorf("invalid proquint length: expected 10 characters, got %d", len(clean))
	}

	// Convert each quintuplet to bytes
	q1 := clean[:5]
	q2 := clean[5:]

	high, err := quintupletToBytes(q1)
	if err != nil {
		return 0, fmt.Errorf("invalid first quintuplet: %w", err)
	}

	low, err := quintupletToBytes(q2)
	if err != nil {
		return 0, fmt.Errorf("invalid second quintuplet: %w", err)
	}

	// Combine into 32-bit number
	return uint32(high[0])<<24 | uint32(high[1])<<16 | uint32(low[0])<<8 | uint32(low[1]), nil
}

// quintupletToBytes converts a 5-character quintuplet to two bytes
func quintupletToBytes(q string) ([2]byte, error) {
	if len(q) != 5 {
		return [2]byte{}, fmt.Errorf("quintuplet must be 5 characters, got %d", len(q))
	}

	// Find indices in consonant/vowel arrays
	c1 := findIndex(consonants, q[0])
	v1 := findIndex(vowels, q[1])
	c2 := findIndex(consonants, q[2])
	v2 := findIndex(vowels, q[3])
	c3 := findIndex(consonants, q[4])

	if c1 < 0 || v1 < 0 || c2 < 0 || v2 < 0 || c3 < 0 {
		return [2]byte{}, fmt.Errorf("invalid characters in quintuplet: %s", q)
	}

	// Reconstruct 16-bit value
	val := uint16(c1)<<12 | uint16(v1)<<10 | uint16(c2)<<6 | uint16(v2)<<4 | uint16(c3)

	return [2]byte{byte(val >> 8), byte(val & 0xFF)}, nil
}

// findIndex finds the index of a character in a string
func findIndex(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
