package handler

import "github.com/gin-gonic/gin"

type PageHandler struct{}

func NewPageHandler() *PageHandler {
	return &PageHandler{}
}

func (h *PageHandler) Index(c *gin.Context) {
	c.HTML(200, "index.html", gin.H{
		"title": "SkipJD",
	})
}
