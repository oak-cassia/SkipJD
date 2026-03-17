package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type rateLimitEntry struct {
	count       int
	windowStart time.Time
}

func NewIPRateLimit(limit int, window time.Duration) gin.HandlerFunc {
	if limit <= 0 || window <= 0 {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	var (
		mu      sync.Mutex
		entries = make(map[string]rateLimitEntry)
	)

	return func(c *gin.Context) {
		now := time.Now()
		fullPath := c.FullPath()
		if fullPath == "" {
			fullPath = c.Request.URL.Path
		}
		key := c.ClientIP() + ":" + fullPath

		mu.Lock()
		entry := entries[key]
		if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= window {
			entry = rateLimitEntry{windowStart: now}
		}
		entry.count++
		entries[key] = entry
		blocked := entry.count > limit
		retryAfter := int(window.Seconds()) - int(now.Sub(entry.windowStart).Seconds())
		mu.Unlock()

		if !blocked {
			c.Next()
			return
		}

		if retryAfter < 1 {
			retryAfter = 1
		}
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error": "too many requests",
		})
	}
}
