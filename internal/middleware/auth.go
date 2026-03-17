package middleware

import (
	"skipjd/internal/errs"
	"skipjd/internal/service"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	ContextUserIDKey    = "authUserID"
	ContextUserEmailKey = "authUserEmail"
)

type tokenVerifier interface {
	ParseToken(token string) (*service.AuthClaims, error)
}

func RequireAuth(verifier tokenVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := extractBearerToken(c.GetHeader("Authorization"))
		if !ok {
			c.Error(errs.InvalidToken)
			c.Abort()
			return
		}

		claims, err := verifier.ParseToken(token)
		if err != nil {
			c.Error(errs.InvalidToken)
			c.Abort()
			return
		}

		c.Set(ContextUserIDKey, claims.UserID)
		c.Set(ContextUserEmailKey, claims.Email)
		c.Next()
	}
}

func extractBearerToken(authHeader string) (string, bool) {
	parts := strings.Fields(authHeader)
	if len(parts) != 2 {
		return "", false
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	if strings.TrimSpace(parts[1]) == "" {
		return "", false
	}

	return parts[1], true
}
