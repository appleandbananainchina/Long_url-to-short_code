package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type MyLimiter struct {
	*rate.Limiter           // 嵌入，继承所有方法
	lastAccess    time.Time // 每个IP独立的最后访问时间
	mu            sync.RWMutex
}

type IPRateLimiter struct {
	ips      map[string]*MyLimiter
	rate     rate.Limit
	burst    int
	cleanupT time.Duration
	mu       sync.RWMutex
}

func NewIPRateLimiter(r rate.Limit, b int, cleanupInterval time.Duration) *IPRateLimiter {
	limiter := &IPRateLimiter{
		ips:      make(map[string]*MyLimiter),
		rate:     r,
		burst:    b,
		cleanupT: cleanupInterval,
	}
	go limiter.cleanupExpired()

	return limiter
}

func (i *IPRateLimiter) getLimiter(ip string) *MyLimiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	limiter, exists := i.ips[ip]
	if !exists {
		limiter = &MyLimiter{
			Limiter:    rate.NewLimiter(i.rate, i.burst),
			lastAccess: time.Now(),
		}
		i.ips[ip] = limiter
	}
	return limiter
}

func (i *IPRateLimiter) Allow(ip string) bool {
	limiter := i.getLimiter(ip)
	limiter.updateAccess()
	return limiter.Limiter.Allow()
}

func (i *IPRateLimiter) cleanupExpired() {
	ticker := time.NewTicker(i.cleanupT)
	defer ticker.Stop()
	for range ticker.C {
		i.mu.Lock()
		for ip, limiter := range i.ips {
			if time.Since(limiter.lastAccessTime()) > i.cleanupT*2 {
				delete(i.ips, ip)
			}
		}
		i.mu.Unlock()
	}
}

func (i *IPRateLimiter) RateLimitMiddleWare(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getRealIp(r)
		if !i.Allow(ip) {
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(i.burst))
			w.Header().Set("Retry-After", "1")
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func getRealIp(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := splitAndTrim(xff)
		for _, ip := range ips {
			if ip != "" {
				return ip
			}
		}
	}
	if xri := r.Header.Get("X-Real-Ip"); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func splitAndTrim(s string) []string {
	ips := strings.Split(s, ",")
	for i := range ips {
		ips[i] = strings.TrimSpace(ips[i])
	}
	return ips
}

func (limiter *MyLimiter) updateAccess() {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	limiter.lastAccess = time.Now()
}

func (limiter *MyLimiter) lastAccessTime() time.Time {
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()
	return limiter.lastAccess
}
