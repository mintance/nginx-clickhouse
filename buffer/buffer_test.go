package buffer

import (
	"errors"
	"sync"
	"testing"
)

func TestMemoryBufferWrite(t *testing.T) {
	buf := NewMemoryBuffer(10)
	for _, line := range []string{"a", "b", "c"} {
		if err := buf.Write(line); err != nil {
			t.Fatalf("Write(%q): unexpected error: %v", line, err)
		}
	}
	if got := buf.Len(); got != 3 {
		t.Errorf("Len() = %d, want 3", got)
	}
}

func TestMemoryBufferReadAll(t *testing.T) {
	buf := NewMemoryBuffer(10)
	want := []string{"x", "y", "z"}
	for _, line := range want {
		if err := buf.Write(line); err != nil {
			t.Fatalf("Write(%q): unexpected error: %v", line, err)
		}
	}

	got, err := buf.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll(): unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("ReadAll() returned %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ReadAll()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if buf.Len() != 0 {
		t.Errorf("Len() after ReadAll = %d, want 0", buf.Len())
	}
}

func TestMemoryBufferFull(t *testing.T) {
	buf := NewMemoryBuffer(2)
	if err := buf.Write("a"); err != nil {
		t.Fatalf("Write(a): unexpected error: %v", err)
	}
	if err := buf.Write("b"); err != nil {
		t.Fatalf("Write(b): unexpected error: %v", err)
	}
	err := buf.Write("c")
	if !errors.Is(err, ErrBufferFull) {
		t.Errorf("Write(c) = %v, want ErrBufferFull", err)
	}
}

func TestMemoryBufferReplay(t *testing.T) {
	buf := NewMemoryBuffer(10)
	lines, err := buf.Replay()
	if err != nil {
		t.Fatalf("Replay(): unexpected error: %v", err)
	}
	if lines != nil {
		t.Errorf("Replay() = %v, want nil", lines)
	}
}

func TestMemoryBufferConcurrent(t *testing.T) {
	const n = 100
	buf := NewMemoryBuffer(n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			_ = buf.Write("line")
		}(i)
	}
	wg.Wait()

	if got := buf.Len(); got != n {
		t.Errorf("Len() = %d, want %d", got, n)
	}
}
