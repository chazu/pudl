package streaming

import (
	"context"
	"io"
	"time"
)

// StreamingParser defines the interface for streaming data parsers
type StreamingParser interface {
	// Parse processes an input stream and returns a channel of parsed chunks
	Parse(ctx context.Context, reader io.Reader) (<-chan ParsedChunk, <-chan error)
	
	// Configure updates the parser configuration
	Configure(config *StreamingConfig) error
	
	// Stats returns current parsing statistics
	Stats() ParsingStats
	
	// Close releases any resources held by the parser
	Close() error
}

// ChunkProcessor defines the interface for processing individual chunks
type ChunkProcessor interface {
	// ProcessChunk processes a single chunk of data
	ProcessChunk(chunk *CDCChunk) (*ProcessedChunk, error)

	// Finalize flushes any remaining buffered data at end of stream
	// Returns nil if no data remains in buffer
	Finalize() (*ProcessedChunk, error)

	// CanProcess returns true if this processor can handle the given data format
	CanProcess(data []byte) bool

	// FormatName returns the name of the format this processor handles
	FormatName() string

	// Reset clears internal state for reuse
	Reset()

	// GetBufferSize returns the current buffer size (0 if no buffering)
	GetBufferSize() int
}

// SchemaDetector defines the interface for detecting schemas from chunks
type SchemaDetector interface {
	// AddSample adds a chunk sample for schema detection
	AddSample(chunk *ProcessedChunk) error
	
	// DetectSchema returns the detected schema with confidence score
	DetectSchema() (*SchemaDetection, error)
	
	// Reset clears all samples and starts fresh
	Reset()
	
	// GetConfidence returns the current confidence level
	GetConfidence() float64
}

// MemoryMonitor defines the interface for monitoring memory usage
type MemoryMonitor interface {
	// CheckMemory returns current memory usage and whether limit is exceeded
	CheckMemory() (current int64, limit int64, exceeded bool)
	
	// SetLimit updates the memory limit
	SetLimit(limitMB int) error
	
	// GetStats returns memory usage statistics
	GetStats() MemoryStats
}

// ProgressReporter defines the interface for reporting parsing progress
type ProgressReporter interface {
	// Start begins progress reporting for an operation
	Start(total int64, operation string)
	
	// Update reports progress with current position and optional message
	Update(processed int64, message string)
	
	// Finish completes progress reporting with final result
	Finish(result ProcessingResult)
	
	// Error reports an error during processing
	Error(err error)
}

// CDCChunk represents a chunk produced by the CDC chunker
type CDCChunk struct {
	Data     []byte    // Raw chunk data
	Offset   int64     // Offset in the original stream
	Size     int       // Size of the chunk
	Hash     string    // Content hash for deduplication
	Sequence int       // Sequence number in the stream
	Time     time.Time // When the chunk was created
}

// ProcessedChunk represents a chunk after format-specific processing
type ProcessedChunk struct {
	Original    *CDCChunk              // Original CDC chunk
	Format      string                 // Detected format (json, csv, yaml, etc.)
	Objects     []interface{}          // Parsed objects from the chunk
	Metadata    map[string]interface{} // Extracted metadata
	Errors      []error                // Any parsing errors encountered
	Partial     bool                   // True if chunk contains partial objects
	Boundaries  []int                  // Object boundaries within the chunk
}

// SchemaDetection represents the result of schema detection
type SchemaDetection struct {
	SchemaName   string                 // Detected schema name
	Confidence   float64                // Confidence score (0-1)
	Samples      int                    // Number of samples used
	Metadata     map[string]interface{} // Schema metadata
	Alternatives []SchemaCandidate      // Alternative schema candidates
}

// SchemaCandidate represents an alternative schema possibility
type SchemaCandidate struct {
	Name       string  // Schema name
	Confidence float64 // Confidence score
	Reason     string  // Why this schema was considered
}

// ParsingStats holds statistics about the parsing process
type ParsingStats struct {
	BytesProcessed   int64         // Total bytes processed
	ChunksProcessed  int64         // Total chunks processed
	ObjectsExtracted int64         // Total objects extracted
	ErrorCount       int64         // Number of errors encountered
	StartTime        time.Time     // When parsing started
	Duration         time.Duration // Total processing time
	Throughput       float64       // MB/s processing rate
	MemoryUsage      int64         // Current memory usage in bytes
	DeduplicatedMB   float64       // Amount of data deduplicated
}

// MemoryStats holds memory usage statistics
type MemoryStats struct {
	CurrentMB int64   // Current memory usage in MB
	LimitMB   int64   // Memory limit in MB
	PeakMB    int64   // Peak memory usage in MB
	Usage     float64 // Usage percentage (0-1)
}

// ProcessingResult represents the final result of a parsing operation
type ProcessingResult struct {
	Success          bool          // Whether processing completed successfully
	BytesProcessed   int64         // Total bytes processed
	ObjectsExtracted int64         // Total objects extracted
	ErrorCount       int64         // Number of errors encountered
	Duration         time.Duration // Total processing time
	SchemaDetected   string        // Final detected schema
	Message          string        // Result message
}

// ParsedChunk represents the final output of the streaming parser
type ParsedChunk struct {
	// Core data
	Objects  []interface{}          // Parsed objects
	Metadata map[string]interface{} // Chunk metadata
	
	// Processing info
	Format     string    // Data format
	Schema     string    // Detected schema
	Confidence float64   // Schema confidence
	
	// Source info
	Offset   int64     // Original offset
	Size     int       // Chunk size
	Hash     string    // Content hash
	Sequence int       // Sequence number
	Time     time.Time // Processing time
	
	// Error info
	Errors   []error // Any errors encountered
	Warnings []string // Any warnings
}
