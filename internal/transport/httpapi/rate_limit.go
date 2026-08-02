package httpapi

import (
	"net"
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
	now     func() time.Time
}

type limitEntry struct {
	count int
	reset time.Time
}

func newRequestLimiter(limit int, window time.Duration) *requestLimiter {
	return &requestLimiter{entries: make(map[string]limitEntry), limit: limit, window: window, now: time.Now}
}

func (limiter *requestLimiter) middleware(next vial.Handler) vial.Handler {
	return func(context *vial.Context) error {
		now := limiter.now()
		key := clientIP(context.Request().RemoteAddr) + " " + context.Request().URL.Path

		limiter.mu.Lock()
		// ponytail: one in-process window is enough for one API replica; move counters to Redis before horizontal scaling.
		for currentKey, entry := range limiter.entries {
			if !now.Before(entry.reset) {
				delete(limiter.entries, currentKey)
			}
		}
		entry := limiter.entries[key]
		if entry.reset.IsZero() {
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

func clientIP(remoteAddress string) string {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err == nil {
		return host
	}
	return remoteAddress
}
