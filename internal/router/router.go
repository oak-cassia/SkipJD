package router

import (
	"skipjd/internal/handler"
	"skipjd/internal/middleware"
	"skipjd/internal/service"

	"github.com/gin-gonic/gin"
)

func Setup(userHandler *handler.AuthHandler, authService *service.AuthService, signInLimiter gin.HandlerFunc) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.ErrorHandler())

	r.LoadHTMLGlob("web/templates/*")

	pageHandler := handler.NewPageHandler()

	r.GET("/", pageHandler.Index)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	auth := r.Group("/auth")
	{
		auth.POST("/signup", userHandler.SignUp)
		auth.POST("/signin", signInLimiter, userHandler.SignIn)
		auth.GET("/me", middleware.RequireAuth(authService), userHandler.Me)
	}

	return r
}
