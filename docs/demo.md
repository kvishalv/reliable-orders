# Reliability Patterns Demo Script

This guide walks you through demonstrating each reliability pattern with real examples and Jaeger trace analysis.

## Setup

Before starting, ensure all services are running:

```bash
make up
# Wait 10 seconds for services to initialize
```

Open Jaeger UI in your browser: http://localhost:16686

## Demo 1: Baseline Performance (No Faults)

### Objective
Establish baseline metrics with no fault injection.

### Steps

1. **Run light load test:**
   ```bash
   make load
   ```

2. **Observe output:**
   ```
   Total Requests:    100
   Successful:        100
   Failed:            0
   Timeout:           0
   Requests/sec:      45.23

   Latency Statistics:
     Average:  35ms
     P50:      32ms
     P95:      55ms
     P99:      78ms
   ```

3. **View traces in Jaeger:**
   - Go to http://localhost:16686
   - Service: `order-service`
   - Click "Find Traces"
   - Click on any trace

4. **Key observations:**
   - Clean trace with no errors
   - Total duration ~30-50ms
   - Spans visible:
     ```
     HTTP POST /orders (order-service)
     ├─ createOrder
     │  ├─ callPayment
     │  │  └─ processCharge (payment-service)
     │  │     ├─ validate
     │  │     └─ gatewayCall
     │  └─ persistOrder
     ```
   - Span attributes: `order.id`, `merchant.id`, `timeout_ms=500`
   - No retry attempts (`retry.attempt=0`)
   - Circuit breaker closed (`cb.state=closed`)

---

## Demo 2: Timeout Handling

### Objective
Show how context deadlines prevent resource exhaustion when payment service is slow.

### Steps

1. **Inject 600ms delay (exceeds 500ms budget):**
   ```bash
   # Stop and restart payment service with delay
   docker compose stop payment-service
   docker compose rm -f payment-service
   PAYMENT_DELAY_MS=600 docker compose up -d payment-service

   # Wait 5 seconds for service to start
   sleep 5
   ```

2. **Run load test:**
   ```bash
   make load
   ```

3. **Observe output:**
   ```
   Total Requests:    100
   Successful:        0
   Failed:            100
   Timeout:           0

   Status Code Distribution:
     500: 100 (100.0%)
   ```

4. **View traces in Jaeger:**
   - Find a failed trace (red error indicator)
   - Click to expand

5. **Key observations:**
   - `callPayment` span duration ~500ms
   - Error message: "context deadline exceeded"
   - Span marked with error status (red)
   - `timeout_ms=500` attribute shows the budget
   - Payment service `processCharge` span shows `fault.injected_delay_ms=600`
   - No retries attempted (timeouts are not retried by default in this implementation)

6. **Real-world implications:**
   - Without timeout: All goroutines would block waiting for slow payment service
   - With timeout: Service remains responsive, fails fast after 500ms
   - User gets error quickly instead of hanging

7. **Clear fault:**
   ```bash
   make fault-clear
   sleep 5
   ```

---

## Demo 3: Retry with Exponential Backoff

### Objective
Demonstrate automatic retries on transient failures with increasing backoff.

### Steps

1. **Inject 30% error rate:**
   ```bash
   docker compose stop payment-service
   docker compose rm -f payment-service
   PAYMENT_ERROR_PCT=30 docker compose up -d payment-service
   sleep 5
   ```

2. **Run load test:**
   ```bash
   make load
   ```

3. **Observe output:**
   ```
   Total Requests:    100
   Successful:        85-95 (varies due to randomness)
   Failed:            5-15

   Latency Statistics:
     Average:  65ms  (higher due to retries)
     P95:      180ms
     P99:      250ms
   ```

4. **View traces in Jaeger:**
   - Find a trace with retries
   - Look for `retry.attempt` attribute

5. **Key observations in successful retry:**
   - First attempt fails: `fault.injected_error=true`
   - Span event: "retry_due_to_error"
   - Second attempt: `retry.attempt=1`, `retry.backoff_ms=~50-65ms` (50ms + jitter)
   - Third attempt (if needed): `retry.attempt=2`, `retry.backoff_ms=~100-130ms`
   - Final success: `retry.succeeded=true`

6. **Key observations in exhausted retry:**
   - Three attempts visible
   - Final error: "retry exhausted after 3 attempts"
   - `retry.exhausted=true`
   - Total duration: ~50ms + ~100ms + ~200ms = ~350ms (with jitter)

7. **Backoff calculation:**
   ```
   Attempt 0: No backoff (initial request)
   Attempt 1: 50ms * 2^0 = 50ms (±30% jitter) = 35-65ms
   Attempt 2: 50ms * 2^1 = 100ms (±30% jitter) = 70-130ms
   ```

8. **Real-world implications:**
   - Rides through transient network glitches
   - Exponential backoff prevents overwhelming struggling service
   - Jitter prevents thundering herd (all clients retrying at same time)

9. **Clear fault:**
   ```bash
   make fault-clear
   sleep 5
   ```

---

## Demo 4: Circuit Breaker Protection

### Objective
Show circuit breaker opening after consecutive failures to fail fast.

### Steps

1. **Inject 80% error rate (high failure scenario):**
   ```bash
   docker compose stop payment-service
   docker compose rm -f payment-service
   PAYMENT_ERROR_PCT=80 docker compose up -d payment-service
   sleep 5
   ```

2. **Run heavy load:**
   ```bash
   make load-heavy
   ```

3. **Observe output:**
   ```
   Total Requests:    1000
   Successful:        ~50-150
   Failed:            ~850-950

   Status Code Distribution:
     200: ~100
     500: ~900
   ```

4. **View traces in Jaeger:**
   - Service: `order-service`
   - Operation: `HTTP POST /orders`
   - Click "Find Traces"
   - Look at timeline view to see pattern

5. **Key observations:**
   - **Early traces** (first ~10-20 requests):
     - `cb.state=closed`
     - Retry attempts visible
     - Some succeed, most fail after exhausting retries

   - **Middle traces** (after 5 consecutive failures):
     - `cb.state=open`
     - `cb.open=true` attribute
     - Error: "circuit breaker open"
     - NO payment service call made (fails immediately)
     - Duration ~1-2ms instead of ~350ms

   - **Later traces** (after 30 seconds):
     - `cb.state=half-open`
     - Allows limited requests through to test recovery
     - If succeed: `cb.state=closed` (circuit recovers)
     - If fail: `cb.state=open` (circuit re-opens)

6. **Circuit breaker states:**
   ```
   CLOSED: Normal operation, all requests pass through
      ↓ (5 consecutive failures OR 60% failure rate with 10+ requests)
   OPEN: Fail fast, no requests to downstream service
      ↓ (after 30 seconds)
   HALF-OPEN: Test recovery with limited requests (3)
      ↓ (if requests succeed)
   CLOSED: Return to normal operation
   ```

7. **Real-world implications:**
   - Prevents cascading failures when downstream is broken
   - Saves resources by failing fast instead of waiting/retrying
   - Gives downstream service time to recover
   - Automatically tests for recovery

8. **Clear fault:**
   ```bash
   make fault-clear
   sleep 35  # Wait for circuit to attempt recovery
   ```

9. **Verify recovery:**
   ```bash
   make test
   # Should succeed
   ```

---

## Demo 5: Idempotency Protection

### Objective
Prove that duplicate requests with same idempotency key return cached response without duplicate processing.

### Steps

1. **Create an order with idempotency key:**
   ```bash
   curl -X POST http://localhost:8080/orders \
     -H "Content-Type: application/json" \
     -H "Idempotency-Key: demo-order-001" \
     -d '{
       "merchant_id": "merchant_demo",
       "amount": 199.99,
       "currency": "USD"
     }' | jq .
   ```

2. **Note the response:**
   ```json
   {
     "order_id": "a1b2c3d4-1234-5678-90ab-cdef12345678",
     "status": "completed",
     "created_at": "2024-01-15T10:30:45Z"
   }
   ```

3. **Send identical request again:**
   ```bash
   curl -X POST http://localhost:8080/orders \
     -H "Content-Type: application/json" \
     -H "Idempotency-Key: demo-order-001" \
     -d '{
       "merchant_id": "merchant_demo",
       "amount": 199.99,
       "currency": "USD"
     }' | jq .
   ```

4. **Observe:**
   - **SAME** order_id returned
   - **SAME** created_at timestamp
   - Response is nearly instant (~1-2ms vs ~30-50ms)

5. **View traces in Jaeger:**
   - First request trace:
     - Full flow: createOrder → callPayment → processCharge → persistOrder
     - `idempotency.key=demo-order-001`
     - Duration ~35ms

   - Second request trace:
     - Only `HTTP POST /orders` span
     - Span event: "idempotent_request_cached"
     - NO payment call made
     - NO database persist
     - Duration ~1ms

6. **Test with different idempotency key:**
   ```bash
   curl -X POST http://localhost:8080/orders \
     -H "Content-Type: application/json" \
     -H "Idempotency-Key: demo-order-002" \
     -d '{
       "merchant_id": "merchant_demo",
       "amount": 199.99,
       "currency": "USD"
     }' | jq .
   ```

7. **Observe:**
   - **DIFFERENT** order_id (new order created)
   - Payment service called
   - Full processing occurs

8. **Real-world implications:**
   - Client can safely retry on network failures without risk of duplicate charges
   - Common in payment systems: Stripe, Square, PayPal all use idempotency
   - 24-hour retention means you can retry anytime within that window
   - Critical for at-least-once delivery guarantees

---

## Demo 6: Bulkhead (Concurrency Limiting)

### Objective
Show how bulkhead prevents resource exhaustion under heavy concurrent load.

### Steps

1. **Inject 400ms delay (within timeout but slow):**
   ```bash
   docker compose stop payment-service
   docker compose rm -f payment-service
   PAYMENT_DELAY_MS=400 docker compose up -d payment-service
   sleep 5
   ```

2. **Run stress test (100 concurrent):**
   ```bash
   docker compose run --rm loadgen \
     -url http://order-service:8080/orders \
     -n 100 \
     -c 100
   ```

3. **Observe output:**
   ```
   Total Requests:    100
   Successful:        ~90-95
   Failed:            ~5-10

   Latency Statistics:
     Average:  420ms
     P95:      480ms  (some queuing due to bulkhead)
     P99:      500ms
   ```

4. **View traces in Jaeger:**
   - Find traces with different latencies
   - Some fast (~400ms), some slower (~450-480ms)

5. **Key observations:**
   - `bulkhead.max=10` attribute on callPayment spans
   - First 10 requests: ~400ms latency (processing immediately)
   - Requests 11-100: Slight queuing delay waiting for bulkhead slot
   - No bulkhead rejections (because requests complete before context deadline)

6. **Increase load to trigger rejections:**
   ```bash
   docker compose run --rm loadgen \
     -url http://order-service:8080/orders \
     -n 200 \
     -c 200 \
     -t 10s
   ```

7. **Observe traces with rejections:**
   - Error: "bulkhead limit reached"
   - `bulkhead.rejected=true`
   - Very fast failure (~1ms) instead of waiting

8. **Real-world implications:**
   - Limits blast radius of slow dependencies
   - Prevents thread/goroutine pool exhaustion
   - Service remains responsive for other operations
   - Better than unlimited concurrency causing OOM

9. **Clear fault:**
   ```bash
   make fault-clear
   ```

---

## Demo 7: Combined Fault Scenario

### Objective
Demonstrate all patterns working together under realistic mixed faults.

### Steps

1. **Inject realistic fault combination:**
   ```bash
   docker compose stop payment-service
   docker compose rm -f payment-service
   PAYMENT_DELAY_MS=150 PAYMENT_ERROR_PCT=10 RATE_LIMIT_PCT=5 docker compose up -d payment-service
   sleep 5
   ```

   This simulates:
   - Slight delay (150ms latency)
   - 10% error rate (occasional failures)
   - 5% rate limiting (429 responses)

2. **Run sustained load:**
   ```bash
   make load-heavy
   ```

3. **Observe output:**
   ```
   Total Requests:    1000
   Successful:        ~950-970
   Failed:            ~30-50

   Latency Statistics:
     Average:  165ms
     P50:      155ms
     P95:      320ms  (includes retry attempts)
     P99:      480ms

   Status Code Distribution:
     200: ~960
     500: ~30
     429: ~10
   ```

4. **Analyze traces in Jaeger:**

   **Successful request (no faults):**
   - Duration: ~150ms
   - No retries
   - CB state: closed

   **Successful after retry:**
   - First attempt failed (injected error)
   - Retry with backoff
   - Second attempt succeeded
   - Duration: ~150ms + 50ms backoff + 150ms = ~350ms
   - `retry.succeeded=true`

   **Rate limited and retried:**
   - First attempt: 429 response
   - Retry after backoff
   - Second attempt: 200 response
   - Shows retry works for 429 as designed

   **Timeout scenario:**
   - If delay + retries > 500ms total budget
   - Context deadline exceeded
   - Circuit breaker tracks this failure

5. **Real-world implications:**
   - Multiple patterns work together seamlessly
   - Retries handle transient failures
   - Timeouts prevent hanging on slow requests
   - Circuit breaker protects against sustained issues
   - Bulkhead prevents resource exhaustion
   - System degrades gracefully under pressure

6. **Clear faults:**
   ```bash
   make fault-clear
   ```

---

## Demo 8: Metrics Analysis (Optional)

**Note:** Requires starting with `make up-metrics`

### Steps

1. **Ensure metrics stack is running:**
   ```bash
   make down
   make up-metrics
   sleep 15
   ```

2. **Generate load:**
   ```bash
   make load-heavy
   ```

3. **Open Prometheus:** http://localhost:9090

4. **Sample PromQL queries:**

   **Request rate:**
   ```promql
   rate(http_requests_total[1m])
   ```

   **Error rate:**
   ```promql
   rate(http_requests_total{status_code=~"5.."}[1m])
   ```

   **P95 latency (if instrumented):**
   ```promql
   histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[1m]))
   ```

   **Circuit breaker state (custom metric if added):**
   ```promql
   circuit_breaker_state{service="order-service",dependency="payment"}
   ```

5. **Open Grafana:** http://localhost:3000 (admin/admin)

6. **Create dashboard:**
   - Add panel for request rate
   - Add panel for error rate
   - Add panel for latency percentiles
   - Add panel for circuit breaker state

7. **Inject faults and observe metrics change in real-time**

---

## Summary: What We Demonstrated

### Timeouts ✓
- Prevent resource exhaustion from slow dependencies
- Fail fast with context deadlines
- Visible in traces as deadline exceeded errors

### Retries ✓
- Handle transient failures automatically
- Exponential backoff prevents thundering herd
- Jitter distributes retry timing
- Visible in traces with retry.attempt attributes

### Circuit Breaker ✓
- Opens after sustained failures
- Fails fast when open (saves resources)
- Automatically tests for recovery
- Visible in traces with cb.state attribute

### Bulkhead ✓
- Limits concurrent requests to dependency
- Prevents goroutine exhaustion
- Graceful degradation under load
- Visible in traces with bulkhead.max attribute

### Idempotency ✓
- Prevents duplicate processing
- Safe retries for clients
- Cached responses for duplicate keys
- Visible in traces with idempotent_request_cached event

### Distributed Tracing ✓
- End-to-end request visibility
- W3C Trace Context propagation
- Span attributes for debugging
- Performance analysis and bottleneck identification

---

## Cleanup

```bash
make down
```

Or with metrics:
```bash
docker compose --profile metrics down
```

To fully clean up including volumes:
```bash
make clean
```

---

## Next Steps

1. **Add Metrics:** Implement Prometheus metrics for SLIs (error rate, latency, throughput)
2. **Add Logging:** Correlate logs with trace IDs for unified debugging
3. **Chaos Engineering:** Use tools like Chaos Mesh to inject faults
4. **Load Profiles:** Test different traffic patterns (spike, sustained, gradual)
5. **Tune Patterns:** Adjust timeouts, retry counts, circuit breaker thresholds for your needs

## Questions to Explore

- What happens if you set timeout < retry duration?
- How does the circuit breaker behave with intermittent failures (50% error rate)?
- What's the optimal bulkhead size for your workload?
- How long should idempotency keys be retained?
- When should you use circuit breaker vs. just retries?

Happy exploring! 🚀
