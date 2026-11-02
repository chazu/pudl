package secrets

import (
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The high-entropy heuristic is deliberately conservative: a value must be a
// single token of base64/base64url/alphanumeric characters, long enough, mix
// lower case, upper case and digits, and carry close to the per-character
// entropy of a random string. Cloud identifiers — numeric IDs, hex digests,
// UUIDs, lower-case resource names, self-links, ARNs, short etags — fail one
// of these tests.
const (
	minEntropyLen = 20
	maxEntropyLen = 4096
)

// tokenRE is the character set a random token is written in.
var tokenRE = regexp.MustCompile(`^[A-Za-z0-9+/=_\-]+$`)

var (
	hexRE  = regexp.MustCompile(`^[0-9a-fA-F]+$`)
	uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// nonSecretWords are field-name words whose values are digests or identifiers.
var nonSecretWords = map[string]bool{
	"sha": true, "sha1": true, "sha256": true, "sha512": true, "md5": true, "crc32c": true,
	"digest": true, "etag": true, "fingerprint": true, "checksum": true, "hash": true,
	"id": true, "ids": true, "uid": true, "uuid": true,
}

// highEntropy reports whether value looks like a random secret.
func highEntropy(value, key string) bool {
	if n := len(value); n < minEntropyLen || n > maxEntropyLen || !tokenRE.MatchString(value) {
		return false
	}
	if hexRE.MatchString(value) || uuidRE.MatchString(value) || nonSecretName(key) {
		return false
	}
	if !randomToken(pathlessToken(value)) {
		return false
	}
	return !encodedText(value)
}

// randomToken applies the length, character-mix and entropy tests.
func randomToken(token string) bool {
	n := len(token)
	return n >= minEntropyLen && classCount(token) >= 3 && shannon(token) >= entropyThreshold(n)
}

// pathlessToken returns the part of value to judge. A value with '/' but
// without base64's other tell-tales ('+', '=' padding) reads as a path or
// "owner/name" slug, so only its longest segment is judged.
func pathlessToken(value string) string {
	if !strings.Contains(value, "/") || strings.ContainsAny(value, "+=") {
		return value
	}
	longest := ""
	for _, seg := range strings.Split(value, "/") {
		if len(seg) > len(longest) {
			longest = seg
		}
	}
	return longest
}

// entropyThreshold is the bits per character a value of length n must reach.
// A random string's entropy is capped by log2(n) for short strings, so the
// bar rises with length toward that of random base64.
func entropyThreshold(n int) float64 {
	return math.Min(4.3, 0.92*math.Log2(float64(n)))
}

// shannon returns the Shannon entropy of s in bits per character.
func shannon(s string) float64 {
	counts := map[rune]int{}
	for _, r := range s {
		counts[r]++
	}
	n := float64(len(s))
	h := 0.0
	for _, c := range counts {
		p := float64(c) / n
		h -= p * math.Log2(p)
	}
	return h
}

// classCount counts the character classes (lower, upper, digit) in s.
func classCount(s string) int {
	var lower, upper, digit int
	for _, r := range s {
		switch {
		case unicode.IsLower(r):
			lower = 1
		case unicode.IsUpper(r):
			upper = 1
		case unicode.IsDigit(r):
			digit = 1
		}
	}
	return lower + upper + digit
}

// nonSecretName reports a field name built from a digest/identifier word, such
// as etag, labelFingerprint, md5Hash, private_key_id or uid.
func nonSecretName(key string) bool {
	for _, w := range keyWords(key) {
		if nonSecretWords[w] {
			return true
		}
	}
	return false
}

// keyWords splits a field name into lower-case words at separators and
// lower-to-upper camelCase boundaries.
func keyWords(key string) []string {
	var words []string
	var cur strings.Builder
	prevLower := false
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, strings.ToLower(cur.String()))
			cur.Reset()
		}
	}
	for _, r := range key {
		switch {
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			flush()
			prevLower = false
			continue
		case unicode.IsUpper(r) && prevLower:
			flush()
		}
		cur.WriteRune(r)
		prevLower = unicode.IsLower(r) || unicode.IsDigit(r)
	}
	flush()
	return words
}

// normalizeKey lower-cases key and drops separators: SecretAccessKey and
// secret_access_key both become secretaccesskey.
func normalizeKey(key string) string {
	return strings.Join(keyWords(key), "")
}

// encodedText reports a base64 value that decodes to readable text — encoded
// configuration, certificates or names rather than random key material.
func encodedText(value string) bool {
	decoded, ok := decodeBase64(value)
	if !ok || len(decoded) == 0 || !utf8.Valid(decoded) {
		return false
	}
	for _, r := range string(decoded) {
		if !unicode.IsPrint(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}
