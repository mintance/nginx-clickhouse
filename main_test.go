package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanLinesFromReader(t *testing.T) {
	input := "line1\nline2\nline3\n"
	ch := scanLines(strings.NewReader(input))

	var got []string
	for line := range ch {
		got = append(got, line)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(got))
	}
	if got[0] != "line1" || got[1] != "line2" || got[2] != "line3" {
		t.Errorf("unexpected lines: %v", got)
	}
}

func TestScanLinesEmpty(t *testing.T) {
	ch := scanLines(strings.NewReader(""))

	var got []string
	for line := range ch {
		got = append(got, line)
	}

	if len(got) != 0 {
		t.Errorf("expected 0 lines, got %d", len(got))
	}
}

func TestScanLinesClosesFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.log")
	if err := os.WriteFile(tmpFile, []byte("hello\nworld\n"), 0644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	ch := scanLines(f)
	var got []string
	for line := range ch {
		got = append(got, line)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(got))
	}

	// File should be closed after channel is drained.
	_, err = f.Read(make([]byte, 1))
	if err == nil {
		t.Error("expected error reading from closed file, got nil")
	}
}

func TestScanLinesFromPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	ch := scanLines(r)

	go func() {
		io.WriteString(w, "from-pipe-1\nfrom-pipe-2\n")
		w.Close()
	}()

	var got []string
	for line := range ch {
		got = append(got, line)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(got))
	}
	if got[0] != "from-pipe-1" || got[1] != "from-pipe-2" {
		t.Errorf("unexpected lines: %v", got)
	}
}

func TestScanLinesNoTrailingNewline(t *testing.T) {
	input := "line1\nline2"
	ch := scanLines(strings.NewReader(input))

	var got []string
	for line := range ch {
		got = append(got, line)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(got))
	}
	if got[1] != "line2" {
		t.Errorf("expected last line 'line2', got %q", got[1])
	}
}
