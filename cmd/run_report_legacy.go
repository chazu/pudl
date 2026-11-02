package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/acute"
)

// legacyFields returns the top-level fields of a stored report that the
// current report type does not know — converge results, approval state and
// sealed bindings from the retired convergence workflow — so a report reader
// can show them as recorded instead of dropping them.
func legacyFields(payload []byte, report any) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&fields); err != nil {
		return nil, fmt.Errorf("decode stored report fields: %w", err)
	}
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	for name := range jsonFieldNames(reflect.TypeOf(report)) {
		delete(fields, name)
	}
	return fields, nil
}

// jsonFieldNames lists the JSON names of a struct type's exported fields.
func jsonFieldNames(t reflect.Type) map[string]bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	names := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		switch name {
		case "-":
			continue
		case "":
			name = field.Name
		}
		names[name] = true
	}
	return names
}

// writeLegacyFields renders fields a report reader no longer understands as
// indented JSON, in name order.
func writeLegacyFields(w io.Writer, fields map[string]json.RawMessage) error {
	if len(fields) == 0 {
		return nil
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Fprintln(w, "\n## legacy fields")
	fmt.Fprintln(w, "Recorded by a retired workflow (see docs/retired-commands.md); shown as stored.")
	for _, name := range names {
		var indented bytes.Buffer
		if err := json.Indent(&indented, fields[name], "", "  "); err != nil {
			return fmt.Errorf("format legacy field %q: %w", name, err)
		}
		fmt.Fprintf(w, "\n### %s\n```json\n%s\n```\n", name, indented.String())
	}
	return nil
}

// printLegacyFields shows the stored report's fields that report's type no
// longer has.
func printLegacyFields(payload []byte, report any) error {
	fields, err := legacyFields(payload, report)
	if err != nil {
		return err
	}
	return writeLegacyFields(outw(), fields)
}

// printSetLegacyFields shows a stored set report's unknown fields, including
// each member's (such as mutation_required), keyed by member.
func printSetLegacyFields(payload []byte) error {
	fields, err := legacyFields(payload, acute.RunSetReport{})
	if err != nil {
		return err
	}
	var stored struct {
		Members []json.RawMessage `json:"members"`
	}
	if err := json.Unmarshal(payload, &stored); err != nil {
		return fmt.Errorf("decode stored set members: %w", err)
	}
	for i, member := range stored.Members {
		memberFields, err := legacyFields(member, acute.RunSetMemberReport{})
		if err != nil {
			return err
		}
		if len(memberFields) == 0 {
			continue
		}
		encoded, err := json.Marshal(memberFields)
		if err != nil {
			return err
		}
		fields[fmt.Sprintf("members[%d]", i)] = encoded
	}
	return writeLegacyFields(outw(), fields)
}
