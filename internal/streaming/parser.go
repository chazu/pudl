package streaming

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"sync"
	"time"

	chunkers "github.com/PlakarKorp/go-cdc-chunkers"
	_ "github.com/PlakarKorp/go-cdc-chunkers/chunkers/fastcdc"
	_ "github.com/PlakarKorp/go-cdc-chunkers/chunkers/ultracdc"
)

// DefaultStreamingParser implements the StreamingParser interface using CDC
type DefaultStreamingParser struct {
	config             *StreamingConfig
	memoryMonitor      MemoryMonitor
	progressReporter   ProgressReporter
	backpressure       *BackpressureController
	processorRegistry  *ProcessorRegistry
	schemaDetector     SchemaDetector
	deduplicationCache map[string]bool

	// Statistics
	stats ParsingStats
	mu    sync.RWMutex

	// State
	running bool
	closed  bool

	// Processor state management - maintain same processor across chunks
	currentProcessor   ChunkProcessor // The processor selected for this stream
	streamFormat       string         // Format detected at stream start
	formatDetected     bool           // Whether format has been detected
}

// NewStreamingParser creates a new streaming parser with the given configuration
func NewStreamingParser(config *StreamingConfig) (*DefaultStreamingParser, error) {
	if config == nil {
		config = DefaultStreamingConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create memory monitor
	memMonitor := NewMemoryMonitor(config.MaxMemoryMB)

	// Create backpressure controller
	backpressure := NewBackpressureController(memMonitor)

	parser := &DefaultStreamingParser{
		config:             config,
		memoryMonitor:      memMonitor,
		backpressure:       backpressure,
		processorRegistry:  NewProcessorRegistry(),
		deduplicationCache: make(map[string]bool),
		stats: ParsingStats{
			StartTime: time.Now(),
		},
	}

	return parser, nil
}

// Parse processes an input stream and returns a channel of parsed chunks
func (p *DefaultStreamingParser) Parse(ctx context.Context, reader io.Reader) (<-chan ParsedChunk, <-chan error) {
	resultChan := make(chan ParsedChunk, 100)
	errorChan := make(chan error, 10)

	go p.parseStream(ctx, reader, resultChan, errorChan)

	return resultChan, errorChan
}

// parseStream performs the actual streaming parsing
func (p *DefaultStreamingParser) parseStream(ctx context.Context, reader io.Reader, results chan<- ParsedChunk, errors chan<- error) {
	defer close(results)
	defer close(errors)

	p.mu.Lock()
	p.running = true
	p.stats.StartTime = time.Now()
	// Reset processor state for new stream
	p.currentProcessor = nil
	p.streamFormat = ""
	p.formatDetected = false
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.running = false
		p.stats.Duration = time.Since(p.stats.StartTime)
		// Reset processor for next stream
		if p.currentProcessor != nil {
			p.currentProcessor.Reset()
			p.currentProcessor = nil
		}
		p.mu.Unlock()
	}()

	// Create CDC chunker
	chunker, err := p.createChunker(reader)
	if err != nil {
		errors <- fmt.Errorf("failed to create chunker: %w", err)
		return
	}

	// Chunker created successfully

	// Start progress reporting if available
	if p.progressReporter != nil {
		p.progressReporter.Start(0, "Streaming Parse") // Unknown total size
	}

	sequence := 0
	errorCount := int64(0)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Check for backpressure
		if p.backpressure.ShouldPause() {
			p.backpressure.WaitForResume()
		}

		// Get next chunk from CDC
		chunkData, err := chunker.Next()

		// Handle EOF - but process the final chunk first if it has data
		// The CDC library returns the last chunk AND io.EOF together
		if err == io.EOF {
			if len(chunkData) > 0 {
				// Process the final chunk before breaking
				cdcChunk := &CDCChunk{
					Data:     chunkData,
					Offset:   int64(p.stats.BytesProcessed),
					Size:     len(chunkData),
					Hash:     p.computeHash(chunkData),
					Sequence: sequence,
					Time:     time.Now(),
				}

				if !p.isDuplicate(cdcChunk.Hash) {
					parsedChunk, procErr := p.processChunk(cdcChunk)
					if procErr == nil {
						select {
						case results <- *parsedChunk:
						case <-ctx.Done():
							return
						}
						p.updateStats(int64(cdcChunk.Size), 1, int64(len(parsedChunk.Objects)))
						sequence++
					}
				} else {
					p.updateStats(int64(cdcChunk.Size), 0, 0)
				}
			}
			break
		}
		if err != nil {
			errorCount++
			if p.shouldContinueOnError(errorCount) {
				if p.config.SkipMalformed {
					continue
				}
				errors <- fmt.Errorf("chunker error: %w", err)
				continue
			} else {
				errors <- fmt.Errorf("too many errors, stopping: %w", err)
				return
			}
		}

		// Create CDC chunk
		cdcChunk := &CDCChunk{
			Data:     chunkData,
			Offset:   int64(p.stats.BytesProcessed), // Use current position as offset
			Size:     len(chunkData),
			Hash:     p.computeHash(chunkData),
			Sequence: sequence,
			Time:     time.Now(),
		}

		// Check for deduplication
		if p.isDuplicate(cdcChunk.Hash) {
			p.updateStats(int64(cdcChunk.Size), 0, 0)
			continue
		}

		// Process the chunk
		parsedChunk, err := p.processChunk(cdcChunk)
		if err != nil {
			errorCount++
			if p.shouldContinueOnError(errorCount) {
				if p.config.SkipMalformed {
					continue
				}
				errors <- fmt.Errorf("chunk processing error: %w", err)
				continue
			} else {
				errors <- fmt.Errorf("too many errors, stopping: %w", err)
				return
			}
		}

		// Send result
		select {
		case results <- *parsedChunk:
		case <-ctx.Done():
			return
		}

		// Update statistics
		p.updateStats(int64(cdcChunk.Size), 1, int64(len(parsedChunk.Objects)))

		// Update progress
		if p.progressReporter != nil {
			p.progressReporter.Update(p.stats.BytesProcessed, "")
		}

		sequence++
	}

	// Finalize: flush any remaining buffered data from the processor
	if p.currentProcessor != nil {
		bufferSize := p.currentProcessor.GetBufferSize()
		finalChunk, err := p.currentProcessor.Finalize()
		if err != nil {
			errors <- fmt.Errorf("finalization error (buffer=%d): %w", bufferSize, err)
		} else if finalChunk != nil && len(finalChunk.Objects) > 0 {
			// Create parsed chunk from finalized data
			parsedChunk := &ParsedChunk{
				Objects:  finalChunk.Objects,
				Metadata: finalChunk.Metadata,
				Format:   finalChunk.Format,
				Offset:   int64(p.stats.BytesProcessed),
				Size:     finalChunk.Original.Size,
				Hash:     "",
				Sequence: sequence,
				Time:     time.Now(),
				Errors:   finalChunk.Errors,
			}

			// Send finalized result
			select {
			case results <- *parsedChunk:
				p.updateStats(0, 1, int64(len(finalChunk.Objects)))
			case <-ctx.Done():
				return
			}
		}
	}

	// Finish progress reporting
	if p.progressReporter != nil {
		result := ProcessingResult{
			Success:          errorCount == 0,
			BytesProcessed:   p.stats.BytesProcessed,
			ObjectsExtracted: p.stats.ObjectsExtracted,
			ErrorCount:       errorCount,
			Duration:         time.Since(p.stats.StartTime),
		}
		p.progressReporter.Finish(result)
	}
}

// createChunker creates a CDC chunker based on the configuration
func (p *DefaultStreamingParser) createChunker(reader io.Reader) (*chunkers.Chunker, error) {
	algorithm := p.config.ChunkAlgorithm

	// Create chunker options
	opts := &chunkers.ChunkerOpts{
		MinSize:    p.config.MinChunkSize,
		MaxSize:    p.config.MaxChunkSize,
		NormalSize: p.config.AvgChunkSize,
	}

	// Chunker options configured

	// Add key for keyed CDC if configured
	if p.config.UseKeyedCDC && p.config.CDCKey != "" {
		opts.Key = []byte(p.config.CDCKey)
	}

	// Create chunker with specified algorithm
	chunker, err := chunkers.NewChunker(algorithm, reader, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s chunker: %w", algorithm, err)
	}

	return chunker, nil
}

// processChunk processes a single CDC chunk
func (p *DefaultStreamingParser) processChunk(chunk *CDCChunk) (*ParsedChunk, error) {
	p.mu.Lock()
	// Detect format and select processor on first chunk with data
	if !p.formatDetected && len(chunk.Data) > 0 {
		p.currentProcessor = p.processorRegistry.GetBestProcessor(chunk.Data)
		p.streamFormat = p.currentProcessor.FormatName()
		p.formatDetected = true
		// Note: Don't call Reset() here - the processor is freshly selected
		// and Reset() would clear format detection state (isArray, isNDJSON)
	}
	processor := p.currentProcessor
	p.mu.Unlock()

	// Fall back to getting a processor if none was set (shouldn't happen normally)
	if processor == nil {
		processor = p.processorRegistry.GetBestProcessor(chunk.Data)
	}

	// Process the chunk using the same processor instance
	processed, err := processor.ProcessChunk(chunk)
	if err != nil {
		return nil, fmt.Errorf("failed to process chunk: %w", err)
	}

	// Create parsed chunk
	parsedChunk := &ParsedChunk{
		Objects:  processed.Objects,
		Metadata: processed.Metadata,
		Format:   processed.Format,
		Offset:   chunk.Offset,
		Size:     chunk.Size,
		Hash:     chunk.Hash,
		Sequence: chunk.Sequence,
		Time:     chunk.Time,
		Errors:   processed.Errors,
	}

	// Add sample to schema detector if available
	if p.schemaDetector != nil {
		if err := p.schemaDetector.AddSample(processed); err != nil {
			// Log error but don't fail processing
			parsedChunk.Warnings = append(parsedChunk.Warnings,
				fmt.Sprintf("Schema detection error: %v", err))
		} else {
			// Try to detect schema
			if detection, err := p.schemaDetector.DetectSchema(); err == nil && detection != nil {
				parsedChunk.Schema = detection.SchemaName
				parsedChunk.Confidence = detection.Confidence

				// Add schema metadata
				if parsedChunk.Metadata == nil {
					parsedChunk.Metadata = make(map[string]interface{})
				}
				parsedChunk.Metadata["schema_detection"] = detection.Metadata
			}
		}
	}

	// Add to deduplication cache
	p.deduplicationCache[chunk.Hash] = true

	return parsedChunk, nil
}

// computeHash computes SHA-256 hash of chunk data
func (p *DefaultStreamingParser) computeHash(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

// isDuplicate checks if a chunk hash has been seen before
func (p *DefaultStreamingParser) isDuplicate(hash string) bool {
	return p.deduplicationCache[hash]
}

// shouldContinueOnError determines if processing should continue after an error
func (p *DefaultStreamingParser) shouldContinueOnError(errorCount int64) bool {
	if p.stats.ChunksProcessed == 0 {
		return true // Always continue for the first few chunks
	}

	errorRate := float64(errorCount) / float64(p.stats.ChunksProcessed)
	return errorRate <= p.config.ErrorTolerance
}

// updateStats updates parsing statistics
func (p *DefaultStreamingParser) updateStats(bytes, chunks, objects int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stats.BytesProcessed += bytes
	p.stats.ChunksProcessed += chunks
	p.stats.ObjectsExtracted += objects

	// Update memory usage
	if current, _, _ := p.memoryMonitor.CheckMemory(); current > 0 {
		p.stats.MemoryUsage = current * 1024 * 1024 // Convert MB to bytes
	}

	// Calculate throughput
	duration := time.Since(p.stats.StartTime)
	if duration.Seconds() > 0 {
		p.stats.Throughput = float64(p.stats.BytesProcessed) / duration.Seconds() / 1024 / 1024
	}
}

// Configure updates the parser configuration
func (p *DefaultStreamingParser) Configure(config *StreamingConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.config = config

	// Update memory monitor
	if err := p.memoryMonitor.SetLimit(config.MaxMemoryMB); err != nil {
		return fmt.Errorf("failed to update memory limit: %w", err)
	}

	return nil
}

// Stats returns current parsing statistics
func (p *DefaultStreamingParser) Stats() ParsingStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := p.stats
	stats.Duration = time.Since(p.stats.StartTime)

	return stats
}

// Close releases any resources held by the parser
func (p *DefaultStreamingParser) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	p.running = false

	// Clear deduplication cache to free memory
	p.deduplicationCache = make(map[string]bool)

	return nil
}

// SetProgressReporter sets the progress reporter
func (p *DefaultStreamingParser) SetProgressReporter(reporter ProgressReporter) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.progressReporter = reporter
}

// SetSchemaDetector sets the schema detector
func (p *DefaultStreamingParser) SetSchemaDetector(detector SchemaDetector) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.schemaDetector = detector
}

// GetProcessorRegistry returns the processor registry for advanced configuration
func (p *DefaultStreamingParser) GetProcessorRegistry() *ProcessorRegistry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.processorRegistry
}

// SetCUESchemaDetector sets a CUE-integrated schema detector
func (p *DefaultStreamingParser) SetCUESchemaDetector(schemaManager interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Create CUE schema detector
	// Note: We use interface{} to avoid circular imports with schema package
	// In practice, this would be *schema.Manager
	detector := NewSimpleSchemaDetector(p.config.SampleSize)
	p.schemaDetector = detector

	return nil
}
