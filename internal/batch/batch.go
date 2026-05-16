// Package batch provides a small generic worker-pool helper shared by the
// ocr and extraction workers. The two callers track different success
// counters (success/empty vs. success/failed), so this helper only handles
// the bounded-concurrency dispatch — counter aggregation stays at the
// caller.
package batch

import (
	"context"

	"golang.org/x/sync/errgroup"
)

// Run dispatches fn over items with at most workers concurrent goroutines.
// fn never returns an error to the group; failure aggregation is the
// caller's job. If workers <= 0, defaults to 3.
func Run[T any](ctx context.Context, items []T, workers int, fn func(ctx context.Context, item T)) {
	if workers <= 0 {
		workers = 3
	}
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(workers)
	for _, it := range items {
		item := it
		eg.Go(func() error {
			fn(egCtx, item)
			return nil
		})
	}
	_ = eg.Wait()
}
