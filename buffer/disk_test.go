package buffer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiskBufferWriteRead(t *testing.T) {
	dir := t.TempDir()
	buf, err := NewDiskBuffer(dir, 1<<20)
	if err != nil {
		t.Fatalf("NewDiskBuffer: %v", err)
	}
	defer buf.Close()

	want := []string{"line1", "line2", "line3", "line4", "line5"}
	for _, line := range want {
		if err := buf.Write(line); err != nil {
			t.Fatalf("Write(%q): %v", line, err)
		}
	}

	got, err := buf.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("ReadAll returned %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ReadAll()[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	// Buffer should be empty after ReadAll.
	if buf.Len() != 0 {
		t.Errorf("Len() after ReadAll = %d, want 0", buf.Len())
	}
	got2, err := buf.ReadAll()
	if err != nil {
		t.Fatalf("second ReadAll: %v", err)
	}
	if got2 != nil {
		t.Errorf("second ReadAll = %v, want nil", got2)
	}
}

func TestDiskBufferReplay(t *testing.T) {
	dir := t.TempDir()

	// Simulate first session: write lines and close without reading.
	buf1, err := NewDiskBuffer(dir, 1<<20)
	if err != nil {
		t.Fatalf("NewDiskBuffer (session 1): %v", err)
	}
	want := []string{"alpha", "bravo", "charlie"}
	for _, line := range want {
		if err := buf1.Write(line); err != nil {
			t.Fatalf("Write(%q): %v", line, err)
		}
	}
	buf1.Close()

	// Simulate second session: create new buffer pointing at same dir.
	buf2, err := NewDiskBuffer(dir, 1<<20)
	if err != nil {
		t.Fatalf("NewDiskBuffer (session 2): %v", err)
	}
	defer buf2.Close()

	got, err := buf2.Replay()
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("Replay returned %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Replay()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDiskBufferRotation(t *testing.T) {
	dir := t.TempDir()

	// Use a small maxBytes but large enough to hold all data. The key is
	// to trigger rotation by exceeding maxSegmentBytes per segment. We
	// temporarily work around the 10 MB constant by writing many lines and
	// checking that segment files are created. Instead, we directly test
	// the segment creation logic by writing data that triggers rotation
	// using a helper that overrides the segment size check.
	//
	// For a unit test we create a buffer with plenty of room and manually
	// verify multiple segments get created by writing enough bytes.
	// We'll write lines that are 1 MB each to force rotation.
	buf, err := NewDiskBuffer(dir, 50*1024*1024) // 50 MB limit
	if err != nil {
		t.Fatalf("NewDiskBuffer: %v", err)
	}
	defer buf.Close()

	// Each line is ~1 MB, so after ~10 lines we should have rotated at
	// least once (maxSegmentBytes = 10 MB).
	bigLine := strings.Repeat("x", 1024*1024) // 1 MB
	for i := 0; i < 15; i++ {
		if err := buf.Write(bigLine); err != nil {
			t.Fatalf("Write #%d: %v", i, err)
		}
	}

	// Close current segment so all files are flushed.
	buf.Close()

	// Count segment files.
	matches, err := filepath.Glob(filepath.Join(dir, segmentPrefix+"*"+segmentSuffix))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(matches) < 2 {
		t.Errorf("expected at least 2 segment files after rotation, got %d", len(matches))
	}
}

func TestDiskBufferEmpty(t *testing.T) {
	dir := t.TempDir()
	buf, err := NewDiskBuffer(dir, 1<<20)
	if err != nil {
		t.Fatalf("NewDiskBuffer: %v", err)
	}
	defer buf.Close()

	got, err := buf.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if got != nil {
		t.Errorf("ReadAll on empty buffer = %v, want nil", got)
	}
}

func TestDiskBufferMaxBytes(t *testing.T) {
	dir := t.TempDir()
	// Allow only 100 bytes total.
	buf, err := NewDiskBuffer(dir, 100)
	if err != nil {
		t.Fatalf("NewDiskBuffer: %v", err)
	}
	defer buf.Close()

	var gotFull bool
	for i := 0; i < 200; i++ {
		err := buf.Write("this is a moderately long line that eats space quickly")
		if errors.Is(err, ErrBufferFull) {
			gotFull = true
			break
		}
		if err != nil {
			t.Fatalf("Write #%d: unexpected error: %v", i, err)
		}
	}
	if !gotFull {
		t.Error("expected ErrBufferFull but never received it")
	}

	// Verify no segment files exceed the limit by much.
	var totalSize int64
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		info, _ := e.Info()
		if info != nil {
			totalSize += info.Size()
		}
	}
	if totalSize > 100 {
		// The buffer should not have written beyond maxBytes.
		t.Errorf("total disk usage = %d, exceeds maxBytes = 100", totalSize)
	}
}
