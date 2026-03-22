package retry

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackoffExponential(t *testing.T) {
	const iterations = 200
	initial := 10 * time.Millisecond
	maxDelay := 10 * time.Second

	var maxAttempt0, maxAttempt3 time.Duration
	for range iterations {
		d0 := Backoff(0, initial, maxDelay)
		d3 := Backoff(3, initial, maxDelay)
		if d0 > maxAttempt0 {
			maxAttempt0 = d0
		}
		if d3 > maxAttempt3 {
			maxAttempt3 = d3
		}
	}

	if maxAttempt3 <= maxAttempt0 {
		t.Errorf("expected max backoff at attempt 3 (%v) > attempt 0 (%v)", maxAttempt3, maxAttempt0)
	}
}

func TestBackoffCap(t *testing.T) {
	initial := 10 * time.Millisecond
	maxDelay := 50 * time.Millisecond

	for range 500 {
		d := Backoff(20, initial, maxDelay)
		if d > maxDelay {
			t.Fatalf("backoff %v exceeds maxDelay %v", d, maxDelay)
		}
	}
}

func TestBackoffNegativeAttempt(t *testing.T) {
	initial := 10 * time.Millisecond
	maxDelay := time.Second

	for range 100 {
		d := Backoff(-1, initial, maxDelay)
		if d > initial {
			t.Fatalf("backoff %v for negative attempt exceeds initial %v", d, initial)
		}
	}
}

func TestDoSuccess(t *testing.T) {
	var calls int
	err := Do(3, time.Millisecond, 5*time.Millisecond, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestDoRetryThenSuccess(t *testing.T) {
	var calls atomic.Int32
	err := Do(5, time.Millisecond, 5*time.Millisecond, func() error {
		n := calls.Add(1)
		if n <= 2 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if c := calls.Load(); c != 3 {
		t.Fatalf("expected 3 calls, got %d", c)
	}
}

func TestDoAllFail(t *testing.T) {
	const maxRetries = 3
	var calls atomic.Int32
	sentinel := errors.New("permanent")

	err := Do(maxRetries, time.Millisecond, 5*time.Millisecond, func() error {
		calls.Add(1)
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if c := calls.Load(); c != maxRetries+1 {
		t.Fatalf("expected %d calls, got %d", maxRetries+1, c)
	}
}

func TestDoZeroRetries(t *testing.T) {
	var calls int
	sentinel := errors.New("fail")

	err := Do(0, time.Millisecond, 5*time.Millisecond, func() error {
		calls++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}
