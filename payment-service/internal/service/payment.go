package service

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/demo/payment-service/internal/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// PaymentService handles payment processing with fault injection for testing
type PaymentService struct {
	tracer          trace.Tracer
	delayMS         int     // Artificial delay in milliseconds
	errorPercentage float64 // Percentage of requests that should error (0-100)
}

// NewPaymentService creates a payment service with configurable fault injection
func NewPaymentService() *PaymentService {
	delayMS, _ := strconv.Atoi(os.Getenv("PAYMENT_DELAY_MS"))
	errorPct, _ := strconv.ParseFloat(os.Getenv("PAYMENT_ERROR_PCT"), 64)

	return &PaymentService{
		tracer:          tracing.GetTracer("payment-service"),
		delayMS:         delayMS,
		errorPercentage: errorPct,
	}
}

// ChargeRequest represents a payment charge request
type ChargeRequest struct {
	OrderID    string  `json:"order_id" binding:"required"`
	MerchantID string  `json:"merchant_id" binding:"required"`
	Amount     float64 `json:"amount" binding:"required,gt=0"`
	Currency   string  `json:"currency" binding:"required"`
}

// ChargeResponse represents a payment charge response
type ChargeResponse struct {
	TransactionID string  `json:"transaction_id"`
	Status        string  `json:"status"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
}

// ProcessCharge processes a payment charge with instrumentation and fault injection
func (s *PaymentService) ProcessCharge(ctx context.Context, req ChargeRequest) (*ChargeResponse, error) {
	ctx, span := s.tracer.Start(ctx, "processCharge",
		trace.WithAttributes(
			attribute.String("order.id", req.OrderID),
			attribute.String("merchant.id", req.MerchantID),
			attribute.Float64("payment.amount", req.Amount),
			attribute.String("payment.currency", req.Currency),
		),
	)
	defer span.End()

	// Apply artificial delay if configured (for testing timeouts)
	if s.delayMS > 0 {
		span.SetAttributes(attribute.Int("fault.injected_delay_ms", s.delayMS))
		time.Sleep(time.Duration(s.delayMS) * time.Millisecond)
	}

	// Apply error injection if configured (for testing retries)
	if s.errorPercentage > 0 && rand.Float64()*100 < s.errorPercentage {
		span.SetAttributes(attribute.Bool("fault.injected_error", true))
		span.SetStatus(codes.Error, "injected error for testing")
		return nil, fmt.Errorf("payment gateway error (injected)")
	}

	// Validate request
	if err := s.validateRequest(ctx, req); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// Simulate payment gateway call
	transactionID, err := s.callPaymentGateway(ctx, req)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	response := &ChargeResponse{
		TransactionID: transactionID,
		Status:        "approved",
		Amount:        req.Amount,
		Currency:      req.Currency,
	}

	span.SetStatus(codes.Ok, "payment processed")
	return response, nil
}

// validateRequest validates the payment request
func (s *PaymentService) validateRequest(ctx context.Context, req ChargeRequest) error {
	_, span := s.tracer.Start(ctx, "validate")
	defer span.End()

	// Simulate validation logic
	time.Sleep(5 * time.Millisecond)

	if req.Amount <= 0 {
		return fmt.Errorf("invalid amount: %f", req.Amount)
	}

	span.SetStatus(codes.Ok, "validation passed")
	return nil
}

// callPaymentGateway simulates calling an external payment gateway
func (s *PaymentService) callPaymentGateway(ctx context.Context, req ChargeRequest) (string, error) {
	_, span := s.tracer.Start(ctx, "gatewayCall")
	defer span.End()

	// Simulate gateway API call latency
	time.Sleep(20 * time.Millisecond)

	transactionID := uuid.New().String()
	span.SetAttributes(attribute.String("transaction.id", transactionID))
	span.SetStatus(codes.Ok, "gateway call successful")

	return transactionID, nil
}
