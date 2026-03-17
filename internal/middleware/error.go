package middleware

import (
	"context"
	"errors"
	"net/http"
	"skipjd/internal/errs"

	"github.com/gin-gonic/gin"
)

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 {
			return
		}

		status, message := classifyError(c.Errors.Last().Err)
		if !c.Writer.Written() {
			c.JSON(status, gin.H{"error": message})
		}
	}
}

func classifyError(err error) (int, string) {
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, "request timeout"
	}

	var code errs.Code
	if !errors.As(err, &code) {
		code = errs.InternalServerError
	}

	switch code {
	case errs.InvalidRequest:
		return http.StatusBadRequest, "invalid request body"
	case errs.EmailAlreadyExists:
		return http.StatusConflict, "email already exists"
	case errs.InvalidCredentials:
		return http.StatusUnauthorized, "invalid email or password"
	default:
		return http.StatusInternalServerError, "internal server error"
	}
}
