package importer

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// The single-document decoders: what a non-collection import parses for schema
// inference and identity extraction.
//
// These replace a content-defined-chunking parser that cut the source at
// rolling-hash boundaries, dropped any chunk whose bytes it had already seen,
// and re-buffered the pieces in format processors. Repeated byte runs vanished
// from parsed values, documents with repeated elements failed to parse, and CSV
// rows straddling a boundary were split or skipped. The standard decoders read
// the stream in order and never discard input.

// DecodeFile decodes a whole file in the given format. It is the entry point
// for callers outside the import pipeline (e.g. re-inference) that need the
// same parsed shape an import produced.
func DecodeFile(path, format string) (interface{}, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	return decodeDocument(file, format)
}

// decodeDocument parses r according to format. It returns the parsed value and
// the number of records it holds:
//
//   - json:   one value; a top-level array counts its elements
//   - yaml:   one value per document; several documents become a list
//   - ndjson: a list of records, one per non-blank line
//   - csv:    a list of rows keyed by the header, cells kept as text
//   - other:  a descriptive object for the raw content
//
// JSON numbers decode as json.Number so integers beyond 2^53 survive intact.
func decodeDocument(r io.Reader, format string) (interface{}, int, error) {
	switch format {
	case "json":
		return decodeJSON(r)
	case "yaml":
		return decodeYAML(r)
	case "ndjson":
		return decodeNDJSON(r)
	case "csv":
		return decodeCSV(r)
	default:
		return describeUnstructured(r, format)
	}
}

func decodeJSON(r io.Reader) (interface{}, int, error) {
	dec := json.NewDecoder(bufio.NewReader(r))
	dec.UseNumber()
	var value interface{}
	if err := dec.Decode(&value); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, 0, fmt.Errorf("failed to parse JSON data: empty file (EOF)")
		}
		return nil, 0, fmt.Errorf("failed to parse JSON data: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, 0, fmt.Errorf("failed to parse JSON data: unexpected content after the top-level value")
	}
	if list, ok := value.([]interface{}); ok {
		return list, len(list), nil
	}
	return value, 1, nil
}

func decodeYAML(r io.Reader) (interface{}, int, error) {
	dec := yaml.NewDecoder(bufio.NewReader(r))
	var docs []interface{}
	for {
		var doc interface{}
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, 0, fmt.Errorf("failed to parse YAML data: %w", err)
		}
		if doc == nil {
			continue // an empty document between separators is not a record
		}
		docs = append(docs, doc)
	}
	switch len(docs) {
	case 0:
		return nil, 0, fmt.Errorf("failed to parse YAML data: empty file (EOF)")
	case 1:
		return docs[0], 1, nil
	default:
		return docs, len(docs), nil
	}
}

func decodeNDJSON(r io.Reader) (interface{}, int, error) {
	var records []interface{}
	_, err := streamNDJSON(r, func(_ int, raw json.RawMessage) error {
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.UseNumber()
		var record interface{}
		if err := dec.Decode(&record); err != nil {
			return err
		}
		records = append(records, record)
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse NDJSON data: %w", err)
	}
	if len(records) == 0 {
		return nil, 0, fmt.Errorf("failed to parse NDJSON data: no records")
	}
	return records, len(records), nil
}

// decodeCSV reads a header row and returns every following row as a map from
// column name to cell text. Cells are kept verbatim: "007" stays "007" and an
// empty cell stays "", so values that look numeric are never reformatted.
func decodeCSV(r io.Reader) (interface{}, int, error) {
	reader := csv.NewReader(bufio.NewReader(r))
	reader.FieldsPerRecord = -1 // ragged rows are tolerated; missing cells are absent
	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, 0, fmt.Errorf("failed to parse CSV data: empty file (EOF)")
		}
		return nil, 0, fmt.Errorf("failed to parse CSV header: %w", err)
	}

	rows := []map[string]string{}
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, 0, fmt.Errorf("failed to parse CSV data: %w", err)
		}
		row := make(map[string]string, len(header))
		for i, column := range header {
			if i < len(record) {
				row[column] = record[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, len(rows), nil
}

// describeUnstructured summarizes content that is not a structured format, so
// inference still has something to classify.
func describeUnstructured(r io.Reader, format string) (interface{}, int, error) {
	content, err := io.ReadAll(r)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read data: %w", err)
	}
	if format == "" {
		format = "unknown"
	}
	if !utf8.Valid(content) {
		return map[string]interface{}{
			"format":     format,
			"encoding":   "binary",
			"byte_count": len(content),
		}, 1, nil
	}
	text := string(content)
	lines := strings.Split(text, "\n")
	described := map[string]interface{}{
		"format":     format,
		"content":    text,
		"line_count": len(lines),
		"char_count": utf8.RuneCountInString(text),
		"byte_count": len(content),
		"first_line": lines[0],
	}
	if len(lines) > 1 {
		described["last_line"] = lines[len(lines)-1]
	}
	return described, 1, nil
}
