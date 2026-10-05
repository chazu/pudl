package importer

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chazu/pudl/internal/ingestprep"
	"github.com/klauspost/compress/zstd"
)

// DetectCompression detects the compression format of a file
// by checking both file extension and magic bytes
func DetectCompression(path string) string {
	ext := strings.ToLower(filepath.Ext(path))

	// Check extension first
	switch ext {
	case ".gz":
		return "gzip"
	case ".zst":
		return "zstd"
	}

	// Check magic bytes if extension is unclear
	file, err := os.Open(path)
	if err != nil {
		return "none"
	}
	defer file.Close()

	// Read first few bytes for magic number detection
	magic := make([]byte, 4)
	n, err := file.Read(magic)
	if err != nil || n < 2 {
		return "none"
	}

	// Check for gzip magic bytes (1f 8b)
	if n >= 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		return "gzip"
	}

	// Check for zstd magic bytes (28 b5 2f fd)
	if n >= 4 && magic[0] == 0x28 && magic[1] == 0xb5 && magic[2] == 0x2f && magic[3] == 0xfd {
		return "zstd"
	}

	return "none"
}

// WrapReader returns a decompressing reader for the given compression format
// If compression is "none", returns the original reader
func WrapReader(r io.Reader, compression string) (io.Reader, error) {
	switch compression {
	case "gzip":
		return gzip.NewReader(r)
	case "zstd":
		decoder, err := zstd.NewReader(r)
		if err != nil {
			return nil, fmt.Errorf("failed to create zstd decoder: %w", err)
		}
		return decoder, nil
	case "none":
		return r, nil
	default:
		return nil, fmt.Errorf("unsupported compression format: %s", compression)
	}
}

// DecompressFile decompresses a file into the system temp directory and returns
// the path to the decompressed file. If the file is not compressed, returns the
// original path.
func DecompressFile(sourcePath string) (string, error) {
	return DecompressTo(sourcePath, "")
}

// DecompressTo decompresses a file into dir ("" for the system temp directory)
// and returns the decompressed file's path. The name keeps the inner extension
// ("data.json.gz" → "pudl-decomp-*-data.json") so extension-based format
// detection sees the real format. If the file is not compressed, returns the
// original path.
func DecompressTo(sourcePath, dir string) (string, error) {
	return DecompressToLimited(context.Background(), sourcePath, dir, 1<<63-1)
}

func DecompressToLimited(ctx context.Context, sourcePath, dir string, maxBytes int64) (string, error) {
	compression := DetectCompression(sourcePath)
	if compression == "none" {
		return sourcePath, nil
	}

	// Open source file
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return "", fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	// Create decompressing reader
	decompReader, err := WrapReader(sourceFile, compression)
	if err != nil {
		return "", err
	}
	if closer, ok := decompReader.(io.Closer); ok {
		defer func() { _ = closer.Close() }()
	}
	if closer, ok := decompReader.(interface{ Close() }); ok {
		defer closer.Close()
	}

	// Create temporary file for decompressed data
	tmpFile, err := os.CreateTemp(dir, "pudl-decomp-*-"+innerName(sourcePath))
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %w", err)
	}

	// Copy decompressed data to temporary file
	_, copyErr := io.Copy(tmpFile, &ingestprep.Reader{Context: ctx, Source: decompReader, Remaining: maxBytes})
	closeErr := tmpFile.Close()
	if copyErr != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to decompress file: %w", copyErr)
	}
	if closeErr != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write decompressed file: %w", closeErr)
	}

	return tmpFile.Name(), nil
}

// innerName strips a compression extension from a file's base name.
func innerName(path string) string {
	base := filepath.Base(path)
	switch strings.ToLower(filepath.Ext(base)) {
	case ".gz", ".zst":
		return strings.TrimSuffix(base, filepath.Ext(base))
	}
	return base
}
