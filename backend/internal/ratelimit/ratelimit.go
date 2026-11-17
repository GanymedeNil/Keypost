// Package ratelimit implements a small in-memory fixed-window limiter.
package ratelimit

import (
	"sync"
	"time"
)

type win struct {
	start time.Time
	n     int
}

type Limiter struct {
	mu     sync.Mutex
	m      map[string]*win
	window time.Duration
	last   time.Time
}

func New(window time.Duration) *Limiter {
	return &Limiter{m: map[string]*win{}, window: window}
}

// Allow records one hit for key and reports whether it is within limit.
// remaining is the number of calls left in the current window.
func (l *Limiter) Allow(key string, limit int) (ok bool, remaining int, reset time.Duration) {
	if limit <= 0 {
		return true, 1 << 30, 0
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.last) > 5*time.Minute {
		for k, w := range l.m {
			if now.Sub(w.start) > l.window {
				delete(l.m, k)
			}
		}
		l.last = now
	}
	w, found := l.m[key]
	if !found || now.Sub(w.start) >= l.window {
		w = &win{start: now}
		l.m[key] = w
	}
	reset = w.start.Add(l.window).Sub(now)
	if w.n >= limit {
		return false, 0, reset
	}
	w.n++
	return true, limit - w.n, reset
}
