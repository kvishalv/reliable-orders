package handler

import (
	"net/http"

	"github.com/demo/order-service/internal/service"
	"github.com/gin-gonic/gin"
)

// OrderHandler handles HTTP requests for orders
type OrderHandler struct {
	orderService *service.OrderService
}

// NewOrderHandler creates a new order handler
func NewOrderHandler(orderService *service.OrderService) *OrderHandler {
	return &OrderHandler{
		orderService: orderService,
	}
}

// CreateOrder handles POST /orders
// Expects Idempotency-Key header for safe retries
func (h *OrderHandler) CreateOrder(c *gin.Context) {
	var req service.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Extract idempotency key from header
	idempotencyKey := c.GetHeader("Idempotency-Key")

	// Create order with all reliability patterns applied
	resp, err := h.orderService.CreateOrder(c.Request.Context(), req, idempotencyKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// Health handles GET /health for health checks
func (h *OrderHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}
