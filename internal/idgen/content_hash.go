package idgen

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"
)

// ComputeContentID generates a content-based ID from data using SHA256
// This provides deterministic, deduplication-friendly IDs
func ComputeContentID(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

// HashToProquint converts the first 32 bits of a hex hash string to a proquint
// This is used for display purposes - the full hash should be stored
func HashToProquint(hashHex string) string {
	// Take first 8 hex characters (32 bits)
	if len(hashHex) < 8 {
		// Fallback: generate random proquint if hash is too short
		return GenerateRandomProquint()
	}

	// Parse hex to uint32
	var num uint32
	fmt.Sscanf(hashHex[:8], "%x", &num)

	return NumberToProquint(num)
}

// GenerateRandomProquint generates a random proquint ID
// Used when content hashing isn't applicable (e.g., for collection items)
func GenerateRandomProquint() string {
	num, _ := rand.Int(rand.Reader, big.NewInt(0xFFFFFFFF))
	return NumberToProquint(uint32(num.Int64()))
}

// NumberToProquint converts a 32-bit number to a proquint string
// Based on the proquint specification: https://arxiv.org/html/0901.4016
func NumberToProquint(n uint32) string {
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

// ProquintToNumber converts a proquint string back to a 32-bit number
func ProquintToNumber(proquint string) (uint32, error) {
	// Remove hyphens
	clean := ""
	for _, ch := range proquint {
		if ch != '-' {
			clean += string(ch)
		}
	}

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

// Consonants and vowels for proquint encoding
var (
	consonants = "bdfghjklmnpqrstvz"
	vowels     = "aiou"
)

// bytesToQuintuplet converts two bytes to a 5-character quintuplet
func bytesToQuintuplet(high, low byte) string {
	// Combine bytes into 16-bit value
	val := uint16(high)<<8 | uint16(low)

	// Extract 5 components (4 bits each for consonants, 2 bits each for vowels)
	c1 := (val >> 12) & 0x0F // bits 15-12
	v1 := (val >> 10) & 0x03 // bits 11-10
	c2 := (val >> 6) & 0x0F  // bits 9-6
	v2 := (val >> 4) & 0x03  // bits 5-4
	c3 := val & 0x0F         // bits 3-0

	return string([]byte{
		consonants[c1],
		vowels[v1],
		consonants[c2],
		vowels[v2],
		consonants[c3],
	})
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

// HashToUint32 converts a hex hash string to a uint32 for use with proquint
// This is a helper function for converting stored hashes to display IDs
func HashToUint32(hashHex string) uint32 {
	if len(hashHex) < 8 {
		return 0
	}

	var num uint32
	fmt.Sscanf(hashHex[:8], "%x", &num)
	return num
}

// Uint32ToHash converts a uint32 back to a partial hash string
// Note: This only reconstructs the first 8 hex characters
func Uint32ToHash(num uint32) string {
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, num)
	return fmt.Sprintf("%08x", num)
}
