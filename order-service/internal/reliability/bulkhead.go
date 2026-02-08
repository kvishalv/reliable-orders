package reliability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/semaphore"
)

// Bulkhead limits concurrent requests to prevent resource exhaustion
// If payment service is slow, this prevents all goroutines from being blocked
// on payment calls, keeping the service responsive for other operations
type Bulkhead struct {
	sem *semaphore.Weighted
	max int64
}

// NewBulkhead creates a bulkhead with max concurrent operations
func NewBulkhead(maxConcurrent int64) *Bulkhead {
	return &Bulkhead{
		sem: semaphore.NewWeighted(maxConcurrent),
		max: maxConcurrent,
	}
}

// Execute runs the function within the bulkhead's concurrency limit
// If the limit is reached, it blocks until a slot becomes available or context expires
func (b *Bulkhead) Execute(ctx context.Context, span trace.Span, fn func(context.Context) error) error {
	// Try to acquire a semaphore slot
	if err := b.sem.Acquire(ctx, 1); err != nil {
		span.SetStatus(codes.Error, "bulkhead acquire failed")
		span.SetAttributes(attribute.Bool("bulkhead.rejected", true))
		return fmt.Errorf("bulkhead limit reached: %w", err)
	}
	defer b.sem.Release(1)

	// Record bulkhead usage for capacity planning
	// In production, you'd export this as a gauge metric
	span.SetAttributes(attribute.Int64("bulkhead.max", b.max))

	return fn(ctx)
}
