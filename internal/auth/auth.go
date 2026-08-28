package auth

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type AttemptLimiter struct {
	mu sync.Mutex
	m  map[string][]time.Time
}

func NewLimiter() *AttemptLimiter { return &AttemptLimiter{m: map[string][]time.Time{}} }
func (l *AttemptLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cut := now.Add(-15 * time.Minute)
	a := l.m[key][:0]
	for _, t := range l.m[key] {
		if t.After(cut) {
			a = append(a, t)
		}
	}
	if len(a) >= 10 {
		l.m[key] = a
		return false
	}
	l.m[key] = append(a, now)
	return true
}

// ClientIP returns the best-effort real client address for the request.
// The direct TCP peer (r.RemoteAddr) is only replaced by a proxy-supplied
// header when that peer's address is explicitly listed in trustedProxies;
// otherwise forwarding headers from untrusted clients could be used to
// spoof audit logs or bypass the login rate limiter.
func ClientIP(r *http.Request, trustedProxies []string) string {
	direct := directIP(r.RemoteAddr)
	if direct == "" || !ipInList(direct, trustedProxies) {
		if direct != "" {
			return direct
		}
		return r.RemoteAddr
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if candidate := strings.TrimSpace(parts[0]); candidate != "" {
			return candidate
		}
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return strings.TrimSpace(xr)
	}
	return direct
}

func directIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// RemoteAddr without a port (rare, but possible in tests).
		if net.ParseIP(remoteAddr) != nil {
			return remoteAddr
		}
		return ""
	}
	return host
}

func ipInList(ip string, list []string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, entry := range list {
		if entry == ip {
			return true
		}
		if _, cidr, err := net.ParseCIDR(entry); err == nil && cidr.Contains(parsed) {
			return true
		}
	}
	return false
}
