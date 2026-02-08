package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

type Stats struct {
	total      int64
	success    int64
	failed     int64
	timeout    int64
	durations  []time.Duration
	statusCode map[int]int64
	mu         sync.Mutex
}

func (s *Stats) recordSuccess(duration time.Duration, statusCode int) {
	atomic.AddInt64(&s.success, 1)
	s.mu.Lock()
	s.durations = append(s.durations, duration)
	s.statusCode[statusCode]++
	s.mu.Unlock()
}

func (s *Stats) recordFailure() {
	atomic.AddInt64(&s.failed, 1)
}

func (s *Stats) recordTimeout() {
	atomic.AddInt64(&s.timeout, 1)
}

func main() {
	targetURL := flag.String("url", "http://localhost:8080/orders", "Target URL")
	concurrency := flag.Int("c", 10, "Number of concurrent requests")
	requests := flag.Int("n", 100, "Total number of requests")
	timeout := flag.Duration("t", 5*time.Second, "Request timeout")
	idempotent := flag.Bool("idempotent", false, "Use idempotency keys")
	flag.Parse()

	fmt.Printf("Load Test Configuration:\n")
	fmt.Printf("  URL: %s\n", *targetURL)
	fmt.Printf("  Concurrency: %d\n", *concurrency)
	fmt.Printf("  Total Requests: %d\n", *requests)
	fmt.Printf("  Timeout: %s\n", *timeout)
	fmt.Printf("  Idempotent: %v\n\n", *idempotent)

	stats := &Stats{
		statusCode: make(map[int]int64),
	}

	client := &http.Client{
		Timeout: *timeout,
	}

	startTime := time.Now()

	// Create worker pool
	jobs := make(chan int, *requests)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				makeRequest(client, *targetURL, *idempotent, stats)
			}
		}()
	}

	// Send jobs
	for i := 0; i < *requests; i++ {
		atomic.AddInt64(&stats.total, 1)
		jobs <- i
	}
	close(jobs)

	// Wait for completion
	wg.Wait()
	duration := time.Since(startTime)

	// Print results
	printResults(stats, duration)
}

func makeRequest(client *http.Client, url string, useIdempotency bool, stats *Stats) {
	// Create request payload
	payload := map[string]interface{}{
		"merchant_id": "merchant_123",
		"amount":      99.99,
		"currency":    "USD",
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		stats.recordFailure()
		return
	}

	req.Header.Set("Content-Type", "application/json")

	// Add idempotency key if enabled
	if useIdempotency {
		req.Header.Set("Idempotency-Key", uuid.New().String())
	}

	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start)

	if err != nil {
		stats.recordTimeout()
		return
	}
	defer resp.Body.Close()

	// Read response body
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		stats.recordSuccess(duration, resp.StatusCode)
	} else {
		stats.recordFailure()
		stats.mu.Lock()
		stats.statusCode[resp.StatusCode]++
		stats.mu.Unlock()
	}
}

func printResults(stats *Stats, totalDuration time.Duration) {
	fmt.Printf("\n=== Load Test Results ===\n\n")
	fmt.Printf("Total Requests:    %d\n", stats.total)
	fmt.Printf("Successful:        %d\n", stats.success)
	fmt.Printf("Failed:            %d\n", stats.failed)
	fmt.Printf("Timeout:           %d\n", stats.timeout)
	fmt.Printf("Total Duration:    %s\n", totalDuration)
	fmt.Printf("Requests/sec:      %.2f\n\n", float64(stats.total)/totalDuration.Seconds())

	if len(stats.durations) > 0 {
		// Calculate latency percentiles
		durations := make([]time.Duration, len(stats.durations))
		copy(durations, stats.durations)

		// Sort durations
		for i := 0; i < len(durations); i++ {
			for j := i + 1; j < len(durations); j++ {
				if durations[i] > durations[j] {
					durations[i], durations[j] = durations[j], durations[i]
				}
			}
		}

		p50 := durations[len(durations)*50/100]
		p95 := durations[len(durations)*95/100]
		p99 := durations[len(durations)*99/100]

		var sum time.Duration
		for _, d := range durations {
			sum += d
		}
		avg := sum / time.Duration(len(durations))

		fmt.Printf("Latency Statistics:\n")
		fmt.Printf("  Average:  %s\n", avg)
		fmt.Printf("  P50:      %s\n", p50)
		fmt.Printf("  P95:      %s\n", p95)
		fmt.Printf("  P99:      %s\n", p99)
		fmt.Printf("  Min:      %s\n", durations[0])
		fmt.Printf("  Max:      %s\n\n", durations[len(durations)-1])
	}

	if len(stats.statusCode) > 0 {
		fmt.Printf("Status Code Distribution:\n")
		for code, count := range stats.statusCode {
			fmt.Printf("  %d: %d (%.1f%%)\n", code, count, float64(count)/float64(stats.total)*100)
		}
	}
}
