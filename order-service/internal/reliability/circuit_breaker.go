package reliability

import (
	"errors"
	"fmt"
	"time"

	"github.com/sony/gobreaker"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CircuitBreaker wraps gobreaker to protect against cascading failures
// When the payment service is consistently failing, the circuit opens to prevent
// wasting resources on requests that will likely fail, giving the downstream service time to recover
type CircuitBreaker struct {
	cb *gobreaker.CircuitBreaker
}

// NewCircuitBreaker creates a circuit breaker with sensible defaults for payment calls
func NewCircuitBreaker() *CircuitBreaker {
	settings := gobreaker.Settings{
		Name:        "payment-service",
		MaxRequests: 3,                // Allow 3 requests in half-open state to test recovery
		Interval:    10 * time.Second, // Rolling window for failure counting
		Timeout:     30 * time.Second, // Time to wait before attempting to close circuit
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Open circuit after 5 consecutive failures or 60% failure rate with at least 10 requests
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.ConsecutiveFailures >= 5 || (counts.Requests >= 10 && failureRatio >= 0.6)
		},
	}

	return &CircuitBreaker{
		cb: gobreaker.NewCircuitBreaker(settings),
	}
}

// Execute runs the function through the circuit breaker
// Records circuit breaker state in the active span for observability
func (c *CircuitBreaker) Execute(span trace.Span, fn func() error) error {
	state := c.cb.State()
	span.SetAttributes(attribute.String("cb.state", state.String()))

	_, err := c.cb.Execute(func() (interface{}, error) {
		return nil, fn()
	})

	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) {
			span.SetAttributes(attribute.Bool("cb.open", true))
			return fmt.Errorf("circuit breaker open: %w", err)
		}
		return err
	}

	return nil
}

// State returns the current circuit breaker state
func (c *CircuitBreaker) State() gobreaker.State {
	return c.cb.State()
}
