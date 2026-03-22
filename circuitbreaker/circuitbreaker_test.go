package circuitbreaker

import (
	"testing"
	"time"
)

func TestCircuitBreakerClosed(t *testing.T) {
	cb := New(3, 10*time.Millisecond)

	for i := 0; i < 5; i++ {
		if !cb.Allow() {
			t.Fatalf("expected Allow() = true for a new breaker, got false on call %d", i)
		}
	}
}

func TestCircuitBreakerOpens(t *testing.T) {
	cb := New(3, 10*time.Millisecond)

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.Allow() {
		t.Fatal("expected Allow() = false after reaching failure threshold")
	}
}

func TestCircuitBreakerHalfOpen(t *testing.T) {
	cb := New(3, 10*time.Millisecond)

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.Allow() {
		t.Fatal("expected Allow() = false immediately after opening")
	}

	time.Sleep(15 * time.Millisecond)

	if !cb.Allow() {
		t.Fatal("expected Allow() = true after cooldown (half-open probe)")
	}

	if cb.State() != StateHalfOpen {
		t.Fatalf("expected state %q, got %q", StateHalfOpen, cb.State())
	}
}

func TestCircuitBreakerResets(t *testing.T) {
	cb := New(3, 10*time.Millisecond)

	// Trip the breaker.
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != StateOpen {
		t.Fatalf("expected state %q, got %q", StateOpen, cb.State())
	}

	// Wait for cooldown and allow probe.
	time.Sleep(15 * time.Millisecond)
	cb.Allow()

	// Simulate successful probe.
	cb.RecordSuccess()

	if cb.State() != StateClosed {
		t.Fatalf("expected state %q after RecordSuccess, got %q", StateClosed, cb.State())
	}

	if !cb.Allow() {
		t.Fatal("expected Allow() = true after reset")
	}
}

func TestCircuitBreakerState(t *testing.T) {
	cb := New(2, 10*time.Millisecond)

	// Initially closed.
	if s := cb.State(); s != StateClosed {
		t.Fatalf("expected state %q, got %q", StateClosed, s)
	}

	// Open after threshold failures.
	cb.RecordFailure()
	cb.RecordFailure()

	if s := cb.State(); s != StateOpen {
		t.Fatalf("expected state %q, got %q", StateOpen, s)
	}

	// Half-open after cooldown.
	time.Sleep(15 * time.Millisecond)
	cb.Allow()

	if s := cb.State(); s != StateHalfOpen {
		t.Fatalf("expected state %q, got %q", StateHalfOpen, s)
	}

	// Back to closed after success.
	cb.RecordSuccess()

	if s := cb.State(); s != StateClosed {
		t.Fatalf("expected state %q, got %q", StateClosed, s)
	}
}
