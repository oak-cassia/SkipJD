package handler

import (
	"context"
	"net/http"
	"skipjd/internal/errs"
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
		c.Error(errs.InvalidRequest)
		return
	}

	result, err := h.authService.SignUp(ctx, req.Email, req.Password, req.Name)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, toAuthResponse(result))
}

func (h *AuthHandler) SignIn(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), authRequestTimeout)
	defer cancel()

	var req signInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errs.InvalidRequest)
		return
	}

	result, err := h.authService.SignIn(ctx, req.Email, req.Password)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, toAuthResponse(result))
}
