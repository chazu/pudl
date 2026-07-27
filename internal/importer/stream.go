package importer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// The bounded half of the import pipeline: stage the source once while hashing
// it, then decode it one record at a time.
//
// The path this replaces read every record into a `[]interface{}` before writing
// anything, so peak memory was the whole decoded record set — for NDJSON and
// large JSON arrays, unboundedly larger than the file itself.

// recordSink consumes one decoded record.
//
// Raw bytes rather than a decoded value: an item's content hash is computed from
// its canonical JSON anyway, so handing the sink the bytes avoids a
// decode/re-encode round trip and keeps each record's residency to one buffer.
type recordSink func(index int, raw json.RawMessage) error

// streamNDJSON decodes newline-delimited JSON, one record at a time.
//
// Blank lines are skipped rather than counted: a trailing newline is ordinary,
// and an empty record is not a record.
func streamNDJSON(r io.Reader, sink recordSink) (int, error) {
	reader := bufio.NewReader(r)
	index := 0
	for lineNumber := 1; ; lineNumber++ {
		line, err := reader.ReadBytes('\n')
		trimmed := trimSpaceBytes(line)

		if len(trimmed) > 0 {
			if !json.Valid(trimmed) {
				return index, fmt.Errorf("line %d is not valid JSON", lineNumber)
			}
			if sinkErr := sink(index, json.RawMessage(trimmed)); sinkErr != nil {
				return index, sinkErr
			}
			index++
		}

		if err == io.EOF {
			return index, nil
		}
		if err != nil {
			return index, fmt.Errorf("read line %d: %w", lineNumber, err)
		}
	}
}

// streamJSONArray decodes a top-level JSON array element by element.
//
// This is the case the architecture report calls out. It used to go through the
// chunking parser, which appended every decoded object into one slice, so a 1 GB
// array of small objects became a 1 GB+ slice before a single row was written.
func streamJSONArray(r io.Reader, sink recordSink) (int, error) {
	decoder := json.NewDecoder(bufio.NewReader(r))

	token, err := decoder.Token()
	if err != nil {
		return 0, fmt.Errorf("read array start: %w", err)
	}
	if delim, ok := token.(json.Delim); !ok || delim != '[' {
		return 0, fmt.Errorf("expected a JSON array, found %v", token)
	}

	index := 0
	for decoder.More() {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return index, fmt.Errorf("decode element %d: %w", index, err)
		}
		if err := sink(index, raw); err != nil {
			return index, err
		}
		index++
	}

	if _, err := decoder.Token(); err != nil {
		return index, fmt.Errorf("read array end: %w", err)
	}
	return index, nil
}

// trimSpaceBytes trims ASCII whitespace without allocating a string.
func trimSpaceBytes(b []byte) []byte {
	start := 0
	for start < len(b) && isSpaceByte(b[start]) {
		start++
	}
	end := len(b)
	for end > start && isSpaceByte(b[end-1]) {
		end--
	}
	return b[start:end]
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

// streamableCollectionFormat reports the streaming decoder for a source, if it
// has one.
//
// NDJSON is detected upstream. A top-level JSON *array* is the case the
// architecture report calls out: it used to be routed through the chunking
// parser, which materialized every element, so this looks at the first
// non-whitespace byte to route it to the element-by-element decoder instead. A
// JSON object stays on the single-record path, where "the whole thing in memory"
// is one record and therefore already bounded.
func streamableCollectionFormat(sourcePath, format string) (string, bool) {
	switch format {
	case "ndjson":
		return "ndjson", true
	case "json":
		if firstJSONByte(sourcePath) == '[' {
			return "json-array", true
		}
	}
	return "", false
}

// firstJSONByte returns the first non-whitespace byte of a file, or 0.
func firstJSONByte(path string) byte {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return 0
		}
		if !isSpaceByte(b) {
			return b
		}
	}
}
