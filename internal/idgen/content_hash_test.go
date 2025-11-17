package idgen

import (
	"regexp"
	"testing"
)

func TestComputeContentID(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string // Known SHA256 hash
	}{
		{
			name:     "empty data",
			data:     []byte{},
			expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:     "hello world",
			data:     []byte("hello world"),
			expected: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
		{
			name:     "json data",
			data:     []byte(`{"name":"test","value":123}`),
			expected: "d2e872b46aef2ab8b02a8548b6fe746a33d33cbe1442ca0603ecfe645882fb2d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := ComputeContentID(tt.data)
			if id != tt.expected {
				t.Errorf("ComputeContentID() = %v, want %v", id, tt.expected)
			}

			// Verify determinism - same data should always produce same ID
			id2 := ComputeContentID(tt.data)
			if id != id2 {
				t.Errorf("ComputeContentID() is not deterministic: %v != %v", id, id2)
			}
		})
	}
}

func TestHashToProquint(t *testing.T) {
	tests := []struct {
		name     string
		hashHex  string
		expected string
	}{
		{
			name:     "known hash to proquint",
			hashHex:  "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
			expected: "qojas-fitun", // First 32 bits: 0xb94d27b9
		},
		{
			name:     "another hash",
			hashHex:  "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
			expected: "hibig-kutog", // First 32 bits: 0x44136fa3
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proquint := HashToProquint(tt.hashHex)
			if proquint != tt.expected {
				t.Errorf("HashToProquint() = %v, want %v", proquint, tt.expected)
			}
		})
	}
}

func TestGenerateRandomProquint(t *testing.T) {
	// Test that random proquints match the expected format
	proquintPattern := `^[bdfghjklmnpqrstvz][aiou][bdfghjklmnpqrstvz][aiou][bdfghjklmnpqrstvz]-[bdfghjklmnpqrstvz][aiou][bdfghjklmnpqrstvz][aiou][bdfghjklmnpqrstvz]$`
	re := regexp.MustCompile(proquintPattern)

	// Generate multiple proquints to test
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		proquint := GenerateRandomProquint()

		if !re.MatchString(proquint) {
			t.Errorf("GenerateRandomProquint() = %v, doesn't match pattern", proquint)
		}

		// Verify uniqueness (should be extremely rare to get duplicates in 100 tries)
		if seen[proquint] {
			t.Errorf("GenerateRandomProquint() generated duplicate: %v", proquint)
		}
		seen[proquint] = true
	}

	if len(seen) < 95 {
		t.Errorf("GenerateRandomProquint() didn't generate enough unique values: %d/100", len(seen))
	}
}

func TestNumberToProquint(t *testing.T) {
	tests := []struct {
		name     string
		number   uint32
		expected string
	}{
		{
			name:     "zero",
			number:   0,
			expected: "babab-babab",
		},
		{
			name:     "max uint32",
			number:   0xFFFFFFFF,
			expected: "vuvuv-vuvuv",
		},
		{
			name:     "known value",
			number:   127,
			expected: "babab-baduv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proquint := NumberToProquint(tt.number)
			if proquint != tt.expected {
				t.Errorf("NumberToProquint(%d) = %v, want %v", tt.number, proquint, tt.expected)
			}
		})
	}
}

func TestProquintToNumber(t *testing.T) {
	tests := []struct {
		name     string
		proquint string
		expected uint32
		wantErr  bool
	}{
		{
			name:     "zero",
			proquint: "babab-babab",
			expected: 0,
			wantErr:  false,
		},
		{
			name:     "max value",
			proquint: "vuvuv-vuvuv",
			expected: 0xFFFFFFFF,
			wantErr:  false,
		},
		{
			name:     "known value",
			proquint: "babab-baduv",
			expected: 127,
			wantErr:  false,
		},
		{
			name:     "without hyphens",
			proquint: "bababbabab",
			expected: 0,
			wantErr:  false,
		},
		{
			name:     "invalid length",
			proquint: "babab",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "invalid characters",
			proquint: "xxxxx-xxxxx",
			expected: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			num, err := ProquintToNumber(tt.proquint)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProquintToNumber() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && num != tt.expected {
				t.Errorf("ProquintToNumber(%v) = %v, want %v", tt.proquint, num, tt.expected)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	// Test that we can convert number -> proquint -> number
	testNumbers := []uint32{0, 1, 127, 255, 1000, 65535, 1000000, 0xFFFFFFFF}

	for _, num := range testNumbers {
		t.Run("roundtrip", func(t *testing.T) {
			proquint := NumberToProquint(num)
			result, err := ProquintToNumber(proquint)
			if err != nil {
				t.Errorf("ProquintToNumber() error = %v", err)
			}
			if result != num {
				t.Errorf("Round trip failed: %d -> %v -> %d", num, proquint, result)
			}
		})
	}
}

func TestContentIDToProquintWorkflow(t *testing.T) {
	// Test the complete workflow: data -> hash -> proquint
	testData := []byte(`{"user":"john","id":123}`)

	// Step 1: Compute content ID
	contentID := ComputeContentID(testData)
	if len(contentID) != 64 { // SHA256 produces 64 hex characters
		t.Errorf("Content ID should be 64 characters, got %d", len(contentID))
	}

	// Step 2: Convert to proquint for display
	proquint := HashToProquint(contentID)
	if len(proquint) != 11 { // Format: "xxxxx-xxxxx"
		t.Errorf("Proquint should be 11 characters, got %d", len(proquint))
	}

	// Verify proquint format
	proquintPattern := `^[bdfghjklmnpqrstvz][aiou][bdfghjklmnpqrstvz][aiou][bdfghjklmnpqrstvz]-[bdfghjklmnpqrstvz][aiou][bdfghjklmnpqrstvz][aiou][bdfghjklmnpqrstvz]$`
	re := regexp.MustCompile(proquintPattern)
	if !re.MatchString(proquint) {
		t.Errorf("Proquint %v doesn't match expected pattern", proquint)
	}

	// Verify determinism - same data always produces same proquint
	contentID2 := ComputeContentID(testData)
	proquint2 := HashToProquint(contentID2)
	if proquint != proquint2 {
		t.Errorf("Proquint conversion is not deterministic: %v != %v", proquint, proquint2)
	}
}

func TestHashToUint32(t *testing.T) {
	tests := []struct {
		name     string
		hashHex  string
		expected uint32
	}{
		{
			name:     "valid hash",
			hashHex:  "b94d27b9934d3e08",
			expected: 0xb94d27b9,
		},
		{
			name:     "short hash",
			hashHex:  "abc",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HashToUint32(tt.hashHex)
			if result != tt.expected {
				t.Errorf("HashToUint32() = %v, want %v", result, tt.expected)
			}
		})
	}
}
