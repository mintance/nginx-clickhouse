package buffer

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const maxSegmentBytes = 10 * 1024 * 1024 // 10 MB per segment
const segmentPrefix = "segment-"
const segmentSuffix = ".log"

// DiskBuffer stores log lines as append-only segment files on disk,
// providing crash recovery. Lines survive process restarts.
type DiskBuffer struct {
	mu             sync.Mutex
	dir            string
	maxBytes       int64
	currentFile    *os.File
	currentBytes   int64
	segmentCounter int
	lineCount      int
}

// NewDiskBuffer creates a disk-backed buffer that writes segment files under
// dir. It enforces an approximate total size limit of maxBytes across all
// segments. Existing segments in dir are counted so that Replay can recover
// them after a crash.
func NewDiskBuffer(dir string, maxBytes int64) (*DiskBuffer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating buffer directory: %w", err)
	}

	b := &DiskBuffer{
		dir:      dir,
		maxBytes: maxBytes,
	}

	segments, err := b.listSegments()
	if err != nil {
		return nil, fmt.Errorf("listing existing segments: %w", err)
	}

	// Estimate line count and current bytes from existing segments.
	for _, seg := range segments {
		info, err := os.Stat(seg)
		if err != nil {
			continue
		}
		b.currentBytes += info.Size()

		// Count lines for the line count estimate.
		f, err := os.Open(seg)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			b.lineCount++
		}
		f.Close()
	}

	// Derive segment counter from the highest existing segment number to
	// avoid filename collisions after a crash where segments were partially
	// cleaned up.
	if len(segments) > 0 {
		last := filepath.Base(segments[len(segments)-1])
		numStr := strings.TrimPrefix(last, segmentPrefix)
		numStr = strings.TrimSuffix(numStr, segmentSuffix)
		if n, err := strconv.Atoi(numStr); err == nil {
			b.segmentCounter = n + 1
		} else {
			b.segmentCounter = len(segments)
		}
	}

	return b, nil
}

// Write appends a single log line to the buffer. It returns ErrBufferFull if
// the total disk usage would exceed the configured maximum.
func (b *DiskBuffer) Write(line string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	data := []byte(line + "\n")

	if b.maxBytes > 0 && b.currentBytes+int64(len(data)) > b.maxBytes {
		return ErrBufferFull
	}

	// Open a new segment if we don't have one or the current one is full.
	if b.currentFile == nil || b.currentBytes > 0 && b.needsRotation(int64(len(data))) {
		if err := b.openSegment(); err != nil {
			return fmt.Errorf("opening segment: %w", err)
		}
	}

	n, err := b.currentFile.Write(data)
	if err != nil {
		return fmt.Errorf("writing to segment: %w", err)
	}
	b.currentBytes += int64(n)
	b.lineCount++
	return nil
}

// ReadAll drains the buffer and returns all stored lines. Segment files are
// deleted after a successful read.
func (b *DiskBuffer) ReadAll() ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Close the current segment so it can be read.
	if b.currentFile != nil {
		b.currentFile.Close()
		b.currentFile = nil
	}

	segments, err := b.listSegments()
	if err != nil {
		return nil, fmt.Errorf("listing segments: %w", err)
	}
	if len(segments) == 0 {
		return nil, nil
	}

	var lines []string
	for _, seg := range segments {
		f, err := os.Open(seg)
		if err != nil {
			return nil, fmt.Errorf("opening segment %s: %w", seg, err)
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			lines = append(lines, sc.Text())
		}
		if err := sc.Err(); err != nil {
			f.Close()
			return nil, fmt.Errorf("reading segment %s: %w", seg, err)
		}
		f.Close()
	}

	// Delete all segments after successful read.
	for _, seg := range segments {
		os.Remove(seg)
	}

	b.currentBytes = 0
	b.lineCount = 0
	b.segmentCounter = 0

	return lines, nil
}

// Replay returns lines from a previous session for crash recovery. It reads
// and deletes all existing segments, identical to ReadAll.
func (b *DiskBuffer) Replay() ([]string, error) {
	return b.ReadAll()
}

// Len returns the approximate number of buffered lines.
func (b *DiskBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lineCount
}

// Close closes the current segment file, if any.
func (b *DiskBuffer) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.currentFile != nil {
		err := b.currentFile.Close()
		b.currentFile = nil
		return err
	}
	return nil
}

// segmentName returns a zero-padded segment file name for the given counter.
func segmentName(n int) string {
	return fmt.Sprintf("%s%06d%s", segmentPrefix, n, segmentSuffix)
}

// openSegment creates a new segment file and sets it as the current file.
// The caller must hold b.mu.
func (b *DiskBuffer) openSegment() error {
	if b.currentFile != nil {
		b.currentFile.Close()
		b.currentFile = nil
	}

	name := filepath.Join(b.dir, segmentName(b.segmentCounter))
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	b.currentFile = f
	b.segmentCounter++
	return nil
}

// needsRotation reports whether writing n more bytes to the current segment
// would exceed the per-segment size limit. The caller must hold b.mu.
func (b *DiskBuffer) needsRotation(n int64) bool {
	if b.currentFile == nil {
		return true
	}
	info, err := b.currentFile.Stat()
	if err != nil {
		return true
	}
	return info.Size()+n > maxSegmentBytes
}

// listSegments returns the sorted paths of all segment files in the buffer
// directory.
func (b *DiskBuffer) listSegments() ([]string, error) {
	pattern := filepath.Join(b.dir, segmentPrefix+"*"+segmentSuffix)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	// Filter to only files that match the expected naming pattern.
	var segments []string
	for _, m := range matches {
		base := filepath.Base(m)
		if strings.HasPrefix(base, segmentPrefix) && strings.HasSuffix(base, segmentSuffix) {
			segments = append(segments, m)
		}
	}
	return segments, nil
}
