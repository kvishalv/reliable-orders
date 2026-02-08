package handler

import (
	"math/rand"
	"net/http"
	"os"
	"strconv"

	"github.com/demo/payment-service/internal/service"
	"github.com/gin-gonic/gin"
)

// PaymentHandler handles HTTP requests for payments
type PaymentHandler struct {
	paymentService *service.PaymentService
	rateLimitPct   float64
}

// NewPaymentHandler creates a new payment handler
func NewPaymentHandler(paymentService *service.PaymentService) *PaymentHandler {
	rateLimitPct, _ := strconv.ParseFloat(os.Getenv("RATE_LIMIT_PCT"), 64)
	return &PaymentHandler{
		paymentService: paymentService,
		rateLimitPct:   rateLimitPct,
	}
}

// Charge handles POST /charge
func (h *PaymentHandler) Charge(c *gin.Context) {
	// Simulate rate limiting (429 responses)
	if h.rateLimitPct > 0 && rand.Float64()*100 < h.rateLimitPct {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": "rate limit exceeded",
		})
		return
	}

	var req service.ChargeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.paymentService.ProcessCharge(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// Health handles GET /health
func (h *PaymentHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}
