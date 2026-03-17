package middleware

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"skipjd/internal/domain/auth"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
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
	switch {
	case isBadRequestError(err):
		return http.StatusBadRequest, "invalid request body"
	case errors.Is(err, auth.ErrEmailAlreadyExists):
		return http.StatusConflict, "email already exists"
	case errors.Is(err, auth.ErrInvalidCredentials):
		return http.StatusUnauthorized, "invalid email or password"
	default:
		return http.StatusInternalServerError, "internal server error"
	}
}

func isBadRequestError(err error) bool {
	var validationErrs validator.ValidationErrors
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError

	return errors.As(err, &validationErrs) ||
		errors.As(err, &syntaxErr) ||
		errors.As(err, &typeErr) ||
		errors.Is(err, io.EOF)
}
