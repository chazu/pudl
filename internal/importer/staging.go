package importer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// stagedSource is a source file copied into raw storage, with the identity
// computed during the copy.
type stagedSource struct {
	// ContentHash is SHA256 of the raw source bytes — unchanged from when it was
	// computed by a separate pass. It is the import's identity, so it is
	// deliberately taken from the bytes as read, not from anything decoded.
	ContentHash string
	// Size is the byte count copied.
	Size int64
	// tempPath is where the bytes landed before being named. Empty once
	// committed.
	tempPath string
}

// stageSource copies a source file into a temp file beside its eventual home,
// hashing it in the same pass.
//
// One read of the source instead of two. The import used to hash the file, then
// decode it, then copy it — three passes, of which the first and third read the
// same bytes for two purposes that a MultiWriter serves at once.
//
// Writing to a temp file and renaming later is what makes staging atomic: a
// killed import leaves a temp file rather than a half-written record in the raw
// tree that a later read would treat as evidence.
func stageSource(sourcePath, rawDir string) (*stagedSource, error) {
	source, err := os.Open(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("open source: %w", err)
	}
	defer source.Close()

	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return nil, fmt.Errorf("create raw directory: %w", err)
	}
	temp, err := os.CreateTemp(rawDir, ".staging-*")
	if err != nil {
		return nil, fmt.Errorf("create staging file: %w", err)
	}

	hasher := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(temp, hasher), source)
	closeErr := temp.Close()
	if copyErr != nil {
		os.Remove(temp.Name())
		return nil, fmt.Errorf("stage source: %w", copyErr)
	}
	if closeErr != nil {
		os.Remove(temp.Name())
		return nil, fmt.Errorf("close staging file: %w", closeErr)
	}

	return &stagedSource{
		ContentHash: hex.EncodeToString(hasher.Sum(nil)),
		Size:        size,
		tempPath:    temp.Name(),
	}, nil
}

// Commit names the staged bytes, returning the final path.
func (s *stagedSource) Commit(finalPath string) (string, error) {
	if s.tempPath == "" {
		return finalPath, nil
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return "", fmt.Errorf("create raw directory: %w", err)
	}
	if err := os.Rename(s.tempPath, finalPath); err != nil {
		return "", fmt.Errorf("commit staged file: %w", err)
	}
	s.tempPath = ""
	return finalPath, nil
}

// Discard removes the staged bytes. Safe to call after Commit, and safe to call
// twice — a discarded import must not leave a temp file behind, and that is the
// path a duplicate takes.
func (s *stagedSource) Discard() {
	if s.tempPath == "" {
		return
	}
	os.Remove(s.tempPath)
	s.tempPath = ""
}

// Path is where the staged bytes currently are, for a reader that wants to
// decode them before they are committed.
func (s *stagedSource) Path(committed string) string {
	if s.tempPath != "" {
		return s.tempPath
	}
	return committed
}
