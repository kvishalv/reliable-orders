package reliability

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// RetryConfig holds retry policy configuration
type RetryConfig struct {
	MaxAttempts     int
	InitialBackoff  time.Duration
	MaxBackoff      time.Duration
	BackoffMultiple float64
	JitterFraction  float64
}

// DefaultRetryConfig returns sensible defaults for payment service retries
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:     3, // Initial attempt + 2 retries
		InitialBackoff:  50 * time.Millisecond,
		MaxBackoff:      1 * time.Second,
		BackoffMultiple: 2.0,
		JitterFraction:  0.3, // ±30% jitter to avoid thundering herd
	}
}

// RetryableHTTPCall executes an HTTP call with exponential backoff and jitter
// Retries on transient failures: 5xx, 429, and network errors
// Does NOT retry on 4xx client errors (except 429) as they indicate bad requests
func RetryableHTTPCall(ctx context.Context, span trace.Span, cfg RetryConfig, fn func(context.Context) (*http.Response, error)) (*http.Response, error) {
	var lastErr error
	var resp *http.Response

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		// Add attempt number to span for debugging
		span.SetAttributes(attribute.Int("retry.attempt", attempt))

		// Execute the function
		resp, lastErr = fn(ctx)

		// Success case
		if lastErr == nil && resp != nil && resp.StatusCode < 500 && resp.StatusCode != 429 {
			if attempt > 0 {
				span.SetAttributes(attribute.Bool("retry.succeeded", true))
			}
			return resp, nil
		}

		// Record retry reason
		if lastErr != nil {
			span.AddEvent("retry_due_to_error", trace.WithAttributes(
				attribute.String("error", lastErr.Error()),
			))
		} else if resp != nil {
			span.AddEvent("retry_due_to_status", trace.WithAttributes(
				attribute.Int("status_code", resp.StatusCode),
			))
			if resp.Body != nil {
				resp.Body.Close() // Close before retry
			}
		}

		// Don't sleep after last attempt
		if attempt < cfg.MaxAttempts-1 {
			backoff := calculateBackoff(cfg, attempt)
			span.SetAttributes(attribute.Int("retry.backoff_ms", int(backoff.Milliseconds())))

			select {
			case <-time.After(backoff):
				// Continue to next attempt
			case <-ctx.Done():
				span.SetStatus(codes.Error, "context cancelled during retry backoff")
				return nil, fmt.Errorf("retry cancelled: %w", ctx.Err())
			}
		}
	}

	// All retries exhausted
	span.SetAttributes(attribute.Bool("retry.exhausted", true))
	span.SetStatus(codes.Error, "all retry attempts failed")

	if lastErr != nil {
		return nil, fmt.Errorf("retry exhausted after %d attempts: %w", cfg.MaxAttempts, lastErr)
	}
	return resp, nil
}

// calculateBackoff computes exponential backoff with jitter
// Jitter prevents synchronized retries from multiple clients (thundering herd problem)
func calculateBackoff(cfg RetryConfig, attempt int) time.Duration {
	// Exponential backoff: initialBackoff * (multiple ^ attempt)
	backoff := float64(cfg.InitialBackoff) * math.Pow(cfg.BackoffMultiple, float64(attempt))

	// Cap at max backoff
	if backoff > float64(cfg.MaxBackoff) {
		backoff = float64(cfg.MaxBackoff)
	}

	// Add jitter: ±jitterFraction of backoff
	jitterRange := backoff * cfg.JitterFraction
	jitter := (rand.Float64() * 2 * jitterRange) - jitterRange
	backoff += jitter

	// Ensure non-negative
	if backoff < 0 {
		backoff = 0
	}

	return time.Duration(backoff)
}
