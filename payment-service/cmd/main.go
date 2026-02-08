package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/demo/payment-service/internal/handler"
	"github.com/demo/payment-service/internal/service"
	"github.com/demo/payment-service/internal/tracing"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func main() {
	// Initialize OpenTelemetry tracing
	collectorEndpoint := getEnv("OTEL_COLLECTOR_ENDPOINT", "otel-collector:4317")
	shutdown, err := tracing.InitTracer("payment-service", collectorEndpoint)
	if err != nil {
		log.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer: %v", err)
		}
	}()

	log.Println("OpenTelemetry initialized, sending traces to", collectorEndpoint)

	// Log fault injection settings
	if delayMS := os.Getenv("PAYMENT_DELAY_MS"); delayMS != "" {
		log.Printf("Fault injection: PAYMENT_DELAY_MS=%s", delayMS)
	}
	if errorPct := os.Getenv("PAYMENT_ERROR_PCT"); errorPct != "" {
		log.Printf("Fault injection: PAYMENT_ERROR_PCT=%s%%", errorPct)
	}
	if rateLimitPct := os.Getenv("RATE_LIMIT_PCT"); rateLimitPct != "" {
		log.Printf("Fault injection: RATE_LIMIT_PCT=%s%%", rateLimitPct)
	}

	// Create Gin router with OpenTelemetry middleware
	router := gin.Default()
	router.Use(otelgin.Middleware("payment-service"))

	// Initialize service and handlers
	paymentService := service.NewPaymentService()
	paymentHandler := handler.NewPaymentHandler(paymentService)

	// Register routes
	router.POST("/charge", paymentHandler.Charge)
	router.GET("/health", paymentHandler.Health)

	// Start HTTP server with graceful shutdown
	port := getEnv("PORT", "8081")
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	go func() {
		log.Printf("Starting payment-service on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

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
