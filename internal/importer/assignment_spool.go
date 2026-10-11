package importer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/validator"
)

// maxItemExplanations caps the per-item classification traces an import keeps.
const maxItemExplanations = 32

// spooledAssignment is one record's schema assignment as the pre-pass made it.
// The main pass reuses it instead of re-inferring, because redaction may leave
// values that no longer satisfy the schema the original record matched — the
// classification must describe the data as it arrived.
type spooledAssignment struct {
	Schema     string                      `json:"schema"`
	Confidence float64                     `json:"confidence"`
	Reason     string                      `json:"reason,omitempty"`
	SourcePath string                      `json:"source_path,omitempty"`
	Trace      *inference.InferenceTrace   `json:"trace,omitempty"`
	Validation *validator.ValidationResult `json:"validation,omitempty"`
	Validated  bool                        `json:"validated,omitempty"`
}

func (s spooledAssignment) assignment() schemaAssignment {
	return schemaAssignment(s)
}

// assignmentReader replays spooled assignments in record order. Records are
// prepared strictly in order, so one sequential reader suffices and memory
// stays one record deep.
type assignmentReader struct {
	file *os.File
	dec  *json.Decoder
	next int
}

func openAssignments(path string) (*assignmentReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bufio.NewReader(file))
	dec.UseNumber()
	return &assignmentReader{file: file, dec: dec}, nil
}

// read returns the assignment for record index, which must be the next one.
func (r *assignmentReader) read(index int) (schemaAssignment, error) {
	if index != r.next {
		return schemaAssignment{}, fmt.Errorf("assignment spool out of order: want record %d, at %d", index, r.next)
	}
	var s spooledAssignment
	if err := r.dec.Decode(&s); err != nil {
		return schemaAssignment{}, fmt.Errorf("read assignment for record %d: %w", index, err)
	}
	r.next++
	return s.assignment(), nil
}

func (r *assignmentReader) close() {
	if r != nil && r.file != nil {
		_ = r.file.Close()
	}
}
