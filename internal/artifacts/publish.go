package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/chazu/pudl/internal/ingestprep"
	"io"
	"os"
	"path/filepath"
)

// Hash streams a file's exact bytes. Catalog content hashes may instead use
// canonical JSON; bundle integrity always uses this byte hash.
func Hash(path string) (string, int64, error) {
	return HashContext(context.Background(), path)
}

func HashContext(ctx context.Context, path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, &ingestprep.Reader{Context: ctx, Source: f, Remaining: 1<<63 - 1})
	return hex.EncodeToString(h.Sum(nil)), n, err
}

// Publish installs prepared bytes without overwriting an existing artifact.
// Call under WithLock. Equal existing bytes are reused; differing bytes fail.
func Publish(source, destination string) (bool, error) {
	return PublishContext(context.Background(), source, destination)
}

func PublishContext(ctx context.Context, source, destination string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return false, err
	}
	if info, err := os.Lstat(destination); err == nil {
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("existing artifact is not a regular file: %s", destination)
		}
		a, _, err := HashContext(ctx, source)
		if err != nil {
			return false, err
		}
		b, _, err := HashContext(ctx, destination)
		if err != nil {
			return false, err
		}
		if a != b {
			return false, fmt.Errorf("artifact collision at %s", destination)
		}
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.Rename(source, destination); err != nil {
		return false, err
	}
	return true, nil
}

// Journal records only newly published paths on disk, so abort cleanup does not
// retain a path slice proportional to an ingestion's record count.
type Journal struct {
	file *os.File
	ctx  context.Context
}

func NewJournal(dir string) (*Journal, error) {
	return NewJournalContext(context.Background(), dir)
}

func NewJournalContext(ctx context.Context, dir string) (*Journal, error) {
	f, err := os.CreateTemp(dir, "published-*")
	if err != nil {
		return nil, err
	}
	return &Journal{file: f, ctx: ctx}, nil
}

func (j *Journal) Publish(source, destination string) error {
	created, err := PublishContext(j.ctx, source, destination)
	if err != nil || !created {
		return err
	}
	if err := json.NewEncoder(j.file).Encode(destination); err != nil {
		_ = os.Remove(destination)
		return err
	}
	return nil
}

// Rollback is called after SQL rollback, still holding the artifact lock.
func (j *Journal) Rollback() {
	if _, err := j.file.Seek(0, io.SeekStart); err != nil {
		return
	}
	dec := json.NewDecoder(j.file)
	for {
		var path string
		if err := dec.Decode(&path); err != nil {
			return
		}
		_ = os.Remove(path)
	}
}

func (j *Journal) Close() { _ = j.file.Close(); _ = os.Remove(j.file.Name()) }
