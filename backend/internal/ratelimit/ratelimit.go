package ratelimit

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

type Limiter struct {
	mu              sync.Mutex
	ipCounts        map[string]int
	ipBrowserCounts map[string]int
	maxPerIP        int
	maxPerIPBrowser int
}

func New(maxPerIPBrowser, maxPerIP int) *Limiter {
	l := &Limiter{
		ipCounts:        make(map[string]int),
		ipBrowserCounts: make(map[string]int),
		maxPerIP:        maxPerIP,
		maxPerIPBrowser: maxPerIPBrowser,
	}
	return l
}

func (l *Limiter) StartCleanup(window time.Duration) {
	go func() {
		ticker := time.NewTicker(window)
		defer ticker.Stop()
		for range ticker.C {
			l.mu.Lock()
			l.ipCounts = make(map[string]int)
			l.ipBrowserCounts = make(map[string]int)
			l.mu.Unlock()
		}
	}()
}

func (l *Limiter) Allow(ip, browserID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.ipCounts[ip] >= l.maxPerIP {
		return false
	}

	key := hashKey(ip, browserID)
	if l.ipBrowserCounts[key] >= l.maxPerIPBrowser {
		return false
	}

	l.ipCounts[ip]++
	l.ipBrowserCounts[key]++
	return true
}

func hashKey(ip, browserID string) string {
	h := sha256.Sum256([]byte(ip + "|" + browserID))
	return fmt.Sprintf("%x", h[:16])
}
