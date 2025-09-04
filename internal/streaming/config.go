package streaming

import (
	"time"
)

// StreamingConfig holds configuration for streaming parsers
type StreamingConfig struct {
	// CDC Configuration
	ChunkAlgorithm string `json:"chunk_algorithm" yaml:"chunk_algorithm" default:"fastcdc"`
	MinChunkSize   int    `json:"min_chunk_size" yaml:"min_chunk_size" default:"4096"`    // 4KB minimum
	MaxChunkSize   int    `json:"max_chunk_size" yaml:"max_chunk_size" default:"65536"`   // 64KB maximum
	AvgChunkSize   int    `json:"avg_chunk_size" yaml:"avg_chunk_size" default:"16384"`   // 16KB average

	// Privacy & Security
	UseKeyedCDC bool   `json:"use_keyed_cdc" yaml:"use_keyed_cdc" default:"false"`
	CDCKey      string `json:"cdc_key" yaml:"cdc_key" default:""`

	// Memory Management
	MaxMemoryMB int `json:"max_memory_mb" yaml:"max_memory_mb" default:"100"`         // Memory limit in MB
	BufferSize  int `json:"buffer_size" yaml:"buffer_size" default:"1048576"`        // 1MB buffer

	// Error Handling
	ErrorTolerance float64 `json:"error_tolerance" yaml:"error_tolerance" default:"0.1"` // 10% error tolerance
	SkipMalformed  bool    `json:"skip_malformed" yaml:"skip_malformed" default:"true"`  // Skip bad chunks

	// Schema Detection
	SampleSize int     `json:"sample_size" yaml:"sample_size" default:"100"`        // Chunks to sample
	Confidence float64 `json:"confidence" yaml:"confidence" default:"0.8"`          // 80% confidence threshold

	// Progress Reporting
	ReportEveryMB      int           `json:"report_every_mb" yaml:"report_every_mb" default:"1"`           // Progress every 1MB
	ProgressInterval   time.Duration `json:"progress_interval" yaml:"progress_interval" default:"1s"`      // Progress every 1 second
	BytesPerUpdate     int64         `json:"bytes_per_update" yaml:"bytes_per_update" default:"1048576"`   // 1MB per update

	// Concurrency
	MaxConcurrency int `json:"max_concurrency" yaml:"max_concurrency" default:"0"` // 0 = runtime.NumCPU()
}

// DefaultStreamingConfig returns a configuration with default values
func DefaultStreamingConfig() *StreamingConfig {
	return &StreamingConfig{
		ChunkAlgorithm:     "fastcdc",
		MinChunkSize:       4096,    // 4KB
		MaxChunkSize:       65536,   // 64KB
		AvgChunkSize:       16384,   // 16KB
		UseKeyedCDC:        false,
		CDCKey:             "",
		MaxMemoryMB:        100,
		BufferSize:         1048576, // 1MB
		ErrorTolerance:     0.1,     // 10%
		SkipMalformed:      true,
		SampleSize:         100,
		Confidence:         0.8, // 80%
		ReportEveryMB:      1,
		ProgressInterval:   time.Second,
		BytesPerUpdate:     1048576, // 1MB
		MaxConcurrency:     0,       // Use runtime.NumCPU()
	}
}

// Validate checks if the configuration is valid
func (c *StreamingConfig) Validate() error {
	if c.MinChunkSize <= 0 {
		return &ConfigError{Field: "min_chunk_size", Message: "must be positive"}
	}
	if c.MaxChunkSize <= c.MinChunkSize {
		return &ConfigError{Field: "max_chunk_size", Message: "must be greater than min_chunk_size"}
	}
	if c.AvgChunkSize < c.MinChunkSize || c.AvgChunkSize > c.MaxChunkSize {
		return &ConfigError{Field: "avg_chunk_size", Message: "must be between min_chunk_size and max_chunk_size"}
	}
	if c.MaxMemoryMB <= 0 {
		return &ConfigError{Field: "max_memory_mb", Message: "must be positive"}
	}
	if c.BufferSize <= 0 {
		return &ConfigError{Field: "buffer_size", Message: "must be positive"}
	}
	if c.ErrorTolerance < 0 || c.ErrorTolerance > 1 {
		return &ConfigError{Field: "error_tolerance", Message: "must be between 0 and 1"}
	}
	if c.SampleSize <= 0 {
		return &ConfigError{Field: "sample_size", Message: "must be positive"}
	}
	if c.Confidence < 0 || c.Confidence > 1 {
		return &ConfigError{Field: "confidence", Message: "must be between 0 and 1"}
	}
	return nil
}

// ConfigError represents a configuration validation error
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return "streaming config error: " + e.Field + " " + e.Message
}
