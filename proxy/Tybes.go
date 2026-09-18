package proxy

import (
	"sync"
	"sync/atomic"
	"time"
)

type ProxyHandler struct {
	Pool *Pool
	RateLimiter *RateLimiter
}
type TokenBucket struct{
CurrentTokens float64 
LastRefillTime time.Time
MaxTokens float64
RefillRate float64 
mu sync.Mutex
}
type Backend struct {
	URL             string
	mu              sync.RWMutex
	alive           bool
	state           string // "Closed", "Open", "Half-Open"
	failCount       int
	LiveConnections int64
}
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*TokenBucket
	maxTokens  float64
	refillRate float64
}

func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// 1. Refill first — calculate how many tokens should have
	//    accumulated since the last time we touched this bucket.
	now := time.Now()
	elapsed := now.Sub(tb.LastRefillTime)
	tokensToAdd := elapsed.Seconds() * tb.RefillRate

	tb.CurrentTokens = min(tb.MaxTokens, tb.CurrentTokens+tokensToAdd)
	tb.LastRefillTime = now

	// 2. Check — is there at least one full token available?
	if tb.CurrentTokens < 1 {
		return false
	}

	// 3. Decrement and allow.
	tb.CurrentTokens--
	return true
}

func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		buckets:    make(map[string]*TokenBucket),
		maxTokens:  cfg.MaxTokens,
		refillRate: cfg.RefillRate,
	}
}

func (rl *RateLimiter) getBucket(clientIP string) *TokenBucket {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, exists := rl.buckets[clientIP]
	if !exists {
		bucket = &TokenBucket{
			CurrentTokens:  rl.maxTokens,
			MaxTokens:      rl.maxTokens,
			RefillRate:     rl.refillRate,
			LastRefillTime: time.Now(),
		}
		rl.buckets[clientIP] = bucket
	}
	return bucket
}

func (rl *RateLimiter) Allow(clientIP string) bool {
	bucket := rl.getBucket(clientIP)
	return bucket.Allow()
}
// --- Alive ---

func (b *Backend) SetAlive(alive bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.alive = alive
}

func (b *Backend) IsAlive() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	// a backend is only usable if it's marked alive AND the breaker isn't open
	return b.alive && b.state != "Open"
}

// --- Connection tracking (unchanged) ---

func (b *Backend) AddLiveConnections() {
	atomic.AddInt64(&b.LiveConnections, 1)
}

func (b *Backend) DelLiveConnection() {
	atomic.AddInt64(&b.LiveConnections, -1)
}

// --- Circuit breaker ---

const failThreshold = 4

// RecordSuccess resets the breaker back to normal.
func (b *Backend) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failCount = 0
	b.state = "Closed"
}

// RecordFailure counts a failure and trips the breaker once the threshold is hit.
func (b *Backend) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failCount++
	if b.failCount >= failThreshold {
		b.state = "Open"
	}
}

func (b *Backend) State() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

type Pool struct {
	Backends []*Backend
	Strategy Strategy
}

func (p *Pool) NextBackend() *Backend {
	return p.pick(nil)
}

func (p *Pool) NextBackendExcluding(exclude *Backend) *Backend {
	return p.pick(exclude)
}

func (p *Pool) pick(exclude *Backend) *Backend {
	var live []*Backend
	for _, b := range p.Backends {
		if b == exclude {
			continue
		}
		if b.IsAlive() {
			live = append(live, b)
		}
	}
	if len(live) == 0 {
		return nil
	}
	return p.Strategy.Pick(live)
}

type Strategy interface {
	Pick(backends []*Backend) *Backend
}

type LeastConnections struct{}

func (lc LeastConnections) Pick(backends []*Backend) *Backend {
	best := backends[0]
	for i := range backends {
		if backends[i].LiveConnections < best.LiveConnections {
			best = backends[i]
		}
	}
	return best
}

type RoundRobin struct {
	counter uint64
}

func (rr *RoundRobin) Pick(backends []*Backend) *Backend {
	n := atomic.AddUint64(&rr.counter, 1)
	idx := n % uint64(len(backends))
	return backends[idx]
} 