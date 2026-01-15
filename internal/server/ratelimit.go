package server

import (
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter tracks failed authentication attempts and blocks IPs
type RateLimiter struct {
	mu          sync.RWMutex
	blockedIPs  map[string]time.Time
	blockPeriod time.Duration
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(blockPeriod time.Duration) *RateLimiter {
	rl := &RateLimiter{
		blockedIPs:  make(map[string]time.Time),
		blockPeriod: blockPeriod,
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// IsBlocked checks if an IP is currently blocked
func (rl *RateLimiter) IsBlocked(ip string) bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	blockedUntil, exists := rl.blockedIPs[ip]
	if !exists {
		return false
	}

	return time.Now().Before(blockedUntil)
}

// BlockIP blocks an IP for the configured period
func (rl *RateLimiter) BlockIP(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.blockedIPs[ip] = time.Now().Add(rl.blockPeriod)
}

// cleanup periodically removes expired blocks
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, blockedUntil := range rl.blockedIPs {
			if now.After(blockedUntil) {
				delete(rl.blockedIPs, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// GetRealIP extracts the real client IP from a request, handling proxies and Cloudflare
func GetRealIP(c *gin.Context) string {
	// Priority order for IP detection:
	// 1. CF-Connecting-IP (Cloudflare)
	// 2. True-Client-IP (Cloudflare Enterprise)
	// 3. X-Real-IP (nginx)
	// 4. X-Forwarded-For (standard proxy header, first IP)
	// 5. RemoteAddr (direct connection)

	// Cloudflare headers
	if ip := c.GetHeader("CF-Connecting-IP"); ip != "" {
		if parsedIP := parseIP(ip); parsedIP != "" {
			return parsedIP
		}
	}

	if ip := c.GetHeader("True-Client-IP"); ip != "" {
		if parsedIP := parseIP(ip); parsedIP != "" {
			return parsedIP
		}
	}

	// Standard proxy headers
	if ip := c.GetHeader("X-Real-IP"); ip != "" {
		if parsedIP := parseIP(ip); parsedIP != "" {
			return parsedIP
		}
	}

	// X-Forwarded-For can contain multiple IPs: client, proxy1, proxy2, ...
	// The first IP is typically the real client
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			if parsedIP := parseIP(strings.TrimSpace(ips[0])); parsedIP != "" {
				return parsedIP
			}
		}
	}

	// Fallback to direct connection
	return parseIP(c.ClientIP())
}

// parseIP validates and extracts an IP address, stripping port if present
func parseIP(ipStr string) string {
	ipStr = strings.TrimSpace(ipStr)
	if ipStr == "" {
		return ""
	}

	// Try parsing as IP:port first
	if host, _, err := net.SplitHostPort(ipStr); err == nil {
		ipStr = host
	}

	// Validate it's a proper IP
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return ""
	}

	return ip.String()
}

// LogFailedAuth logs a failed authentication attempt
func LogFailedAuth(ip, reason string, blocked bool) {
	status := "FAILED"
	if blocked {
		status = "BLOCKED"
	}
	log.Printf("[AUTH %s] ip=%s reason=%q", status, ip, reason)
}
