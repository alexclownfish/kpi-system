package handlers

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// BaseMiddleware 基础中间件
// 设置基础地址
func BaseMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 基础地址
		scheme := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		host := c.GetHeader("X-Forwarded-Host")
		if host == "" {
			host = c.Request.Host
		}
		c.Set("base_url", fmt.Sprintf("%s://%s", scheme, host))

		c.Next()
	}
}
