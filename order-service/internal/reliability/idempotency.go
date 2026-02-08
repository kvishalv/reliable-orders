package reliability

import (
	"sync"
	"time"
)

// IdempotencyStore tracks request idempotency keys to prevent duplicate processing
// In production, use Redis or a database for distributed idempotency
// This in-memory implementation is for demo purposes
type IdempotencyStore struct {
	mu      sync.RWMutex
	entries map[string]*IdempotentResponse
}

// IdempotentResponse stores the cached response for an idempotency key
type IdempotentResponse struct {
	OrderID   string
	Status    string
	CreatedAt time.Time
}

// NewIdempotencyStore creates an in-memory idempotency store
func NewIdempotencyStore() *IdempotencyStore {
	store := &IdempotencyStore{
		entries: make(map[string]*IdempotentResponse),
	}

	// Start background cleanup goroutine to prevent memory leaks
	go store.cleanup()

	return store
}

// Get retrieves a cached response for an idempotency key
func (s *IdempotencyStore) Get(key string) (*IdempotentResponse, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resp, exists := s.entries[key]
	return resp, exists
}

// Set stores a response for an idempotency key
func (s *IdempotencyStore) Set(key string, resp *IdempotentResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[key] = resp
}

// cleanup removes entries older than 24 hours to prevent unbounded growth
func (s *IdempotencyStore) cleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		cutoff := time.Now().Add(-24 * time.Hour)
		for key, entry := range s.entries {
			if entry.CreatedAt.Before(cutoff) {
				delete(s.entries, key)
			}
		}
		s.mu.Unlock()
	}
}
