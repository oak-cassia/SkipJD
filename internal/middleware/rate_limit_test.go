package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestNewIPRateLimitBlocksAfterLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.POST("/auth/signin", NewIPRateLimit(2, time.Minute), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	perform := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/auth/signin", nil)
		req.RemoteAddr = "203.0.113.10:34567"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	first := perform()
	second := perform()
	third := perform()

	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, http.StatusOK, second.Code)
	assert.Equal(t, http.StatusTooManyRequests, third.Code)
	assert.NotEmpty(t, third.Header().Get("Retry-After"))
}
