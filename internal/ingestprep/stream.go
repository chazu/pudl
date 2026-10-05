// Package ingestprep bounds the preparation of imported record streams.
// Preparation owns private temporary files; publication and catalog commits
// remain the responsibility of the caller.
package ingestprep

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// Limits bound decoded input, individual JSON values, and private staged bytes.
// Zero fields select defaults. Negative fields are invalid, not unlimited.
type Limits struct {
	RecordBytes  int64
	DecodedBytes int64
	StagingBytes int64
}

func (l Limits) Resolve() (Limits, error) {
	if l.RecordBytes < 0 || l.DecodedBytes < 0 || l.StagingBytes < 0 {
		return l, fmt.Errorf("ingestion limits must be nonnegative")
	}
	if l.RecordBytes == 0 {
		l.RecordBytes = 64 << 20
	}
	if l.DecodedBytes == 0 {
		l.DecodedBytes = 1 << 30
	}
	if l.StagingBytes == 0 {
		l.StagingBytes = 2 << 30
	}
	return l, nil
}

// Reader checks cancellation and rejects input beyond the limit. It probes
// one extra byte rather than disguising an exceeded limit as clean EOF.
type Reader struct {
	Context   context.Context
	Source    io.Reader
	Remaining int64
}

func (r *Reader) Read(p []byte) (int, error) {
	if r.Context == nil {
		r.Context = context.Background()
	}
	if err := r.Context.Err(); err != nil {
		return 0, err
	}
	if r.Remaining == 0 {
		var probe [1]byte
		n, err := r.Source.Read(probe[:])
		if n > 0 {
			return 0, fmt.Errorf("decoded input exceeds ingestion byte limit")
		}
		return 0, err
	}
	if int64(len(p)) > r.Remaining {
		p = p[:r.Remaining]
	}
	n, err := r.Source.Read(p)
	r.Remaining -= int64(n)
	return n, err
}

// Value reads one JSON value without allocating beyond maxBytes. The framing
// scanner only locates a boundary; encoding/json remains the syntax validator.
func Value(r *bufio.Reader, maxBytes int64) (json.RawMessage, error) {
	var raw []byte
	depth, quoted, escaped := 0, false, false
	started, compound := false, false
	for {
		b, err := r.Peek(1)
		if err != nil {
			if err == io.EOF && started && !quoted && depth == 0 {
				break
			}
			return nil, err
		}
		c := b[0]
		if !started && space(c) {
			_, _ = r.ReadByte()
			continue
		}
		if started && !quoted && depth == 0 && (space(c) || c == ',' || c == ']' || c == '}') {
			break
		}
		if int64(len(raw)) >= maxBytes {
			return nil, fmt.Errorf("JSON value exceeds record byte limit (%d)", maxBytes)
		}
		_, _ = r.ReadByte()
		raw = append(raw, c)
		if !started {
			started = true
			compound = c == '{' || c == '['
		}
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
				if !compound && depth == 0 {
					break
				}
			}
			continue
		}
		switch c {
		case '"':
			quoted = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				goto complete
			}
		}
	}
complete:
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid JSON value")
	}
	return raw, nil
}

func space(b byte) bool { return b == ' ' || b == '\t' || b == '\r' || b == '\n' }

// Array streams complete top-level array values and requires clean EOF.
func Array(r io.Reader, maxBytes int64, emit func(int, json.RawMessage) error) (int, error) {
	b := bufio.NewReader(r)
	if err := delimiter(b, '['); err != nil {
		return 0, err
	}
	index := 0
	for {
		c, err := next(b)
		if err != nil {
			return index, err
		}
		if c == ']' {
			break
		}
		if err := b.UnreadByte(); err != nil {
			return index, err
		}
		raw, err := Value(b, maxBytes)
		if err != nil {
			return index, fmt.Errorf("record %d: %w", index, err)
		}
		if err := emit(index, raw); err != nil {
			return index, err
		}
		index++
		c, err = next(b)
		if err != nil {
			return index, err
		}
		if c == ']' {
			break
		}
		if c != ',' {
			return index, fmt.Errorf("expected array comma")
		}
		c, err = next(b)
		if err != nil {
			return index, err
		}
		if c == ']' {
			return index, fmt.Errorf("trailing array comma")
		}
		if err := b.UnreadByte(); err != nil {
			return index, err
		}
	}
	if _, err := next(b); err != io.EOF {
		if err != nil {
			return index, err
		}
		return index, fmt.Errorf("unexpected content after array")
	}
	return index, nil
}

func delimiter(r *bufio.Reader, want byte) error {
	c, err := next(r)
	if err != nil {
		return err
	}
	if c != want {
		return fmt.Errorf("expected %q", want)
	}
	return nil
}

func next(r *bufio.Reader) (byte, error) {
	for {
		c, err := r.ReadByte()
		if err != nil || !space(c) {
			return c, err
		}
	}
}

// NDJSON limits each line while reading, including whitespace, and skips blanks.
func NDJSON(r io.Reader, maxBytes int64, emit func(int, json.RawMessage) error) (int, error) {
	b := bufio.NewReader(r)
	index := 0
	for {
		var line []byte
		var err error
		for {
			var part []byte
			part, err = b.ReadSlice('\n')
			if int64(len(line)+len(part)) > maxBytes {
				return index, fmt.Errorf("NDJSON line exceeds record byte limit")
			}
			line = append(line, part...)
			if err != bufio.ErrBufferFull {
				break
			}
		}
		blank := true
		for _, c := range line {
			if !space(c) {
				blank = false
				break
			}
		}
		if !blank {
			if !json.Valid(line) {
				return index, fmt.Errorf("record %d is not valid JSON", index)
			}
			if emitErr := emit(index, line); emitErr != nil {
				return index, emitErr
			}
			index++
		}
		if err == io.EOF {
			return index, nil
		}
		if err != nil {
			return index, err
		}
	}
}
