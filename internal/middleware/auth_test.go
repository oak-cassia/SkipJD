package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"skipjd/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type stubVerifier struct{}

func (v stubVerifier) ParseToken(token string) (*service.AuthClaims, error) {
	if token != "valid-token" {
		return nil, errors.New("invalid token")
	}

	return &service.AuthClaims{
		UserID:    1,
		Email:     "user@example.com",
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, nil
}

func TestRequireAuthRejectsMissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(ErrorHandler())
	r.GET("/auth/me", RequireAuth(stubVerifier{}), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuthPassesValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(ErrorHandler())
	r.GET("/auth/me", RequireAuth(stubVerifier{}), func(c *gin.Context) {
		id, _ := c.Get(ContextUserIDKey)
		email, _ := c.Get(ContextUserEmailKey)
		c.JSON(http.StatusOK, gin.H{
			"id":    id,
			"email": email,
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	assert.NoError(t, err)
	assert.Equal(t, "user@example.com", body["email"])
}
