package streaming

import (
	"fmt"
	"sync"
	"time"
)

// CLIProgressReporter implements ProgressReporter for command-line interface
type CLIProgressReporter struct {
	mu           sync.RWMutex
	total        int64
	processed    int64
	operation    string
	startTime    time.Time
	lastUpdate   time.Time
	updateInterval time.Duration
	verbose      bool
	finished     bool
}

// NewCLIProgressReporter creates a new CLI progress reporter
func NewCLIProgressReporter(verbose bool) *CLIProgressReporter {
	return &CLIProgressReporter{
		updateInterval: time.Second,
		verbose:        verbose,
	}
}

// Start begins progress reporting for an operation
func (p *CLIProgressReporter) Start(total int64, operation string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.total = total
	p.processed = 0
	p.operation = operation
	p.startTime = time.Now()
	p.lastUpdate = time.Now()
	p.finished = false
	
	if p.verbose {
		fmt.Printf("🚀 Starting %s", operation)
		if total > 0 {
			fmt.Printf(" (%s)", formatBytes(total))
		}
		fmt.Println()
	}
}

// Update reports progress with current position and optional message
func (p *CLIProgressReporter) Update(processed int64, message string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.finished {
		return
	}
	
	p.processed = processed
	now := time.Now()
	
	// Only update if enough time has passed
	if now.Sub(p.lastUpdate) < p.updateInterval {
		return
	}
	p.lastUpdate = now
	
	// Calculate progress
	var percentage float64
	if p.total > 0 {
		percentage = float64(processed) / float64(p.total) * 100
	}
	
	// Calculate rate
	duration := now.Sub(p.startTime)
	var rate float64
	if duration.Seconds() > 0 {
		rate = float64(processed) / duration.Seconds() / 1024 / 1024 // MB/s
	}
	
	// Format output
	if p.verbose {
		if p.total > 0 {
			fmt.Printf("\r📊 %s: %.1f%% (%s/%s) at %.1f MB/s", 
				p.operation, percentage, formatBytes(processed), formatBytes(p.total), rate)
		} else {
			fmt.Printf("\r📊 %s: %s at %.1f MB/s", 
				p.operation, formatBytes(processed), rate)
		}
		
		if message != "" {
			fmt.Printf(" - %s", message)
		}
	} else {
		// Simple progress for non-verbose mode
		if p.total > 0 {
			fmt.Printf("\rProcessing: %.1f%% (%s)", percentage, formatBytes(processed))
		} else {
			fmt.Printf("\rProcessed: %s", formatBytes(processed))
		}
	}
}

// Finish completes progress reporting with final result
func (p *CLIProgressReporter) Finish(result ProcessingResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.finished {
		return
	}
	p.finished = true
	
	// Clear the progress line
	fmt.Print("\r" + clearLine())
	
	if result.Success {
		fmt.Printf("✅ %s completed successfully\n", p.operation)
		if p.verbose {
			fmt.Printf("   📈 Processed: %s in %v\n", formatBytes(result.BytesProcessed), result.Duration)
			fmt.Printf("   📦 Objects: %d extracted\n", result.ObjectsExtracted)
			if result.ErrorCount > 0 {
				fmt.Printf("   ⚠️  Errors: %d encountered\n", result.ErrorCount)
			}
			if result.SchemaDetected != "" {
				fmt.Printf("   🔍 Schema: %s detected\n", result.SchemaDetected)
			}
			
			// Calculate final throughput
			if result.Duration.Seconds() > 0 {
				throughput := float64(result.BytesProcessed) / result.Duration.Seconds() / 1024 / 1024
				fmt.Printf("   ⚡ Throughput: %.1f MB/s\n", throughput)
			}
		}
	} else {
		fmt.Printf("❌ %s failed\n", p.operation)
		if result.Message != "" {
			fmt.Printf("   Error: %s\n", result.Message)
		}
		if result.ErrorCount > 0 {
			fmt.Printf("   Errors encountered: %d\n", result.ErrorCount)
		}
	}
}

// Error reports an error during processing
func (p *CLIProgressReporter) Error(err error) {
	p.mu.RLock()
	verbose := p.verbose
	p.mu.RUnlock()
	
	if verbose {
		fmt.Printf("\n⚠️  Error: %v\n", err)
	}
}

// SetUpdateInterval sets how often progress updates are displayed
func (p *CLIProgressReporter) SetUpdateInterval(interval time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.updateInterval = interval
}

// SilentProgressReporter implements ProgressReporter but produces no output
type SilentProgressReporter struct{}

// NewSilentProgressReporter creates a progress reporter that produces no output
func NewSilentProgressReporter() *SilentProgressReporter {
	return &SilentProgressReporter{}
}

func (p *SilentProgressReporter) Start(total int64, operation string)        {}
func (p *SilentProgressReporter) Update(processed int64, message string)    {}
func (p *SilentProgressReporter) Finish(result ProcessingResult)            {}
func (p *SilentProgressReporter) Error(err error)                           {}

// Helper functions

// formatBytes formats a byte count as a human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// clearLine returns a string that clears the current terminal line
func clearLine() string {
	// Get terminal width, default to 80 if we can't determine it
	width := 80
	if w := getTerminalWidth(); w > 0 {
		width = w
	}
	return fmt.Sprintf("%*s", width, " ")
}

// getTerminalWidth attempts to get the terminal width
func getTerminalWidth() int {
	// This is a simplified version - in a real implementation,
	// you might want to use a library like golang.org/x/term
	// For now, we'll just return a reasonable default
	return 80
}

// ProgressStats holds statistics about progress reporting
type ProgressStats struct {
	Operation      string        `json:"operation"`
	BytesProcessed int64         `json:"bytes_processed"`
	Total          int64         `json:"total"`
	Percentage     float64       `json:"percentage"`
	Duration       time.Duration `json:"duration"`
	Throughput     float64       `json:"throughput_mbps"`
	StartTime      time.Time     `json:"start_time"`
}

// GetStats returns current progress statistics
func (p *CLIProgressReporter) GetStats() ProgressStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	var percentage float64
	if p.total > 0 {
		percentage = float64(p.processed) / float64(p.total) * 100
	}
	
	duration := time.Since(p.startTime)
	var throughput float64
	if duration.Seconds() > 0 {
		throughput = float64(p.processed) / duration.Seconds() / 1024 / 1024
	}
	
	return ProgressStats{
		Operation:      p.operation,
		BytesProcessed: p.processed,
		Total:          p.total,
		Percentage:     percentage,
		Duration:       duration,
		Throughput:     throughput,
		StartTime:      p.startTime,
	}
}
