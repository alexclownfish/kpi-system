package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"dootask-kpi-server/handlers"
	"dootask-kpi-server/models"
	"dootask-kpi-server/routes"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := checkHealth(); err != nil {
			log.Print(err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "init-schema" {
		if strings.ToLower(strings.TrimSpace(os.Getenv("DB_DRIVER"))) != "postgres" {
			log.Fatal("init-schema 需要 DB_DRIVER=postgres")
		}
		if err := models.InitSchema(); err != nil {
			log.Fatal("PostgreSQL 数据表初始化失败: ", err)
		}
		log.Print("PostgreSQL 数据表初始化完成；未写入业务种子数据")
		return
	}

	if err := handlers.InitAuthConfig(); err != nil {
		log.Fatal("认证配置无效: ", err)
	}
	allowedOrigins := splitCSV(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if len(allowedOrigins) == 0 && strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production") {
		log.Fatal("生产环境必须设置 CORS_ALLOWED_ORIGINS")
	}

	// 初始化数据库
	models.InitDB()
	if err := models.EnsureSystemDefaults(); err != nil {
		log.Fatal("系统默认配置初始化失败: ", err)
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("SEED_DEMO_DATA")), "true") {
		log.Print("SEED_DEMO_DATA 已弃用，不再创建演示账号或模板")
	}
	if err := handlers.SeedHongxingCatalog(models.DB); err != nil {
		log.Fatal("荭杏业务目录初始化失败: ", err)
	}
	if err := models.BootstrapSuperAdmin(); err != nil {
		log.Fatal("超级管理员初始化失败: ", err)
	}
	if len(os.Args) > 1 && os.Args[1] == "init-business" {
		log.Print("业务初始化完成")
		return
	}

	// 设置Gin模式
	gin.SetMode(gin.ReleaseMode)

	// 创建Gin引擎
	r := gin.Default()

	// 配置CORS
	config := cors.DefaultConfig()
	if len(allowedOrigins) == 0 {
		config.AllowAllOrigins = true
	} else {
		config.AllowOrigins = allowedOrigins
	}
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

func checkHealth() error {
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Get("http://127.0.0.1:8080/health")
	if err != nil {
		return fmt.Errorf("健康检查请求失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("健康检查返回状态码 %d", response.StatusCode)
	}
	return nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			result = append(result, item)
		}
	}
	return result
}
