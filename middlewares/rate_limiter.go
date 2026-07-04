package middleware

import (
	"fmt"
	"sync"
	"time"

	"github.com/nelthaarion/breeze"
)

type clientData struct {
	lastRequest time.Time
	requests    int
}

// RateLimiterOptions defines the configuration for the middleware.
type RateLimiterOptions struct {
	Requests int           // allowed requests
	Per      time.Duration // per duration
	Message  string        // optional message on limit
}

// RateLimiter holds the per-client counters and the pre-formatted limit
// message so the hot path never calls fmt.Sprintf.
type RateLimiter struct {
	options  RateLimiterOptions
	clients  map[string]*clientData
	mu       sync.Mutex
	limitMsg string // FIX: pre-computed to avoid fmt.Sprintf on every 429
}

// NewRateLimiter returns a rate limiting middleware.
//
// FIX: The original code held mu.Lock() across ctx.Next(), serializing every
// request and completely defeating the WorkerPool. The lock is now released
// before ctx.Next() — it is held only for the map lookup and counter update
// (microseconds).
//
// FIX: The limit message is pre-computed at construction time so the 429
// path does not call fmt.Sprintf on every rejected request.
func NewRateLimiter(opts RateLimiterOptions) breeze.HandlerFunc {
	rl := &RateLimiter{
		options: opts,
		clients: make(map[string]*clientData),
	}

	// Pre-compute the 429 message once.
	if opts.Message == "" {
		rl.limitMsg = fmt.Sprintf("Rate limit exceeded: max %d requests per %s",
			opts.Requests, opts.Per)
	} else {
		rl.limitMsg = opts.Message
	}

	return func(ctx *breeze.Context) {
		// Use IP as key (Conn.RemoteAddr).
		clientIP := ctx.Conn.RemoteAddr().String()

		// ── Critical section: map lookup + counter update only ──────────
		// The lock is held for microseconds, never across ctx.Next().
		rl.mu.Lock()
		now := time.Now()
		data, exists := rl.clients[clientIP]
		if !exists {
			data = &clientData{lastRequest: now, requests: 1}
			rl.clients[clientIP] = data
		} else {
			if now.Sub(data.lastRequest) > rl.options.Per {
				data.requests = 1
				data.lastRequest = now
			} else {
				data.requests++
			}
		}
		exceeded := data.requests > rl.options.Requests
		rl.mu.Unlock()
		// ── End critical section ─────────────────────────────────────────

		if exceeded {
			ctx.Status(429)
			ctx.WriteString(rl.limitMsg)
			return
		}

		// Handler runs lock-free — the WorkerPool can fully parallelize.
		ctx.Next()
	}
}
