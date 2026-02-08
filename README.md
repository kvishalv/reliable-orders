# Reliable Orders - Microservices Demo

A production-grade Go microservices demonstration showcasing distributed tracing, reliability patterns, and fault tolerance using OpenTelemetry and Jaeger.

## Architecture

```
┌─────────────┐      ┌──────────────┐      ┌──────────────┐
│  Load Test  │─────▶│    Order     │─────▶│   Payment    │
│   Client    │      │   Service    │      │   Service    │
└─────────────┘      └──────────────┘      └──────────────┘
                            │                      │
                            └──────────┬───────────┘
                                       ▼
                              ┌─────────────────┐
                              │ OTel Collector  │
                              └─────────────────┘
                                       │
                    ┌──────────────────┼──────────────────┐
                    ▼                  ▼                  ▼
              ┌─────────┐        ┌──────────┐      ┌──────────┐
              │ Jaeger  │        │Prometheus│      │ Grafana  │
              │   UI    │        │          │      │          │
              └─────────┘        └──────────┘      └──────────┘
```

## Features

### Distributed Tracing (OpenTelemetry + Jaeger)
- **W3C Trace Context propagation** across service boundaries
- **Automatic HTTP span creation** with method and route naming
- **Custom span attributes** for business metrics (order.id, merchant.id, retry.attempt, cb.state)
- **Child span creation** for logical operations (createOrder, callPayment, validate, gatewayCall)
- **End-to-end trace visualization** in Jaeger UI

### Reliability Patterns (Order Service)

1. **Client Timeouts & Context Deadlines**
   - 500ms budget for payment calls
   - Prevents resource exhaustion from slow dependencies
   - Graceful timeout handling with proper error messages

2. **Retries with Exponential Backoff**
   - Max 3 attempts (initial + 2 retries)
   - Exponential backoff: 50ms → 100ms → 200ms
   - ±30% jitter to prevent thundering herd
   - Retries only on transient failures (5xx, 429, network errors)
   - Does NOT retry on 4xx client errors

3. **Circuit Breaker**
   - Opens after 5 consecutive failures or 60% failure rate
   - 30s timeout before attempting recovery
   - Fails fast when open, preventing cascading failures
   - Tracks state in spans (cb.state, cb.open attributes)

4. **Bulkhead (Concurrency Limiter)**
   - Limits to 10 concurrent payment calls
   - Prevents resource exhaustion during traffic spikes
   - Uses semaphore-based admission control
   - Tracks capacity usage in spans

5. **Idempotency**
   - Accepts `Idempotency-Key` header
   - Returns cached response for duplicate requests
   - Prevents duplicate charges under retry scenarios
   - 24-hour retention with automatic cleanup

### Fault Injection (Payment Service)

Environment variables to simulate real-world failures:
- `PAYMENT_DELAY_MS`: Artificial delay (e.g., 300ms for timeout testing)
- `PAYMENT_ERROR_PCT`: Error rate percentage (e.g., 30 for 30% failures)
- `RATE_LIMIT_PCT`: 429 response rate (e.g., 10 for 10% rate limiting)

## Quick Start

### Prerequisites
- Docker and Docker Compose
- Make (optional, for convenience commands)
- Go 1.21+ (for local development only)

### One-Command Setup

```bash
# Start all services
docker compose up --build

# Or use Make for convenience
make up
```

This starts:
- **Order Service** on http://localhost:8080
- **Payment Service** on http://localhost:8081
- **Jaeger UI** on http://localhost:16686

### With Metrics (Optional)

```bash
# Start with Prometheus and Grafana
docker compose --profile metrics up --build

# Or use Make
make up-metrics
```

This additionally provides:
- **Prometheus** on http://localhost:9090
- **Grafana** on http://localhost:3000 (admin/admin)

## Usage Examples

### Create an Order (curl)

```bash
# Simple order creation
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "merchant_id": "merchant_123",
    "amount": 99.99,
    "currency": "USD"
  }'
```

### Create Order with Idempotency

```bash
# First request
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: order-abc-123" \
  -d '{
    "merchant_id": "merchant_123",
    "amount": 99.99,
    "currency": "USD"
  }'

# Duplicate request returns same order (no duplicate charge)
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: order-abc-123" \
  -d '{
    "merchant_id": "merchant_123",
    "amount": 99.99,
    "currency": "USD"
  }'
```

### Health Check

```bash
curl http://localhost:8080/health
curl http://localhost:8081/health
```

## Load Testing

### Using Make (Recommended)

```bash
# Light load (100 requests, 10 concurrent)
make load

# Heavy load (1000 requests, 50 concurrent)
make load-heavy

# Stress test (1000 requests, 100 concurrent)
make load-stress
```

### Using Docker Directly

```bash
docker compose run --rm loadgen \
  -url http://order-service:8080/orders \
  -n 1000 \
  -c 100 \
  -t 5s
```

### Using Loadgen Binary (Local)

```bash
cd loadgen
go run cmd/main.go \
  -url http://localhost:8080/orders \
  -n 1000 \
  -c 100 \
  -idempotent
```

## Fault Injection Testing

### Inject Delay (Test Timeouts)

```bash
# Add 300ms delay to payment service
make fault-delay

# Or manually
docker compose stop payment-service
docker compose rm -f payment-service
PAYMENT_DELAY_MS=300 docker compose up -d payment-service
```

Now run load test and observe in Jaeger:
- Increased latency spans
- Timeout errors when delay exceeds 500ms budget
- Retry attempts visible in traces

### Inject Errors (Test Retries & Circuit Breaker)

```bash
# Inject 30% error rate
make fault-errors

# Or manually
docker compose stop payment-service
docker compose rm -f payment-service
PAYMENT_ERROR_PCT=30 docker compose up -d payment-service
```

Run load test and observe:
- Retry spans with backoff timings
- Circuit breaker opening after consecutive failures
- Successful requests after retries

### Combined Faults

```bash
# Inject both delay and errors
make fault-combined

# Clear all faults
make fault-clear
```

## Observability

### Viewing Traces in Jaeger

1. Open http://localhost:16686
2. Select Service: `order-service`
3. Click "Find Traces"
4. Click on a trace to see the full request flow

**Key Observations:**
- Server span: `HTTP POST /orders`
- Child spans: `createOrder`, `callPayment`, `persistOrder`, `validate`, `gatewayCall`
- Span attributes: `order.id`, `merchant.id`, `timeout_ms`, `retry.attempt`, `cb.state`
- Error spans marked in red with error messages

### Analyzing Reliability Patterns

**Timeouts:**
```
Look for spans with timeout_ms attribute
Spans exceeding 500ms will show context deadline errors
```

**Retries:**
```
Search for retry.attempt attribute in spans
Observe exponential backoff in span timing
Look for retry.succeeded=true on final attempt
```

**Circuit Breaker:**
```
Filter spans with cb.state attribute
Watch for cb.open=true when failures accumulate
Observe cb.state transitions: closed → open → half-open → closed
```

**Bulkhead:**
```
Check bulkhead.max attribute on callPayment spans
Look for "bulkhead limit reached" errors under heavy load
```

**Idempotency:**
```
Search for idempotency.key attribute
Compare CreatedAt timestamps for duplicate keys
Observe "idempotent_request_cached" events
```

## Project Structure

```
.
├── order-service/          # Main order processing service
│   ├── cmd/
│   │   └── main.go        # Service entrypoint
│   ├── internal/
│   │   ├── handler/       # HTTP handlers
│   │   ├── service/       # Business logic
│   │   ├── reliability/   # Reliability patterns
│   │   └── tracing/       # OpenTelemetry setup
│   ├── Dockerfile
│   └── go.mod
│
├── payment-service/        # Downstream payment service
│   ├── cmd/
│   │   └── main.go
│   ├── internal/
│   │   ├── handler/
│   │   ├── service/
│   │   └── tracing/
│   ├── Dockerfile
│   └── go.mod
│
├── loadgen/                # Load testing tool
│   ├── cmd/
│   │   └── main.go
│   ├── Dockerfile
│   └── go.mod
│
├── config/                 # Configuration files
│   ├── otel-collector-config.yaml
│   ├── prometheus.yml
│   └── grafana/
│       └── datasources.yml
│
├── docs/
│   └── demo.md            # Step-by-step demo script
│
├── docker-compose.yml
├── Makefile
└── README.md
```

## Development

### Running Locally (Without Docker)

```bash
# Terminal 1: Start Jaeger
docker run -d --name jaeger \
  -p 16686:16686 \
  -p 4317:4317 \
  jaegertracing/all-in-one:1.52

# Terminal 2: Start OTel Collector
docker run -d --name otel-collector \
  -p 4317:4317 \
  -v $(pwd)/config/otel-collector-config.yaml:/etc/otel-collector-config.yaml \
  otel/opentelemetry-collector:0.91.0 \
  --config=/etc/otel-collector-config.yaml

# Terminal 3: Start Payment Service
cd payment-service
go run cmd/main.go

# Terminal 4: Start Order Service
cd order-service
PAYMENT_SERVICE_URL=http://localhost:8081 go run cmd/main.go
```

### Running Tests

```bash
# Unit tests (when added)
cd order-service && go test ./...
cd payment-service && go test ./...

# Integration test
make test
```

## Make Commands Reference

```bash
make help           # Show all available commands
make build          # Build all Docker images
make up             # Start basic services
make up-metrics     # Start with Prometheus/Grafana
make down           # Stop all services
make logs           # Tail all logs
make test           # Run single test request
make load           # Light load test
make load-heavy     # Heavy load test
make load-stress    # Stress test
make fault-delay    # Inject 300ms delay
make fault-errors   # Inject 30% errors
make fault-combined # Inject delay + errors
make fault-clear    # Clear all faults
make jaeger         # Open Jaeger UI
make clean          # Clean up everything
```

## Reliability Patterns Explained

### Why Each Pattern Matters

**Timeouts**: Without timeouts, slow dependencies can exhaust all available threads/goroutines, making your service unresponsive to new requests.

**Retries**: Network glitches and transient failures are common. Retries with backoff allow you to ride through temporary issues without manual intervention.

**Circuit Breaker**: When a dependency is consistently failing, continuing to call it wastes resources and increases latency. The circuit breaker fails fast, giving the dependency time to recover.

**Bulkhead**: Limits blast radius. If one dependency is slow, it won't consume all your resources, allowing other operations to continue normally.

**Idempotency**: Networks can fail after a request is sent but before the response is received. Clients will retry, but without idempotency, you might double-charge users or create duplicate orders.

## Advanced Scenarios

See [docs/demo.md](docs/demo.md) for detailed step-by-step demonstrations of:
- Baseline performance testing
- Timeout handling under delay
- Retry behavior under transient failures
- Circuit breaker opening and recovery
- Idempotency preventing duplicates
- Combined fault scenarios
- Metrics analysis (if using Prometheus/Grafana)

## Troubleshooting

### Services won't start
```bash
# Check if ports are already in use
lsof -i :8080
lsof -i :8081
lsof -i :16686

# Clean up and restart
make clean
make up
```

### No traces in Jaeger
```bash
# Check if services are healthy
make test

# Check otel-collector logs
docker compose logs otel-collector

# Verify connectivity
docker compose exec order-service nc -zv otel-collector 4317
```

### Load test times out
```bash
# Increase timeout in loadgen
docker compose run --rm loadgen \
  -url http://order-service:8080/orders \
  -n 100 -c 10 -t 30s
```

## Production Considerations

This is a **demo project** for learning. For production use, consider:

1. **Persistence**: Replace in-memory idempotency store with Redis/database
2. **Metrics**: Add Prometheus instrumentation for SLI/SLO tracking
3. **Security**: Add authentication, rate limiting, TLS
4. **Configuration**: Use proper config management (Viper, env files)
5. **Observability**: Add structured logging with trace ID correlation
6. **Testing**: Add unit tests, integration tests, chaos engineering
7. **Deployment**: Add Kubernetes manifests, health checks, readiness probes
8. **CI/CD**: Add GitHub Actions, automated testing, security scanning

## License

MIT License - See LICENSE file for details

## Contributing

This is a portfolio demo project. Feel free to fork and modify for your own learning!
