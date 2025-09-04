package streaming

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

// DefaultMemoryMonitor implements the MemoryMonitor interface
type DefaultMemoryMonitor struct {
	mu       sync.RWMutex
	limitMB  int64
	peakMB   int64
	enabled  bool
	interval time.Duration
	
	// Callbacks for memory events
	callbacks []func(usage float64)
}

// NewMemoryMonitor creates a new memory monitor with the specified limit
func NewMemoryMonitor(limitMB int) *DefaultMemoryMonitor {
	return &DefaultMemoryMonitor{
		limitMB:  int64(limitMB),
		enabled:  true,
		interval: time.Second,
	}
}

// CheckMemory returns current memory usage and whether limit is exceeded
func (m *DefaultMemoryMonitor) CheckMemory() (current int64, limit int64, exceeded bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if !m.enabled {
		return 0, m.limitMB, false
	}
	
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	// Convert bytes to MB
	currentMB := int64(memStats.Alloc / 1024 / 1024)
	
	// Update peak usage
	m.mu.RUnlock()
	m.mu.Lock()
	if currentMB > m.peakMB {
		m.peakMB = currentMB
	}
	limit = m.limitMB
	m.mu.Unlock()
	m.mu.RLock()
	
	exceeded = currentMB > m.limitMB
	
	// Trigger callbacks if usage is high
	if len(m.callbacks) > 0 {
		usage := float64(currentMB) / float64(m.limitMB)
		for _, callback := range m.callbacks {
			go callback(usage)
		}
	}
	
	return currentMB, limit, exceeded
}

// SetLimit updates the memory limit
func (m *DefaultMemoryMonitor) SetLimit(limitMB int) error {
	if limitMB <= 0 {
		return fmt.Errorf("memory limit must be positive, got %d", limitMB)
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.limitMB = int64(limitMB)
	return nil
}

// GetStats returns memory usage statistics
func (m *DefaultMemoryMonitor) GetStats() MemoryStats {
	current, limit, _ := m.CheckMemory()
	
	m.mu.RLock()
	peak := m.peakMB
	m.mu.RUnlock()
	
	usage := float64(current) / float64(limit)
	if limit == 0 {
		usage = 0
	}
	
	return MemoryStats{
		CurrentMB: current,
		LimitMB:   limit,
		PeakMB:    peak,
		Usage:     usage,
	}
}

// Enable enables or disables memory monitoring
func (m *DefaultMemoryMonitor) Enable(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = enabled
}

// AddCallback adds a callback function that will be called when memory usage changes
func (m *DefaultMemoryMonitor) AddCallback(callback func(usage float64)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks = append(m.callbacks, callback)
}

// Reset resets the peak memory usage
func (m *DefaultMemoryMonitor) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.peakMB = 0
}

// BackpressureController manages backpressure based on memory usage
type BackpressureController struct {
	monitor         MemoryMonitor
	pauseThreshold  float64 // Pause processing at this usage level
	resumeThreshold float64 // Resume processing at this usage level
	paused          bool
	mu              sync.RWMutex
}

// NewBackpressureController creates a new backpressure controller
func NewBackpressureController(monitor MemoryMonitor) *BackpressureController {
	return &BackpressureController{
		monitor:         monitor,
		pauseThreshold:  0.8, // Pause at 80% memory usage
		resumeThreshold: 0.6, // Resume at 60% memory usage
	}
}

// ShouldPause returns true if processing should be paused due to memory pressure
func (b *BackpressureController) ShouldPause() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	if b.paused {
		// Check if we can resume
		stats := b.monitor.GetStats()
		if stats.Usage <= b.resumeThreshold {
			b.mu.RUnlock()
			b.mu.Lock()
			b.paused = false
			b.mu.Unlock()
			b.mu.RLock()
			return false
		}
		return true
	}
	
	// Check if we should pause
	stats := b.monitor.GetStats()
	if stats.Usage >= b.pauseThreshold {
		b.mu.RUnlock()
		b.mu.Lock()
		b.paused = true
		b.mu.Unlock()
		b.mu.RLock()
		return true
	}
	
	return false
}

// WaitForResume blocks until memory usage drops below the resume threshold
func (b *BackpressureController) WaitForResume() {
	for b.ShouldPause() {
		time.Sleep(100 * time.Millisecond)
		runtime.GC() // Suggest garbage collection
	}
}

// SetThresholds updates the pause and resume thresholds
func (b *BackpressureController) SetThresholds(pause, resume float64) error {
	if pause <= resume {
		return fmt.Errorf("pause threshold (%f) must be greater than resume threshold (%f)", pause, resume)
	}
	if pause < 0 || pause > 1 || resume < 0 || resume > 1 {
		return fmt.Errorf("thresholds must be between 0 and 1")
	}
	
	b.mu.Lock()
	defer b.mu.Unlock()
	
	b.pauseThreshold = pause
	b.resumeThreshold = resume
	return nil
}

// GetStatus returns the current backpressure status
func (b *BackpressureController) GetStatus() (paused bool, usage float64) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	stats := b.monitor.GetStats()
	return b.paused, stats.Usage
}
