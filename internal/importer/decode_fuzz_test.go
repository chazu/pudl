package importer

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Seeds reproduce the shapes the old content-defined-chunking parser lost:
// long repeated values, identical large list items, CSV leading zeros and
// empty cells, and integers beyond 2^53.
func jsonFuzzSeeds() [][]byte {
	repeated, _ := json.Marshal(map[string]any{"kind": "Thing", "blob": strings.Repeat("z", 70000), "tail": "end"})
	labels := map[string]any{}
	for i := 0; i < 400; i++ {
		labels[fmt.Sprintf("l%04d", i)] = "shared-value"
	}
	items := make([]any, 6)
	for i := range items {
		items[i] = map[string]any{"kind": "Pod", "metadata": map[string]any{"labels": labels, "name": "same"}}
	}
	identical, _ := json.Marshal(map[string]any{"items": items})
	return [][]byte{
		repeated,
		identical,
		[]byte(`{"id":9007199254740993,"n":1.0,"e":1e400,"neg":-0}`),
		[]byte(`[1,2,{"a":[3,{"b":null}]}]`),
		[]byte(`  {"a":1}  `),
		[]byte(`{"a":1} {"b":2}`),
		[]byte(`{"a":`),
		[]byte(``),
		[]byte(`"\ud800"`),
		[]byte("\xef\xbb\xbf{}"),
	}
}

// FuzzDecodeJSONMatchesStdlib checks decodeJSON against encoding/json: it
// succeeds exactly when the input is one valid JSON value, and then yields the
// value encoding/json decodes with UseNumber.
func FuzzDecodeJSONMatchesStdlib(f *testing.F) {
	for _, seed := range jsonFuzzSeeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		got, count, err := decodeDocument(bytes.NewReader(data), "json")
		if !json.Valid(data) {
			if err == nil {
				t.Fatalf("decoded invalid JSON %q", data)
			}
			return
		}
		if err != nil {
			t.Fatalf("rejected valid JSON %q: %v", data, err)
		}
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		var want any
		if err := dec.Decode(&want); err != nil {
			t.Fatalf("stdlib rejected json.Valid input: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("value mismatch for %q:\n got %#v\nwant %#v", data, got, want)
		}
		wantCount := 1
		if list, ok := want.([]any); ok {
			wantCount = len(list)
		}
		if count != wantCount {
			t.Fatalf("count %d, want %d", count, wantCount)
		}
	})
}

// FuzzDecodeYAMLMatchesYAMLv3 checks decodeYAML never panics and returns the
// non-empty documents yaml.v3 decodes, in order.
func FuzzDecodeYAMLMatchesYAMLv3(f *testing.F) {
	for _, seed := range []string{
		"a: 1\nb: [x, y]\n",
		"---\na: 1\n---\nb: 2\n",
		"---\n---\na: 1\n",
		"zip: 00001\nempty:\n",
		"big: 9007199254740993\n",
		"blob: " + strings.Repeat("z", 20000) + "\n",
		"a: &x [1, 2]\nb: *x\n",
		"? [a, b]\n: c\n",
		"a: .nan\n",
		"- 1\n- 2\n",
		"\t",
		"",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data string) {
		want, wantErr := referenceYAMLDocs(data)
		got, count, err := decodeDocument(strings.NewReader(data), "yaml")
		if wantErr != nil || len(want) == 0 {
			if err == nil {
				t.Fatalf("decoded %q to %#v, reference error %v / %d docs", data, got, wantErr, len(want))
			}
			return
		}
		if err != nil {
			t.Fatalf("rejected %q that yaml.v3 decodes: %v", data, err)
		}
		if count != len(want) {
			t.Fatalf("count %d, want %d", count, len(want))
		}
		var expect any = want
		if len(want) == 1 {
			expect = want[0]
		}
		if !reflect.DeepEqual(got, expect) && fmt.Sprintf("%#v", got) != fmt.Sprintf("%#v", expect) {
			t.Fatalf("value mismatch for %q:\n got %#v\nwant %#v", data, got, expect)
		}
	})
}

// referenceYAMLDocs decodes every document with yaml.v3 directly, dropping
// empty documents the way an import does.
func referenceYAMLDocs(data string) ([]any, error) {
	dec := yaml.NewDecoder(strings.NewReader(data))
	var docs []any
	for {
		var doc any
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return nil, err
		}
		if doc != nil {
			docs = append(docs, doc)
		}
	}
}

// FuzzDecodeCSVVerbatim checks decodeCSV yields one row per encoding/csv
// record after the header, with every cell kept verbatim under its column. A
// duplicated header name keeps its last column's cell.
func FuzzDecodeCSVVerbatim(f *testing.F) {
	var rows strings.Builder
	rows.WriteString("id,name,zip,note\n")
	for i := 0; i < 300; i++ {
		note := fmt.Sprintf("n%d", i)
		if i%5 == 0 {
			note = ""
		}
		fmt.Fprintf(&rows, "%d,name-%d,0%04d,%s\n", i, i, i, note)
	}
	for _, seed := range []string{
		rows.String(),
		"a,b\n007,\n",
		"a,a\n1,2\n",
		"a,b,c\n1\n1,2,3,4\n",
		"\"q\"\"x\",b\n\"multi\nline\",2\n",
		"a\n" + strings.Repeat("same\n", 2000),
		"",
		"a,\"b\n",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data string) {
		header, records, refErr := referenceCSV(data)
		got, count, err := decodeDocument(strings.NewReader(data), "csv")
		if refErr != nil {
			if err == nil {
				t.Fatalf("decoded %q that encoding/csv rejects: %v", data, refErr)
			}
			return
		}
		if err != nil {
			t.Fatalf("rejected %q that encoding/csv reads: %v", data, err)
		}
		rowsOut, ok := got.([]map[string]string)
		if !ok {
			t.Fatalf("got %T, want []map[string]string", got)
		}
		if count != len(records) || len(rowsOut) != len(records) {
			t.Fatalf("rows %d (count %d), want %d", len(rowsOut), count, len(records))
		}
		for r, record := range records {
			// A column holds the cell of its name's last occurrence that the
			// record reaches; a name the record never reaches is absent.
			want := map[string]string{}
			for col, name := range header {
				if col < len(record) {
					want[name] = record[col]
				}
			}
			if !reflect.DeepEqual(rowsOut[r], want) {
				t.Fatalf("row %d: %#v, want verbatim %#v", r, rowsOut[r], want)
			}
		}
	})
}

func referenceCSV(data string) ([]string, [][]string, error) {
	reader := csv.NewReader(strings.NewReader(data))
	reader.FieldsPerRecord = -1
	all, err := reader.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(all) == 0 {
		return nil, nil, io.EOF
	}
	return all[0], all[1:], nil
}
