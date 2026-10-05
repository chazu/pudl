package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/database"
	"gopkg.in/yaml.v3"
)

func validateExportFormat(format string) error {
	switch strings.ToLower(format) {
	case "json", "yaml", "ndjson", "csv":
		return nil
	}
	return fmt.Errorf("unknown export format %q: use json, yaml, csv, or ndjson", format)
}

// prepareExport uses normalized memberships; a collection's provenance file is
// not itself a record. Each record lives in memory only while being decoded.
func prepareExport(ctx context.Context, db *database.CatalogDB, entries []database.CatalogEntry, partial bool) (*os.File, int, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	spool, err := os.CreateTemp("", "pudl-export-*.ndjson")
	if err != nil {
		return nil, 0, 0, err
	}
	count, omitted := 0, 0
	seen := map[string]bool{}
	fail := func(err error) (*os.File, int, int, error) {
		spool.Close()
		os.Remove(spool.Name())
		return nil, 0, 0, err
	}
	var visit func(database.CatalogEntry, bool) error
	visit = func(entry database.CatalogEntry, member bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if seen[entry.ID] {
			return nil
		}
		seen[entry.ID] = true
		if entry.CollectionType != nil && *entry.CollectionType == "collection" {
			items, err := db.GetCollectionItems(entry.ID)
			if err != nil {
				return err
			}
			for _, item := range items {
				if err := visit(item, true); err != nil {
					return err
				}
			}
			return nil
		}
		offset, err := spool.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		before := count
		err = walkExportFile(ctx, entry.StoredPath, entry.Format, !member, func(value any) error {
			if err := json.NewEncoder(spool).Encode(value); err != nil {
				return err
			}
			count++
			return nil
		})
		if err == nil {
			return nil
		}
		// A malformed source contributes no prefix even with --allow-partial.
		if truncErr := spool.Truncate(offset); truncErr != nil {
			return truncErr
		}
		if _, seekErr := spool.Seek(offset, io.SeekStart); seekErr != nil {
			return seekErr
		}
		count = before
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !partial {
			return fmt.Errorf("export entry %s: %w", entry.ID, err)
		}
		omitted++
		fmt.Fprintf(errw(), "Omitted entry %s: %v\n", entry.ID, err)
		return nil
	}
	for _, entry := range entries {
		if err := visit(entry, false); err != nil {
			return fail(err)
		}
	}
	return spool, count, omitted, nil
}

func walkExportFile(ctx context.Context, path, format string, expand bool, emit func(any) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	checked := func(v any) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return emit(v)
	}
	switch format {
	case "json":
		// Peek past whitespace without consuming the first token.
		for {
			b, err := reader.Peek(1)
			if err != nil {
				return err
			}
			if !bytes.ContainsRune([]byte(" \t\r\n"), rune(b[0])) {
				break
			}
			if _, err := reader.ReadByte(); err != nil {
				return err
			}
		}
		dec := json.NewDecoder(reader)
		dec.UseNumber()
		first, _ := reader.Peek(1)
		if expand && first[0] == '[' {
			if _, err := dec.Token(); err != nil {
				return err
			}
			for dec.More() {
				var v any
				if err := dec.Decode(&v); err != nil {
					return err
				}
				if err := checked(v); err != nil {
					return err
				}
			}
			if _, err := dec.Token(); err != nil {
				return err
			}
		} else {
			var v any
			if err := dec.Decode(&v); err != nil {
				return err
			}
			if err := checked(v); err != nil {
				return err
			}
		}
		var trailing any
		if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
			return fmt.Errorf("unexpected data after JSON value")
		}
		return nil
	case "ndjson":
		for line := 1; ; line++ {
			raw, readErr := reader.ReadBytes('\n')
			if len(bytes.TrimSpace(raw)) > 0 {
				dec := json.NewDecoder(bytes.NewReader(raw))
				dec.UseNumber()
				var v any
				if err := dec.Decode(&v); err != nil {
					return fmt.Errorf("line %d: %w", line, err)
				}
				var trailing any
				if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
					return fmt.Errorf("line %d has trailing content", line)
				}
				if err := checked(v); err != nil {
					return err
				}
			}
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			if readErr != nil {
				return readErr
			}
		}
	case "yaml":
		dec := yaml.NewDecoder(reader)
		for {
			var node yaml.Node
			err := dec.Decode(&node)
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			v, err := exactExportYAML(&node)
			if err != nil {
				return err
			}
			if v == nil {
				continue
			}
			if list, ok := v.([]any); ok && expand {
				for _, item := range list {
					if err := checked(item); err != nil {
						return err
					}
				}
			} else if err := checked(v); err != nil {
				return err
			}
		}
	case "csv":
		dec := csv.NewReader(reader)
		dec.FieldsPerRecord = -1
		header, err := dec.Read()
		if err != nil {
			return err
		}
		for {
			row, err := dec.Read()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			v := map[string]any{}
			for i, key := range header {
				if i < len(row) {
					v[key] = row[i]
				}
			}
			if err := checked(v); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported stored format %q", format)
	}
}

func rewindExport(spool *os.File) (*json.Decoder, error) {
	if _, err := spool.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(spool)
	dec.UseNumber()
	return dec, nil
}

func writeExportSpool(w io.Writer, spool *os.File, count int, format string, pretty bool) error {
	dec, err := rewindExport(spool)
	if err != nil {
		return err
	}
	if format == "csv" {
		return writeExportCSV(w, spool)
	}
	enc := json.NewEncoder(w)
	if pretty && format == "json" {
		enc.SetIndent("", "  ")
	}
	var yenc *yaml.Encoder
	if format == "yaml" {
		yenc = yaml.NewEncoder(w)
	}
	array := format == "json" && count != 1
	if array {
		if _, err = io.WriteString(w, "[\n"); err != nil {
			return err
		}
	}
	for i := 0; i < count; i++ {
		var v any
		if err := dec.Decode(&v); err != nil {
			return err
		}
		if yenc != nil {
			if err := yenc.Encode(exportYAMLValue(v)); err != nil {
				_ = yenc.Close()
				return err
			}
			continue
		}
		if array && i > 0 {
			if _, err = io.WriteString(w, ",\n"); err != nil {
				return err
			}
		}
		if err := enc.Encode(v); err != nil {
			return err
		}
	}
	if array {
		_, err = io.WriteString(w, "]\n")
		return err
	}
	if yenc != nil {
		return yenc.Close()
	}
	return nil
}

// yaml.v3 treats json.Number as text. Preserve numeric tags and tokens instead.
func exportYAMLValue(v any) *yaml.Node {
	switch x := v.(type) {
	case json.Number:
		tag := "!!int"
		if strings.ContainsAny(x.String(), ".eE") {
			tag = "!!float"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: x.String()}
	case map[string]any:
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		keys := make([]string, 0, len(x))
		for key := range x {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, exportYAMLValue(x[key]))
		}
		return n
	case []any:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range x {
			n.Content = append(n.Content, exportYAMLValue(item))
		}
		return n
	default:
		n := &yaml.Node{}
		_ = n.Encode(v)
		return n
	}
}

func writeExportCSV(w io.Writer, spool *os.File) error {
	dec, err := rewindExport(spool)
	if err != nil {
		return err
	}
	keys := map[string]bool{}
	for {
		var v map[string]any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || v == nil {
			return fmt.Errorf("CSV export requires object records")
		}
		for key := range v {
			keys[key] = true
		}
	}
	headers := make([]string, 0, len(keys))
	for key := range keys {
		headers = append(headers, key)
	}
	sort.Strings(headers)
	dec, err = rewindExport(spool)
	if err != nil {
		return err
	}
	enc := csv.NewWriter(w)
	if err := enc.Write(headers); err != nil {
		return err
	}
	for {
		var v map[string]any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		row := make([]string, len(headers))
		for i, key := range headers {
			if val, ok := v[key]; ok && val != nil {
				switch cell := val.(type) {
				case map[string]any, []any:
					return fmt.Errorf("CSV field %q contains a nested value", key)
				default:
					row[i] = fmt.Sprint(cell)
				}
			}
		}
		if err := enc.Write(row); err != nil {
			return err
		}
	}
	enc.Flush()
	return enc.Error()
}

func publishExport(path string, write func(io.Writer) error) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".pudl-export-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if info, err := os.Stat(path); err == nil {
		if err := f.Chmod(info.Mode().Perm()); err != nil {
			f.Close()
			return err
		}
	}
	if err := write(f); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
