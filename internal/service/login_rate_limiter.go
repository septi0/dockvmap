package service

import (
	"net/netip"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/septi0/dockvmap/internal/ipmatch"
)

type LoginRateLimiter struct {
	mu          sync.Mutex
	enabled     bool
	maxAttempts int
	bypass      ipmatch.Set
	attempts    *expirable.LRU[string, int]
}

const loginRateLimiterMaxTrackedIPs = 10_000

func NewLoginRateLimiter(enabled bool, maxAttempts int, window time.Duration, bypassIPs []string) (*LoginRateLimiter, error) {
	bypass, err := ipmatch.Parse(bypassIPs)

	if err != nil {
		return nil, err
	}

	var attempts *expirable.LRU[string, int]

	if enabled {
		attempts = expirable.NewLRU[string, int](loginRateLimiterMaxTrackedIPs, nil, window)
	}

	return &LoginRateLimiter{
		enabled:     enabled,
		maxAttempts: maxAttempts,
		bypass:      bypass,
		attempts:    attempts,
	}, nil
}

func (l *LoginRateLimiter) Reserve(ip string) bool {
	if !l.enabled || l.bypassed(ip) {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	count, _ := l.attempts.Get(ip)

	if count >= l.maxAttempts {
		return false
	}

	l.attempts.Add(ip, count+1)

	return true
}

func (l *LoginRateLimiter) RecordSuccess(ip string) {
	if !l.enabled {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.attempts.Remove(ip)
}

func (l *LoginRateLimiter) bypassed(ip string) bool {
	if l.bypass.Empty() {
		return false
	}

	addr, err := netip.ParseAddr(ip)

	if err != nil {
		return false
	}

	return l.bypass.Contains(addr)
}
