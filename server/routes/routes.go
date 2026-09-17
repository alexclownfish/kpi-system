package routes

import (
	"dootask-kpi-server/handlers"
	"net/http"

	"github.com/gin-gonic/gin"
)

// 设置所有路由
func SetupRoutes(r *gin.RouterGroup) {

	// 健康检查（公开）
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "OK",
			"message": "KPI系统后端服务正常运行",
		})
	})

	// 内部系统 Hook（公开，仅用于应用内部调用）
	hookRoutes := r.Group("/hooks")
	{
		hookRoutes.POST("/user/onboard", handlers.SystemUserOnboard)
		hookRoutes.POST("/user/offboard", handlers.SystemUserOffboard)
	}

	// 认证路由（公开）
	publicRoutes := r.Group("/auth")
	{
		publicRoutes.POST("/register", handlers.Register)
		publicRoutes.POST("/login", handlers.Login)
		publicRoutes.POST("/login-by-dootask-token", handlers.LoginByDooTaskToken)
		publicRoutes.POST("/refresh", handlers.RefreshToken)
		publicRoutes.GET("/departments", handlers.GetDepartments) // 注册时需要获取部门列表
	}

	// 系统设置（公开，部分需要HR权限）
	settingsRoutes := r.Group("/settings")
	{
		settingsRoutes.GET("", handlers.GetSystemSettings) // 所有用户可以读取设置
		settingsRoutes.PUT("", handlers.AuthMiddleware(), handlers.PermissionMiddleware("system:edit"), handlers.UpdateSystemSettings)
	}

	// 文件下载（公开）
	downloadRoutes := r.Group("/download")
	{
		downloadRoutes.GET("/exports/:randomKey", handlers.DownloadFile)
		downloadRoutes.GET("/backups/:randomKey", handlers.DownloadBackup)
	}

	// SSE事件流（所有认证用户）
	sseRoutes := r.Group("/events")
	{
		sseRoutes.GET("/stream", handlers.SSEHandler)                              // SSE事件流，通过URL参数token认证
		sseRoutes.GET("/status", handlers.AuthMiddleware(), handlers.GetSSEStatus) // 获取连接状态，所有认证用户
	}

	// 需要认证的路由
	protected := r.Group("/")
	protected.Use(handlers.AuthMiddleware())
	{
		// 当前用户信息
		protected.GET("/me", handlers.GetCurrentUser)
		protected.GET("/roles", handlers.PermissionAnyMiddleware("employee:assign_role", "role:view"), handlers.GetRoles)
		protected.GET("/roles/:id", handlers.PermissionMiddleware("role:view"), handlers.GetRole)
		protected.POST("/roles", handlers.PermissionMiddleware("role:create"), handlers.CreateRole)
		protected.POST("/roles/:id/clone", handlers.PermissionMiddleware("role:create"), handlers.CloneRole)
		protected.PUT("/roles/:id", handlers.PermissionMiddleware("role:edit"), handlers.UpdateRole)
		protected.DELETE("/roles/:id", handlers.PermissionMiddleware("role:delete"), handlers.DeleteRole)
		protected.GET("/roles/:id/users", handlers.PermissionMiddleware("role:view"), handlers.GetRoleUsers)
		protected.PUT("/roles/:id/users", handlers.PermissionMiddleware("employee:assign_role"), handlers.AssignRoleUsers)
		protected.GET("/permissions", handlers.PermissionMiddleware("role:view"), handlers.GetPermissions)
		protected.GET("/data-scopes", handlers.PermissionMiddleware("role:view"), handlers.GetDataScopes)

		// 部门管理（HR和管理员）
		departmentRoutes := protected.Group("/departments")
		{
			departmentRoutes.GET("", handlers.PermissionMiddleware("department:view"), handlers.GetDepartments)
			departmentRoutes.POST("", handlers.PermissionMiddleware("department:create"), handlers.CreateDepartment)
			departmentRoutes.GET("/:id", handlers.GetDepartment)
			departmentRoutes.PUT("/:id", handlers.PermissionMiddleware("department:edit"), handlers.UpdateDepartment)
			departmentRoutes.DELETE("/:id", handlers.PermissionMiddleware("department:delete"), handlers.DeleteDepartment)
		}

		// 员工管理
		employeeRoutes := protected.Group("/employees")
		{
			employeeRoutes.GET("", handlers.PermissionMiddleware("employee:view"), handlers.GetEmployees)
			employeeRoutes.POST("", handlers.PermissionMiddleware("employee:create"), handlers.CreateEmployee)
			employeeRoutes.GET("/:id", handlers.PermissionMiddleware("employee:view"), handlers.GetEmployee)
			employeeRoutes.PUT("/:id", handlers.PermissionMiddleware("employee:edit"), handlers.UpdateEmployee)
			employeeRoutes.PUT("/:id/roles", handlers.PermissionMiddleware("employee:assign_role"), handlers.AssignEmployeeRole)
			employeeRoutes.DELETE("/:id", handlers.PermissionMiddleware("employee:delete"), handlers.DeleteEmployee)
			employeeRoutes.GET("/:id/subordinates", handlers.PermissionMiddleware("employee:view"), handlers.GetEmployeeSubordinates)
		}

		// KPI模板管理（HR和管理员）
		templateRoutes := protected.Group("/templates")
		{
			templateRoutes.GET("", handlers.PermissionMiddleware("kpi:view"), handlers.GetTemplates)
			templateRoutes.POST("", handlers.PermissionMiddleware("kpi:create"), handlers.CreateTemplate)
			templateRoutes.GET("/:id", handlers.PermissionMiddleware("kpi:view"), handlers.GetTemplate)
			templateRoutes.PUT("/:id", handlers.PermissionMiddleware("kpi:edit"), handlers.UpdateTemplate)
			templateRoutes.DELETE("/:id", handlers.PermissionMiddleware("kpi:delete"), handlers.DeleteTemplate)
			templateRoutes.GET("/:id/items", handlers.GetTemplateItems)
		}

		// 绩效规则管理（仅HR）
		performanceRuleRoutes := protected.Group("/performance-rules")
		{
			performanceRuleRoutes.GET("", handlers.PermissionMiddleware("kpi:view"), handlers.GetPerformanceRule)
			performanceRuleRoutes.PUT("", handlers.PermissionMiddleware("kpi:edit"), handlers.UpdatePerformanceRule)
		}

		// KPI考核项目管理（HR和管理员）
		itemRoutes := protected.Group("/items")
		{
			itemRoutes.POST("", handlers.PermissionMiddleware("kpi:create"), handlers.CreateItem)
			itemRoutes.GET("/:id", handlers.GetItem)
			itemRoutes.PUT("/:id", handlers.PermissionMiddleware("kpi:edit"), handlers.UpdateItem)
			itemRoutes.DELETE("/:id", handlers.PermissionMiddleware("kpi:delete"), handlers.DeleteItem)
		}

		// KPI评估管理
		evaluationRoutes := protected.Group("/evaluations")
		{
			evaluationRoutes.GET("", handlers.PermissionMiddleware("assessment:view"), handlers.GetEvaluations)
			evaluationRoutes.POST("", handlers.PermissionMiddleware("assessment:create"), handlers.CreateEvaluation)
			evaluationRoutes.GET("/:id", handlers.PermissionMiddleware("assessment:view"), handlers.GetEvaluation)
			evaluationRoutes.PUT("/:id", handlers.PermissionAnyMiddleware("assessment:edit", "assessment:submit", "assessment:review", "assessment:approve"), handlers.UpdateEvaluation)
			evaluationRoutes.DELETE("/:id", handlers.PermissionMiddleware("assessment:delete"), handlers.DeleteEvaluation)
			evaluationRoutes.GET("/employee/:employeeId", handlers.PermissionMiddleware("assessment:view"), handlers.GetEmployeeEvaluations)
			evaluationRoutes.GET("/pending/:employeeId", handlers.PermissionMiddleware("assessment:view"), handlers.GetPendingEvaluations)
			evaluationRoutes.GET("/pending/count", handlers.PermissionMiddleware("assessment:view"), handlers.GetPendingCountEvaluations)

			// 评论管理（所有认证用户）
			evaluationRoutes.GET("/:id/comments", handlers.GetEvaluationComments)
			evaluationRoutes.POST("/:id/comments", handlers.CreateEvaluationComment)
			evaluationRoutes.PUT("/:id/comments/:comment_id", handlers.UpdateEvaluationComment)
			evaluationRoutes.DELETE("/:id/comments/:comment_id", handlers.DeleteEvaluationComment)

			// 邀请评分管理（HR发起邀请）
			evaluationRoutes.POST("/:id/invitations", handlers.PermissionMiddleware("review:create"), handlers.CreateInvitation)
			// 获取邀请列表：HR可以查看所有，被评估员工和被邀请人可以查看相关邀请（权限检查在函数内部）
			evaluationRoutes.GET("/:id/invitations", handlers.GetEvaluationInvitations)

			// 异议处理
			evaluationRoutes.POST("/:id/objection", handlers.SubmitObjection) // 员工提交异议
			evaluationRoutes.PUT("/:id/objection/handle", handlers.PermissionMiddleware("assessment:approve"), handlers.HandleObjection)

			// 最终结果确认与纸质签字归档
			evaluationRoutes.GET("/:id/confirmation", handlers.PermissionMiddleware("assessment:view"), handlers.GetEvaluationConfirmation)
			evaluationRoutes.POST("/:id/confirm-online", handlers.PermissionMiddleware("assessment:submit"), handlers.ConfirmEvaluationOnline)
			evaluationRoutes.POST("/:id/confirm-paper", handlers.PermissionMiddleware("assessment:approve"), handlers.ConfirmEvaluationPaper)
			evaluationRoutes.GET("/:id/confirmation/attachment", handlers.PermissionMiddleware("assessment:view"), handlers.DownloadConfirmationAttachment)
		}

		// 邀请评分管理
		invitationRoutes := protected.Group("/invitations")
		{
			invitationRoutes.GET("/my", handlers.GetMyInvitations)             // 获取我的邀请列表
			invitationRoutes.GET("/sent", handlers.GetMySentInvitations)       // 获取我发出的邀请列表
			invitationRoutes.GET("/:id", handlers.GetInvitationDetails)        // 获取邀请详情
			invitationRoutes.PUT("/:id/accept", handlers.AcceptInvitation)     // 接受邀请
			invitationRoutes.PUT("/:id/decline", handlers.DeclineInvitation)   // 拒绝邀请
			invitationRoutes.PUT("/:id/complete", handlers.CompleteInvitation) // 完成邀请评分
			invitationRoutes.GET("/:id/scores", handlers.GetInvitationScores)  // 获取邀请评分
			invitationRoutes.PUT("/:id/cancel", handlers.PermissionMiddleware("review:manage"), handlers.CancelInvitation)
			invitationRoutes.PUT("/:id/reinvite", handlers.PermissionMiddleware("review:manage"), handlers.ReinviteInvitation)
			invitationRoutes.DELETE("/:id", handlers.PermissionMiddleware("review:manage"), handlers.DeleteInvitation)
			invitationRoutes.GET("/pending/count", handlers.GetPendingCountInvitations) // 获取待确认邀请数量
		}

		// 邀请评分记录管理
		invitedScoreRoutes := protected.Group("/invited-scores")
		{
			invitedScoreRoutes.PUT("/:id", handlers.UpdateInvitedScore) // 更新邀请评分
		}

		// KPI评分管理（所有认证用户）
		scoreRoutes := protected.Group("/scores")
		{
			scoreRoutes.GET("/evaluation/:evaluationId", handlers.PermissionAnyMiddleware("assessment:view", "review:view"), handlers.GetEvaluationScores)
			scoreRoutes.PUT("/:id/self", handlers.UpdateSelfScore)
			scoreRoutes.PUT("/:id/manager", handlers.PermissionMiddleware("assessment:review"), handlers.UpdateManagerScore)
			scoreRoutes.PUT("/:id/hr", handlers.PermissionMiddleware("assessment:review"), handlers.UpdateHRScore)
			scoreRoutes.PUT("/:id/final", handlers.PermissionMiddleware("assessment:approve"), handlers.UpdateFinalScore)
		}

		// 统计分析（所有认证用户）
		statsRoutes := protected.Group("/statistics")
		{
			statsRoutes.GET("/dashboard", handlers.PermissionMiddleware("report:company"), handlers.GetDashboardStats)
			statsRoutes.GET("/department/:id", handlers.PermissionMiddleware("report:department"), handlers.GetDepartmentStats)
			statsRoutes.GET("/employee/:id", handlers.PermissionAnyMiddleware("report:personal", "report:department", "report:company"), handlers.GetEmployeeStats)
			statsRoutes.GET("/trends", handlers.PermissionMiddleware("report:company"), handlers.GetTrends)
			statsRoutes.GET("/data", handlers.PermissionMiddleware("report:company"), handlers.GetStatisticsData)
			statsRoutes.GET("/signoffs", handlers.PermissionMiddleware("report:export"), handlers.GetSignoffStatistics)
		}

		// 最终评分批量导入（HR/绩效管理员）
		finalScoreRoutes := protected.Group("/final-scores")
		finalScoreRoutes.Use(handlers.PermissionMiddleware("assessment:approve"))
		{
			finalScoreRoutes.GET("/import-template", handlers.ExportFinalScoreImportTemplate)
			finalScoreRoutes.POST("/import/preview", handlers.PreviewFinalScoreImport)
			finalScoreRoutes.POST("/import/:batchId/commit", handlers.CommitFinalScoreImport)
		}

		// 导出功能（管理员和HR）
		exportRoutes := protected.Group("/export")
		exportRoutes.Use(handlers.PermissionMiddleware("report:export"))
		{
			exportRoutes.GET("/evaluation/:id", handlers.ExportEvaluationToExcel)
			exportRoutes.GET("/signoff-batch", handlers.ExportSignoffBatch)
			exportRoutes.GET("/department/:id", handlers.ExportDepartmentToExcel)
			exportRoutes.GET("/period/:period", handlers.ExportPeriodToExcel)
		}

		// 备份管理（仅HR）
		backupRoutes := protected.Group("/backup")
		backupRoutes.Use(handlers.PermissionMiddleware("system:edit"))
		{
			backupRoutes.POST("", handlers.CreateBackup)
			backupRoutes.GET("", handlers.GetBackupHistory)
			backupRoutes.GET("/download/:filename", handlers.GenerateBackupDownloadURL)
			backupRoutes.POST("/restore/:filename", handlers.RestoreBackup)
			backupRoutes.DELETE("/:filename", handlers.DeleteBackup)
		}
	}
}
