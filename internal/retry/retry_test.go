package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDo_SuccessFirstTry(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	err := Do(ctx, 3, time.Millisecond, func(ctx context.Context) error {
		attempts++
		return nil
	}, func(err error) bool { return true })

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

func TestDo_SuccessThirdTry(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	err := Do(ctx, 5, time.Millisecond, func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temp error")
		}
		return nil
	}, func(err error) bool { return true })

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestDo_Exhausted(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	expectedErr := errors.New("always fails")
	err := Do(ctx, 3, time.Millisecond, func(ctx context.Context) error {
		attempts++
		return expectedErr
	}, func(err error) bool { return true })

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestDo_NonRetryable(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	expectedErr := errors.New("fatal error")
	err := Do(ctx, 5, time.Millisecond, func(ctx context.Context) error {
		attempts++
		return expectedErr
	}, func(err error) bool { return false })

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

func TestDo_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Canceled immediately

	attempts := 0
	err := Do(ctx, 3, time.Millisecond, func(ctx context.Context) error {
		attempts++
		return errors.New("temp err")
	}, func(err error) bool { return true })

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled error, got %v", err)
	}
}
