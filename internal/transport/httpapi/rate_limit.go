package httpapi

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	vial "github.com/jrgf/go-vial"
)

type requestLimiter struct {
	mu      sync.Mutex
	entries map[string]limitEntry
	limit   int
	window  time.Duration
	maxSize int
	now     func() time.Time
}

type limitEntry struct {
	count int
	reset time.Time
}

func newRequestLimiter(limit int, window time.Duration) *requestLimiter {
	return &requestLimiter{entries: make(map[string]limitEntry), limit: limit, window: window, maxSize: 10_000, now: time.Now}
}

func (limiter *requestLimiter) middleware(next vial.Handler) vial.Handler {
	return func(context *vial.Context) error {
		now := limiter.now()
		address, err := context.ClientIP()
		if err != nil {
			return vial.NewHTTPError(http.StatusBadRequest, "invalidClientIP", "Invalid client address")
		}
		key := address.String() + " " + context.Request().URL.Path

		limiter.mu.Lock()
		entry := limiter.entries[key]
		if !entry.reset.IsZero() && !now.Before(entry.reset) {
			delete(limiter.entries, key)
			entry = limitEntry{}
		}
		if entry.reset.IsZero() {
			if len(limiter.entries) >= limiter.maxSize {
				// ponytail: arbitrary eviction keeps one-process memory bounded; use a shared limiter before horizontal scaling.
				for currentKey := range limiter.entries {
					delete(limiter.entries, currentKey)
					break
				}
			}
			entry.reset = now.Add(limiter.window)
		}
		limited := entry.count >= limiter.limit
		if !limited {
			entry.count++
			limiter.entries[key] = entry
		}
		limiter.mu.Unlock()

		if limited {
			retryAfter := max(1, int((entry.reset.Sub(now)+time.Second-1)/time.Second))
			context.Response().Header().Set("Retry-After", strconv.Itoa(retryAfter))
			return vial.NewHTTPError(http.StatusTooManyRequests, "rateLimited", "Too many authentication attempts; try again later")
		}
		return next(context)
	}
}
