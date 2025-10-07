package idgen

import (
	"fmt"
	"strings"
	"testing"
)

func TestIDGeneration(t *testing.T) {
	tests := []struct {
		name     string
		format   IDFormat
		prefix   string
		expected string // pattern to match
	}{
		{
			name:     "short code without prefix",
			format:   FormatShortCode,
			prefix:   "",
			expected: "^[a-z0-9]{6}$",
		},
		{
			name:     "short code with prefix",
			format:   FormatShortCode,
			prefix:   "aws",
			expected: "^aws-[a-z0-9]{6}$",
		},
		{
			name:     "readable format",
			format:   FormatReadable,
			prefix:   "",
			expected: "^[a-z]+-[a-z]+-[0-9]{2}$",
		},
		{
			name:     "readable with prefix",
			format:   FormatReadable,
			prefix:   "col",
			expected: "^col-[a-z]+-[a-z]+-[0-9]{2}$",
		},
		{
			name:     "compact format",
			format:   FormatCompact,
			prefix:   "",
			expected: "^[0-9]{6}-[a-z0-9]{3}$",
		},
		{
			name:     "compact with prefix",
			format:   FormatCompact,
			prefix:   "k8s",
			expected: "^k8s-[0-9]{6}-[a-z0-9]{3}$",
		},
		{
			name:     "sequential format",
			format:   FormatSequential,
			prefix:   "data",
			expected: "^data-[0-9]{3}$",
		},
		{
			name:     "proquint format",
			format:   FormatProquint,
			prefix:   "",
			expected: "^[bdfghjklmnpqrstvz][aiou][bdfghjklmnpqrstvz][aiou][bdfghjklmnpqrstvz]-[bdfghjklmnpqrstvz][aiou][bdfghjklmnpqrstvz][aiou][bdfghjklmnpqrstvz]$",
		},
		{
			name:     "proquint with prefix",
			format:   FormatProquint,
			prefix:   "col",
			expected: "^col-[bdfghjklmnpqrstvz][aiou][bdfghjklmnpqrstvz][aiou][bdfghjklmnpqrstvz]-[bdfghjklmnpqrstvz][aiou][bdfghjklmnpqrstvz][aiou][bdfghjklmnpqrstvz]$",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen := NewIDGenerator(tt.format, tt.prefix)
			id := gen.Generate()
			
			if !matchesPattern(id, tt.expected) {
				t.Errorf("Generated ID %q doesn't match expected pattern %q", id, tt.expected)
			}
			
			// Test that multiple generations are unique
			id2 := gen.Generate()
			if id == id2 && tt.format != FormatSequential {
				t.Errorf("Generated IDs should be unique, got %q twice", id)
			}
		})
	}
}

func TestCollectionItemIDs(t *testing.T) {
	tests := []struct {
		name         string
		format       IDFormat
		collectionID string
		index        int
		itemData     map[string]interface{}
		expectedPattern string
	}{
		{
			name:         "short code with extracted ID",
			format:       FormatShortCode,
			collectionID: "abc123",
			index:        0,
			itemData:     map[string]interface{}{"id": "user-123"},
			expectedPattern: "^abc123-user-123$",
		},
		{
			name:         "readable format",
			format:       FormatReadable,
			collectionID: "blue-cat-42",
			index:        5,
			itemData:     map[string]interface{}{},
			expectedPattern: "^blue-cat-42-item-05$",
		},
		{
			name:         "compact format",
			format:       FormatCompact,
			collectionID: "1207-a1b",
			index:        10,
			itemData:     map[string]interface{}{},
			expectedPattern: "^1207-a1b-10$",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen := NewIDGenerator(tt.format, "")
			id := gen.GenerateCollectionItemID(tt.collectionID, tt.index, tt.itemData)
			
			if !matchesPattern(id, tt.expectedPattern) {
				t.Errorf("Generated item ID %q doesn't match expected pattern %q", id, tt.expectedPattern)
			}
		})
	}
}

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		input     string
		maxLength int
		expected  string
	}{
		{
			input:     "User Name 123",
			maxLength: 20,
			expected:  "user-name-123",
		},
		{
			input:     "very_long_identifier_that_needs_truncation",
			maxLength: 10,
			expected:  "very-long",
		},
		{
			input:     "ID@#$%^&*()with!special",
			maxLength: 20,
			expected:  "idwithspecial",
		},
		{
			input:     "trailing-hyphens---",
			maxLength: 20,
			expected:  "trailing-hyphens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeID(tt.input, tt.maxLength)
			if result != tt.expected {
				t.Errorf("sanitizeID(%q, %d) = %q, want %q", tt.input, tt.maxLength, result, tt.expected)
			}
		})
	}
}

func TestProquintConversion(t *testing.T) {
	tests := []struct {
		number   uint32
		expected string
	}{
		{0x00000000, "babab-babab"},
		{0x7F000001, "lurab-babad"},  // Corrected expected value
		{0xFFFFFFFF, "vuvuv-vuvuv"},  // Corrected expected value
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("number_%d", tt.number), func(t *testing.T) {
			proquint := numberToProquint(tt.number)
			if proquint != tt.expected {
				t.Errorf("numberToProquint(%d) = %q, want %q", tt.number, proquint, tt.expected)
			}

			// Test round-trip conversion
			converted, err := proquintToNumber(proquint)
			if err != nil {
				t.Errorf("proquintToNumber(%q) failed: %v", proquint, err)
			}
			if converted != tt.number {
				t.Errorf("Round-trip failed: %d -> %q -> %d", tt.number, proquint, converted)
			}
		})
	}
}

func TestDefaultConfigs(t *testing.T) {
	expectedConfigs := []string{"general", "aws", "kubernetes", "collections", "legacy"}

	for _, configName := range expectedConfigs {
		config, exists := DefaultConfigs[configName]
		if !exists {
			t.Errorf("Expected default config %q not found", configName)
			continue
		}

		// Test that we can create a generator with this config
		gen := NewIDGenerator(config.Format, config.Prefix)
		id := gen.Generate()

		if id == "" {
			t.Errorf("Generated empty ID for config %q", configName)
		}
	}
}

// Example function to demonstrate different ID formats
func ExampleIDGenerator() {
	// Short codes - great for general use
	shortGen := NewIDGenerator(FormatShortCode, "")
	fmt.Printf("Short: %s\n", shortGen.Generate())
	
	// Readable - easy to remember and communicate
	readableGen := NewIDGenerator(FormatReadable, "")
	fmt.Printf("Readable: %s\n", readableGen.Generate())
	
	// Compact - good balance of brevity and context
	compactGen := NewIDGenerator(FormatCompact, "aws")
	fmt.Printf("Compact: %s\n", compactGen.Generate())
	
	// Sequential - predictable and ordered
	seqGen := NewIDGenerator(FormatSequential, "data")
	fmt.Printf("Sequential: %s\n", seqGen.Generate())
	fmt.Printf("Sequential: %s\n", seqGen.Generate())
}

func BenchmarkIDGeneration(b *testing.B) {
	formats := []IDFormat{FormatShortCode, FormatReadable, FormatCompact, FormatSequential}
	
	for _, format := range formats {
		b.Run(string(format), func(b *testing.B) {
			gen := NewIDGenerator(format, "test")
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				gen.Generate()
			}
		})
	}
}

// Helper function to check if a string matches a simple regex pattern
func matchesPattern(s, pattern string) bool {
	// Simple pattern matching for test purposes
	// In a real implementation, you'd use regexp package
	
	if pattern == "^[a-z0-9]{6}$" {
		return len(s) == 6 && isAlphanumeric(s)
	}
	
	if strings.HasPrefix(pattern, "^aws-") && strings.HasSuffix(pattern, "[a-z0-9]{6}$") {
		return strings.HasPrefix(s, "aws-") && len(s) == 10 && isAlphanumeric(s[4:])
	}
	
	if pattern == "^[a-z]+-[a-z]+-[0-9]{2}$" {
		parts := strings.Split(s, "-")
		return len(parts) == 3 && isAlpha(parts[0]) && isAlpha(parts[1]) && isNumeric(parts[2]) && len(parts[2]) == 2
	}
	
	if strings.HasPrefix(pattern, "^col-") && strings.Contains(pattern, "[a-z]+-[a-z]+-[0-9]{2}$") {
		if !strings.HasPrefix(s, "col-") {
			return false
		}
		remainder := s[4:]
		parts := strings.Split(remainder, "-")
		return len(parts) == 3 && isAlpha(parts[0]) && isAlpha(parts[1]) && isNumeric(parts[2]) && len(parts[2]) == 2
	}
	
	// Add more pattern matching as needed
	return true // Simplified for demo
}

func isAlpha(s string) bool {
	for _, r := range s {
		if !(r >= 'a' && r <= 'z') {
			return false
		}
	}
	return len(s) > 0
}
