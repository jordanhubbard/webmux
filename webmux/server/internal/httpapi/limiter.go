package httpapi

import (
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"
)

const authWindow = 15 * time.Minute
const globalWindow = time.Minute

type bucket struct {
	count int
	reset time.Time
}
type limiter struct {
	mu        sync.Mutex
	entries   map[string]bucket
	limit     int
	window    time.Duration
	now       func() time.Time
	nextPurge time.Time
}

func newLimiter(limit int, window time.Duration) *limiter {
	return &limiter{entries: make(map[string]bucket), limit: limit, window: window, now: time.Now}
}

func (l *limiter) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		// Like Express without trust proxy, only trust the socket peer. Group
		// IPv6 clients by /56 to match express-rate-limit's default behavior.
		key, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			key = r.RemoteAddr
		}
		if address, err := netip.ParseAddr(key); err == nil {
			address = address.Unmap()
			if address.Is6() {
				key = netip.PrefixFrom(address, 56).Masked().String()
			} else {
				key = address.String()
			}
		}
		now := l.now()
		l.mu.Lock()
		if !now.Before(l.nextPurge) {
			for id, entry := range l.entries {
				if !now.Before(entry.reset) {
					delete(l.entries, id)
				}
			}
			l.nextPurge = now.Add(time.Minute)
		}
		entry := l.entries[key]
		if !now.Before(entry.reset) {
			entry = bucket{reset: now.Add(l.window)}
		}
		entry.count++
		l.entries[key] = entry
		l.mu.Unlock()
		remaining := max(0, l.limit-entry.count)
		reset := int(math.Ceil(entry.reset.Sub(now).Seconds()))
		w.Header().Set("RateLimit-Policy", strconv.Itoa(l.limit)+";w="+strconv.Itoa(int(l.window.Seconds())))
		w.Header().Set("RateLimit-Limit", strconv.Itoa(l.limit))
		w.Header().Set("RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("RateLimit-Reset", strconv.Itoa(reset))
		if entry.count > l.limit {
			w.Header().Set("Retry-After", strconv.Itoa(reset))
			writeError(w, 429, "Too many requests, please try again later.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
