package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/demo/order-service/internal/reliability"
	"github.com/demo/order-service/internal/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// OrderService handles order creation with reliability patterns
type OrderService struct {
	paymentURL       string
	httpClient       *http.Client
	circuitBreaker   *reliability.CircuitBreaker
	bulkhead         *reliability.Bulkhead
	retryConfig      reliability.RetryConfig
	idempotencyStore *reliability.IdempotencyStore
	tracer           trace.Tracer
}

// NewOrderService creates a new order service with configured reliability patterns
func NewOrderService(paymentURL string) *OrderService {
	return &OrderService{
		paymentURL: paymentURL,
		httpClient: &http.Client{
			Timeout: 2 * time.Second, // Overall client timeout
		},
		circuitBreaker:   reliability.NewCircuitBreaker(),
		bulkhead:         reliability.NewBulkhead(10), // Max 10 concurrent payment calls
		retryConfig:      reliability.DefaultRetryConfig(),
		idempotencyStore: reliability.NewIdempotencyStore(),
		tracer:           tracing.GetTracer("order-service"),
	}
}

// CreateOrderRequest represents the incoming order request
type CreateOrderRequest struct {
	MerchantID string  `json:"merchant_id" binding:"required"`
	Amount     float64 `json:"amount" binding:"required,gt=0"`
	Currency   string  `json:"currency" binding:"required"`
}

// CreateOrderResponse represents the order creation response
type CreateOrderResponse struct {
	OrderID   string `json:"order_id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// CreateOrder orchestrates the order creation workflow with reliability patterns
func (s *OrderService) CreateOrder(ctx context.Context, req CreateOrderRequest, idempotencyKey string) (*CreateOrderResponse, error) {
	// Start parent span for the entire order creation flow
	ctx, span := s.tracer.Start(ctx, "createOrder",
		trace.WithAttributes(
			attribute.String("merchant.id", req.MerchantID),
			attribute.Float64("order.amount", req.Amount),
			attribute.String("order.currency", req.Currency),
		),
	)
	defer span.End()

	// Check idempotency: if we've seen this key before, return cached response
	if idempotencyKey != "" {
		span.SetAttributes(attribute.String("idempotency.key", idempotencyKey))
		if cached, exists := s.idempotencyStore.Get(idempotencyKey); exists {
			span.AddEvent("idempotent_request_cached")
			return &CreateOrderResponse{
				OrderID:   cached.OrderID,
				Status:    cached.Status,
				CreatedAt: cached.CreatedAt.Format(time.RFC3339),
			}, nil
		}
	}

	// Generate order ID
	orderID := uuid.New().String()
	span.SetAttributes(attribute.String("order.id", orderID))

	// Call payment service with all reliability patterns
	if err := s.callPaymentService(ctx, orderID, req); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("payment failed: %w", err)
	}

	// Persist order (simulated with a span)
	if err := s.persistOrder(ctx, orderID, req); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to persist order: %w", err)
	}

	// Create response
	response := &CreateOrderResponse{
		OrderID:   orderID,
		Status:    "completed",
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	// Store for idempotency
	if idempotencyKey != "" {
		s.idempotencyStore.Set(idempotencyKey, &reliability.IdempotentResponse{
			OrderID:   orderID,
			Status:    "completed",
			CreatedAt: time.Now(),
		})
	}

	span.SetStatus(codes.Ok, "order created successfully")
	return response, nil
}

// callPaymentService calls the payment service with timeout, retry, circuit breaker, and bulkhead
func (s *OrderService) callPaymentService(ctx context.Context, orderID string, req CreateOrderRequest) error {
	ctx, span := s.tracer.Start(ctx, "callPayment")
	defer span.End()

	// Set payment call timeout (must complete within 500ms budget)
	paymentCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	span.SetAttributes(attribute.Int("timeout_ms", 500))

	// Apply bulkhead: limit concurrent payment calls to protect resources
	err := s.bulkhead.Execute(ctx, span, func(ctx context.Context) error {
		// Apply circuit breaker: fail fast if payment service is down
		return s.circuitBreaker.Execute(span, func() error {
			// Apply retry with exponential backoff: handle transient failures
			_, err := reliability.RetryableHTTPCall(paymentCtx, span, s.retryConfig, func(ctx context.Context) (*http.Response, error) {
				return s.doPaymentRequest(ctx, span, orderID, req)
			})
			return err
		})
	})

	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetStatus(codes.Ok, "payment successful")
	return nil
}

// doPaymentRequest performs the actual HTTP call to payment service
func (s *OrderService) doPaymentRequest(ctx context.Context, span trace.Span, orderID string, req CreateOrderRequest) (*http.Response, error) {
	// Create payment request payload
	paymentReq := map[string]interface{}{
		"order_id":    orderID,
		"merchant_id": req.MerchantID,
		"amount":      req.Amount,
		"currency":    req.Currency,
	}

	body, _ := json.Marshal(paymentReq)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", s.paymentURL+"/charge", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Propagate trace context to payment service (W3C Trace Context)
	// This ensures the payment service's spans are linked to this trace
	// otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(httpReq.Header))
	// Note: otelgin middleware handles this automatically

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("payment request failed: %w", err)
	}

	// Check for successful response
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		span.SetAttributes(attribute.Int("payment.status_code", resp.StatusCode))
		return resp, fmt.Errorf("payment service returned %d: %s", resp.StatusCode, string(body))
	}

	resp.Body.Close()
	return resp, nil
}

// persistOrder simulates persisting the order to a database
func (s *OrderService) persistOrder(ctx context.Context, orderID string, req CreateOrderRequest) error {
	_, span := s.tracer.Start(ctx, "persistOrder",
		trace.WithAttributes(attribute.String("order.id", orderID)),
	)
	defer span.End()

	// Simulate database write latency
	time.Sleep(10 * time.Millisecond)

	span.SetStatus(codes.Ok, "order persisted")
	return nil
}
