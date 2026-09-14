package main

import (
	"log"
	"net/http"

	"dootask-kpi-server/handlers"
	"dootask-kpi-server/models"
	"dootask-kpi-server/routes"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	// 初始化数据库
	models.InitDB()

	// 创建测试数据
	models.CreateTestData()

	// 设置Gin模式
	gin.SetMode(gin.ReleaseMode)

	// 创建Gin引擎
	r := gin.Default()

	// 配置CORS
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization", "DooTaskAuth"}
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	r.Use(cors.New(config))

	// 基础中间件
	r.Use(handlers.BaseMiddleware())

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "OK",
			"message": "KPI系统后端服务正常运行",
		})
	})

	// 设置路由
	api := r.Group("/api")
	routes.SetupRoutes(api)

	// 启动自动清理任务
	handlers.StartSSECleanupTask()
	handlers.CleanupExportFiles()

	log.Println("KPI系统服务器启动在端口 :8080")
	log.Fatal(r.Run(":8080"))
}
