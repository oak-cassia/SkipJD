package retry

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"
)

// Do executes the given function up to 'attempts' times.
// It uses exponential backoff with full jitter: delay = base * 2^(n-1).
// The delay is capped at 30 seconds.
// If the context is canceled, it returns immediately.
// If the retryable function returns false for an error, it stops retrying and returns.
func Do(ctx context.Context, attempts int, base time.Duration,
	fn func(context.Context) error,
	retryable func(error) bool) error {

	if attempts < 1 {
		attempts = 1
	}

	var err error
	for i := 1; i <= attempts; i++ {
		err = fn(ctx)
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("context done during attempt %d: %w (last error: %v)", i, ctx.Err(), err)
		default:
		}

		if i == attempts {
			break
		}

		if !retryable(err) {
			return err
		}

		log.Printf("retry attempt=%d/%d err=%v", i, attempts, err)

		delay := base * time.Duration(1<<(i-1))
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}

		// Full jitter: Random delay between 0 and delay
		jitter := time.Duration(rand.Int63n(int64(delay) + 1))

		select {
		case <-time.After(jitter):
		case <-ctx.Done():
			return fmt.Errorf("context done while waiting for retry attempt %d: %w (last error: %v)", i+1, ctx.Err(), err)
		}
	}

	return fmt.Errorf("exhausted %d attempts: %w", attempts, err)
}
