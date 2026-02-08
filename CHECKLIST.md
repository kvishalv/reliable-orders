# Project Verification Checklist

This checklist helps you verify that the project is set up correctly and all components are working.

## Initial Setup Verification

### 1. Prerequisites Check
- [ ] Docker installed and running: `docker --version`
- [ ] Docker Compose installed: `docker compose version`
- [ ] Make installed (optional): `make --version`

### 2. File Structure Verification
```bash
# Run from project root
ls -R
```

Expected structure:
```
order-service/
├── cmd/main.go
├── internal/
│   ├── handler/
│   ├── service/
│   ├── reliability/
│   └── tracing/
├── Dockerfile
└── go.mod

payment-service/
├── cmd/main.go
├── internal/
│   ├── handler/
│   ├── service/
│   └── tracing/
├── Dockerfile
└── go.mod

loadgen/
├── cmd/main.go
├── Dockerfile
└── go.mod

config/
├── otel-collector-config.yaml
├── prometheus.yml
└── grafana/

docs/
└── demo.md
```

## Build Verification

### 3. Build All Services
```bash
make build
# Or:
docker compose build
```

**Expected:** All 5 services build successfully:
- [ ] order-service
- [ ] payment-service
- [ ] otel-collector
- [ ] jaeger
- [ ] loadgen

### 4. Start Services
```bash
make up
# Or:
docker compose up -d
```

**Expected:** All services start without errors

### 5. Check Service Health
```bash
# Wait 10 seconds for services to start
sleep 10

# Check order service
curl http://localhost:8080/health
# Expected: {"status":"healthy"}

# Check payment service
curl http://localhost:8081/health
# Expected: {"status":"healthy"}

# Check Jaeger UI
curl -s http://localhost:16686 | grep -q "Jaeger" && echo "Jaeger UI is up" || echo "Jaeger UI failed"
```

- [ ] Order service responds with healthy status
- [ ] Payment service responds with healthy status
- [ ] Jaeger UI is accessible

## Functionality Verification

### 6. Create a Single Order
```bash
make test
# Or:
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: test-$(date +%s)" \
  -d '{"merchant_id":"merchant_123","amount":99.99,"currency":"USD"}'
```

**Expected:**
- [ ] HTTP 200 response
- [ ] Response contains: order_id, status=completed, created_at
- [ ] No errors in logs: `docker compose logs order-service payment-service`

### 7. Verify Traces in Jaeger
1. Open http://localhost:16686
2. Select Service: `order-service`
3. Click "Find Traces"
4. Click on the most recent trace

**Expected:**
- [ ] Trace shows full request flow
- [ ] Spans visible: HTTP POST /orders → createOrder → callPayment → processCharge → validate → gatewayCall → persistOrder
- [ ] No error spans (no red spans)
- [ ] Span attributes present: order.id, merchant.id, timeout_ms, cb.state

### 8. Run Load Test
```bash
make load
# Or:
docker compose run --rm loadgen -url http://order-service:8080/orders -n 100 -c 10
```

**Expected:**
- [ ] 100 total requests
- [ ] 100 successful requests
- [ ] 0 failed requests
- [ ] Average latency ~30-50ms
- [ ] P95 latency ~50-80ms

## Reliability Patterns Verification

### 9. Test Idempotency
```bash
# First request
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: verify-idempotency-001" \
  -d '{"merchant_id":"merchant_123","amount":99.99,"currency":"USD"}' | jq .

# Note the order_id

# Second request (same key)
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: verify-idempotency-001" \
  -d '{"merchant_id":"merchant_123","amount":99.99,"currency":"USD"}' | jq .
```

**Expected:**
- [ ] Both requests return same order_id
- [ ] Both requests return same created_at
- [ ] Second request is much faster (~1-2ms)
- [ ] In Jaeger: second trace shows "idempotent_request_cached" event

### 10. Test Timeout Handling
```bash
# Inject 600ms delay (exceeds 500ms budget)
make fault-delay

# Wait for service restart
sleep 5

# Run test
make test
```

**Expected:**
- [ ] Request fails with HTTP 500
- [ ] Error message contains "context deadline exceeded"
- [ ] In Jaeger: span duration ~500ms (not 600ms)
- [ ] Clear fault: `make fault-clear`

### 11. Test Retry Behavior
```bash
# Inject 30% error rate
make fault-errors
sleep 5

# Run load test
make load
```

**Expected:**
- [ ] ~85-95% success rate (some succeed after retries)
- [ ] Higher average latency (~65ms due to retries)
- [ ] In Jaeger: traces show retry.attempt attributes
- [ ] Clear fault: `make fault-clear`

### 12. Test Circuit Breaker
```bash
# Inject 80% error rate
docker compose stop payment-service
docker compose rm -f payment-service
PAYMENT_ERROR_PCT=80 docker compose up -d payment-service
sleep 5

# Run heavy load
make load-heavy
```

**Expected:**
- [ ] Initial requests fail with retries
- [ ] After ~10-20 requests, circuit opens
- [ ] In Jaeger: later traces show cb.state=open and cb.open=true
- [ ] Requests fail immediately with "circuit breaker open"
- [ ] Clear fault: `make fault-clear`

## Optional: Metrics Verification

### 13. Start with Metrics (Optional)
```bash
make down
make up-metrics
sleep 15
```

**Expected:**
- [ ] Prometheus accessible at http://localhost:9090
- [ ] Grafana accessible at http://localhost:3000 (admin/admin)

## Troubleshooting Common Issues

### Services won't start
```bash
# Check port conflicts
lsof -i :8080
lsof -i :8081
lsof -i :16686

# Clean and restart
make clean
make up
```

### No traces in Jaeger
```bash
# Check otel-collector logs
docker compose logs otel-collector

# Verify service can reach collector
docker compose exec order-service ping -c 3 otel-collector
```

### Build fails
```bash
# Check Docker daemon
docker info

# Clean build cache
docker system prune -f

# Rebuild
docker compose build --no-cache
```

## Final Checklist

- [ ] All services build successfully
- [ ] All services start and are healthy
- [ ] Single order request works
- [ ] Traces visible in Jaeger
- [ ] Load test completes successfully
- [ ] Idempotency works correctly
- [ ] Timeout handling works
- [ ] Retries work with errors
- [ ] Circuit breaker opens under sustained failures
- [ ] Documentation is clear and accurate

## Success Criteria

✅ **All items checked above**
✅ **No errors in service logs**
✅ **Jaeger shows complete distributed traces**
✅ **All reliability patterns demonstrated successfully**

## Next Steps

Once all checks pass:
1. Review `README.md` for detailed documentation
2. Follow `docs/demo.md` for comprehensive demos
3. Experiment with different fault scenarios
4. Customize for your portfolio presentation

---

**Project Status:** Ready for demonstration ✓
