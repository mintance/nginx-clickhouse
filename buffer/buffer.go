// Package buffer provides buffering strategies for log lines before they
// are flushed to ClickHouse.
package buffer

import (
	"errors"
	"sync"
)

// ErrBufferFull is returned when the buffer has reached its maximum capacity.
var ErrBufferFull = errors.New("buffer full")

// Buffer stores log lines between flushes. Implementations must be safe
// for concurrent use.
type Buffer interface {
	// Write appends a single log line to the buffer.
	Write(line string) error
	// ReadAll drains the buffer and returns all stored lines.
	ReadAll() ([]string, error)
	// Replay returns lines from a previous session (crash recovery).
	// For memory buffers this returns nil.
	Replay() ([]string, error)
	// Len returns the current number of buffered lines.
	Len() int
}

// MemoryBuffer holds log lines in memory. Lines are lost on crash.
type MemoryBuffer struct {
	mu      sync.Mutex
	lines   []string
	maxSize int
}

// NewMemoryBuffer creates a buffer that holds up to maxSize lines.
func NewMemoryBuffer(maxSize int) *MemoryBuffer {
	return &MemoryBuffer{
		lines:   make([]string, 0, maxSize),
		maxSize: maxSize,
	}
}

// Write appends a single log line to the buffer. It returns ErrBufferFull
// if the buffer has reached its maximum capacity.
func (b *MemoryBuffer) Write(line string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.lines) >= b.maxSize {
		return ErrBufferFull
	}
	b.lines = append(b.lines, line)
	return nil
}

// ReadAll drains the buffer and returns all stored lines.
func (b *MemoryBuffer) ReadAll() ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.lines
	b.lines = nil
	return out, nil
}

// Replay returns lines from a previous session (crash recovery).
// For MemoryBuffer this always returns nil since nothing survives a crash.
func (b *MemoryBuffer) Replay() ([]string, error) {
	return nil, nil
}

// Len returns the current number of buffered lines.
func (b *MemoryBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.lines)
}
