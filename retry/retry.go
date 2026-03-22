// Package retry provides exponential backoff with full jitter.
package retry

import (
	"math/rand/v2"
	"time"
)

// Backoff returns a jittered duration for the given attempt using exponential
// backoff with full jitter. The result is a random value in
// [initial, min(initial*2^attempt, maxDelay)].
// If attempt is negative it is treated as 0.
func Backoff(attempt int, initial, maxDelay time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	delay := initial
	for range attempt {
		delay *= 2
		if delay <= 0 || delay >= maxDelay {
			delay = maxDelay
			break
		}
	}

	if delay <= 0 {
		return initial
	}

	jittered := rand.N(delay)
	if jittered < initial {
		return initial
	}
	return jittered
}

// Do calls fn and retries up to maxRetries times on error, sleeping with
// exponential backoff between attempts. The total number of calls to fn is
// at most maxRetries+1. If all attempts fail, the last error is returned.
func Do(maxRetries int, initial, maxDelay time.Duration, fn func() error) error {
	var err error
	for attempt := range maxRetries + 1 {
		err = fn()
		if err == nil {
			return nil
		}
		if attempt < maxRetries {
			time.Sleep(Backoff(attempt, initial, maxDelay))
		}
	}
	return err
}
