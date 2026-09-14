package handlers

import (
	"net/http"
	"strconv"

	"dootask-kpi-server/models"

	"github.com/gin-gonic/gin"
)

// 获取评估评论列表
func GetEvaluationComments(c *gin.Context) {
	evaluationID := c.Param("id")

	// 从JWT token获取当前用户ID
	currentUserID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未找到用户信息"})
		return
	}

	// 解析分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))

	// 验证分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 10
	}

	var comments []models.EvaluationComment

	// 构建查询条件
	query := models.DB.Where("evaluation_id = ?", evaluationID)

	// 转换用户ID类型
	userID := currentUserID.(uint)

	// 如果不是管理员，只显示公开评论和自己的私有评论
	// 这里简化处理，假设所有用户都可以看到公开评论
	query = query.Where("is_private = ? OR user_id = ?", false, userID)

	// 获取总数
	var total int64
	countQuery := models.DB.Model(&models.EvaluationComment{}).Where("evaluation_id = ?", evaluationID)
	countQuery = countQuery.Where("is_private = ? OR user_id = ?", false, userID)

	if err := countQuery.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取评论总数失败"})
		return
	}

	// 分页查询，按创建时间倒序
	offset := (page - 1) * pageSize
	if err := query.Preload("User").Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&comments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取评论失败"})
		return
	}

	// 计算分页信息
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))

	c.JSON(http.StatusOK, gin.H{
		"message":    "获取评论成功",
		"data":       comments,
		"total":      total,
		"page":       page,
		"pageSize":   pageSize,
		"totalPages": totalPages,
		"hasNext":    page < totalPages,
		"hasPrev":    page > 1,
	})
}

// 创建评论
func CreateEvaluationComment(c *gin.Context) {
	evaluationID := c.Param("id")

	// 从JWT token获取当前用户ID
	currentUserID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未找到用户信息"})
		return
	}
	userID := currentUserID.(uint)

	var req struct {
		Content   string `json:"content" binding:"required"`
		IsPrivate bool   `json:"is_private"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证评估记录是否存在
	var evaluation models.KPIEvaluation
	if err := models.DB.First(&evaluation, evaluationID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "评估记录不存在"})
		return
	}

	// 创建评论
	evalID, _ := strconv.ParseUint(evaluationID, 10, 32)
	comment := models.EvaluationComment{
		EvaluationID: uint(evalID),
		UserID:       userID,
		Content:      req.Content,
		IsPrivate:    req.IsPrivate,
	}

	if err := models.DB.Create(&comment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建评论失败"})
		return
	}

	// 预加载用户信息后返回
	models.DB.Preload("User").First(&comment, comment.ID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "评论创建成功",
		"data":    comment,
	})
}

// 更新评论
func UpdateEvaluationComment(c *gin.Context) {
	commentID := c.Param("comment_id")

	// 从JWT token获取当前用户ID
	currentUserID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未找到用户信息"})
		return
	}
	userID := currentUserID.(uint)

	var req struct {
		Content   string `json:"content" binding:"required"`
		IsPrivate bool   `json:"is_private"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 查找评论
	var comment models.EvaluationComment
	if err := models.DB.First(&comment, commentID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "评论不存在"})
		return
	}

	// 检查权限：只有评论作者可以编辑
	if comment.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权限编辑此评论"})
		return
	}

	// 更新评论
	comment.Content = req.Content
	comment.IsPrivate = req.IsPrivate

	if err := models.DB.Save(&comment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新评论失败"})
		return
	}

	// 预加载用户信息后返回
	models.DB.Preload("User").First(&comment, comment.ID)

	c.JSON(http.StatusOK, gin.H{
		"message": "评论更新成功",
		"data":    comment,
	})
}

// 删除评论
func DeleteEvaluationComment(c *gin.Context) {
	commentID := c.Param("comment_id")

	// 从JWT token获取当前用户ID
	currentUserID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未找到用户信息"})
		return
	}
	userID := currentUserID.(uint)

	// 查找评论
	var comment models.EvaluationComment
	if err := models.DB.First(&comment, commentID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "评论不存在"})
		return
	}

	// 检查权限：只有评论作者可以删除
	if comment.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权限删除此评论"})
		return
	}

	// 删除评论
	if err := models.DB.Delete(&comment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除评论失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "评论删除成功",
	})
}
