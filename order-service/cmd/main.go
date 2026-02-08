package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/demo/order-service/internal/handler"
	"github.com/demo/order-service/internal/service"
	"github.com/demo/order-service/internal/tracing"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func main() {
	// Initialize OpenTelemetry tracing
	collectorEndpoint := getEnv("OTEL_COLLECTOR_ENDPOINT", "otel-collector:4317")
	shutdown, err := tracing.InitTracer("order-service", collectorEndpoint)
	if err != nil {
		log.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer: %v", err)
		}
	}()

	log.Println("OpenTelemetry initialized, sending traces to", collectorEndpoint)

	// Create Gin router with OpenTelemetry middleware
	router := gin.Default()

	// Add OpenTelemetry middleware to auto-instrument HTTP requests
	// This creates server spans named "HTTP {method} {route}" for each request
	router.Use(otelgin.Middleware("order-service"))

	// Initialize service and handlers
	paymentURL := getEnv("PAYMENT_SERVICE_URL", "http://payment-service:8081")
	orderService := service.NewOrderService(paymentURL)
	orderHandler := handler.NewOrderHandler(orderService)

	// Register routes
	router.POST("/orders", orderHandler.CreateOrder)
	router.GET("/health", orderHandler.Health)

	// Start HTTP server with graceful shutdown
	port := getEnv("PORT", "8080")
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Starting order-service on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with 5 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
