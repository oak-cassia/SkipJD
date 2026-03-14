package router

import (
	"skipjd/internal/handler"

	"github.com/gin-gonic/gin"
)

func Setup(userHandler *handler.UserHandler) *gin.Engine {
	r := gin.Default()

	r.LoadHTMLGlob("web/templates/*")

	pageHandler := handler.NewPageHandler()

	r.GET("/", pageHandler.Index)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	_ = userHandler

	return r
}
