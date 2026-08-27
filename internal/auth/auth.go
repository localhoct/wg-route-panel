package auth

import (
	"database/sql"
	"net/http"
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
func ClientIP(r *http.Request) string {
	h, _, e := netSplit(r.RemoteAddr)
	if e == nil {
		return h
	}
	return r.RemoteAddr
}
func netSplit(s string) (string, string, error) { return splitHostPort(s) }

var splitHostPort = func(s string) (string, string, error) { return netSplitHostPort(s) }

func netSplitHostPort(s string) (string, string, error) {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return s[:i], s[i+1:], nil
		}
	}
	return "", "", sql.ErrNoRows
}
