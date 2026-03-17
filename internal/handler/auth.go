package handler

import (
	"context"
	"net/http"
	"skipjd/internal/errs"
	"skipjd/internal/middleware"
	"skipjd/internal/service"
	"time"

	"github.com/gin-gonic/gin"
)

const authRequestTimeout = 5 * time.Second

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) SignUp(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), authRequestTimeout)
	defer cancel()

	var req signUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(errs.InvalidRequest)
		return
	}

	result, err := h.authService.SignUp(ctx, req.Email, req.Password, req.Name)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, toAuthResponse(result))
}

func (h *AuthHandler) SignIn(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), authRequestTimeout)
	defer cancel()

	var req signInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(errs.InvalidRequest)
		return
	}

	result, err := h.authService.SignIn(ctx, req.Email, req.Password)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, toAuthResponse(result))
}

func (h *AuthHandler) Me(c *gin.Context) {
	userIDRaw, ok := c.Get(middleware.ContextUserIDKey)
	if !ok {
		c.Error(errs.InvalidToken)
		return
	}
	userID, ok := userIDRaw.(uint)
	if !ok {
		c.Error(errs.InvalidToken)
		return
	}

	emailRaw, ok := c.Get(middleware.ContextUserEmailKey)
	if !ok {
		c.Error(errs.InvalidToken)
		return
	}
	email, ok := emailRaw.(string)
	if !ok {
		c.Error(errs.InvalidToken)
		return
	}

	c.JSON(http.StatusOK, meResponse{
		ID:    userID,
		Email: email,
	})
}
