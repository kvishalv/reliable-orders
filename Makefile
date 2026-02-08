.PHONY: help build up down logs test load clean demo

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build all services
	docker compose build

up: ## Start all services (basic setup)
	docker compose up --build -d
	@echo ""
	@echo "Services started! Available at:"
	@echo "  - Order Service:  http://localhost:8080"
	@echo "  - Payment Service: http://localhost:8081"
	@echo "  - Jaeger UI:      http://localhost:16686"
	@echo ""

up-metrics: ## Start all services including Prometheus and Grafana
	docker compose --profile metrics up --build -d
	@echo ""
	@echo "Services started! Available at:"
	@echo "  - Order Service:  http://localhost:8080"
	@echo "  - Payment Service: http://localhost:8081"
	@echo "  - Jaeger UI:      http://localhost:16686"
	@echo "  - Prometheus:     http://localhost:9090"
	@echo "  - Grafana:        http://localhost:3000 (admin/admin)"
	@echo ""

down: ## Stop all services
	docker compose --profile metrics down

logs: ## Tail logs from all services
	docker compose logs -f

logs-order: ## Tail logs from order service
	docker compose logs -f order-service

logs-payment: ## Tail logs from payment service
	docker compose logs -f payment-service

test: ## Run a simple test request
	@echo "Creating an order..."
	@curl -X POST http://localhost:8080/orders \
		-H "Content-Type: application/json" \
		-H "Idempotency-Key: test-$(shell date +%s)" \
		-d '{"merchant_id":"merchant_123","amount":99.99,"currency":"USD"}' | jq .

load: ## Run load test (100 requests, 10 concurrent)
	docker compose run --rm loadgen -url http://order-service:8080/orders -n 100 -c 10

load-heavy: ## Run heavy load test (1000 requests, 50 concurrent)
	docker compose run --rm loadgen -url http://order-service:8080/orders -n 1000 -c 50

load-stress: ## Run stress test (1000 requests, 100 concurrent)
	docker compose run --rm loadgen -url http://order-service:8080/orders -n 1000 -c 100

fault-delay: ## Inject 300ms delay in payment service
	docker compose stop payment-service
	docker compose rm -f payment-service
	PAYMENT_DELAY_MS=300 docker compose up -d payment-service
	@echo "Payment service restarted with 300ms delay"

fault-errors: ## Inject 30% error rate in payment service
	docker compose stop payment-service
	docker compose rm -f payment-service
	PAYMENT_ERROR_PCT=30 docker compose up -d payment-service
	@echo "Payment service restarted with 30% error rate"

fault-combined: ## Inject both delay (200ms) and errors (20%)
	docker compose stop payment-service
	docker compose rm -f payment-service
	PAYMENT_DELAY_MS=200 PAYMENT_ERROR_PCT=20 docker compose up -d payment-service
	@echo "Payment service restarted with 200ms delay and 20% errors"

fault-clear: ## Clear all faults (restart with no delays/errors)
	docker compose stop payment-service
	docker compose rm -f payment-service
	docker compose up -d payment-service
	@echo "Payment service restarted with no faults"

jaeger: ## Open Jaeger UI in browser
	@echo "Opening Jaeger UI at http://localhost:16686"
	@open http://localhost:16686 2>/dev/null || xdg-open http://localhost:16686 2>/dev/null || echo "Please open http://localhost:16686 in your browser"

clean: ## Clean up all containers, volumes, and images
	docker compose --profile metrics down -v
	docker system prune -f

demo: ## Run the complete demo scenario
	@echo "Starting demo scenario..."
	@./docs/demo.sh
