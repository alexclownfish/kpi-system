package handlers

import (
	"net/http"
	"strconv"

	"dootask-kpi-server/models"

	"github.com/gin-gonic/gin"
)

// 获取所有KPI模板
func GetTemplates(c *gin.Context) {
	var templates []models.KPITemplate

	result := models.DB.Preload("Items").Find(&templates)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取模板列表失败",
			"message": result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  templates,
		"total": len(templates),
	})
}

// 创建KPI模板
func CreateTemplate(c *gin.Context) {
	var template models.KPITemplate

	if err := c.ShouldBindJSON(&template); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}

	result := models.DB.Create(&template)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "创建模板失败",
			"message": result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "模板创建成功",
		"data":    template,
	})
}

// 获取单个KPI模板
func GetTemplate(c *gin.Context) {
	id := c.Param("id")
	templateId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的模板ID",
		})
		return
	}

	var template models.KPITemplate
	result := models.DB.Preload("Items").First(&template, templateId)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "模板不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": template,
	})
}

// 更新KPI模板
func UpdateTemplate(c *gin.Context) {
	id := c.Param("id")
	templateId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的模板ID",
		})
		return
	}

	var template models.KPITemplate
	result := models.DB.First(&template, templateId)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "模板不存在",
		})
		return
	}

	var updateData models.KPITemplate
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}

	result = models.DB.Model(&template).Updates(updateData)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "更新模板失败",
			"message": result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "模板更新成功",
		"data":    template,
	})
}

// 删除KPI模板
func DeleteTemplate(c *gin.Context) {
	id := c.Param("id")
	templateId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的模板ID",
		})
		return
	}

	// 检查是否有相关的评估记录
	var evaluationCount int64
	models.DB.Model(&models.KPIEvaluation{}).Where("template_id = ?", templateId).Count(&evaluationCount)
	if evaluationCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "该模板有相关的评估记录，无法删除",
		})
		return
	}

	// 删除模板的同时删除相关的KPI项目
	models.DB.Where("template_id = ?", templateId).Delete(&models.KPIItem{})

	result := models.DB.Delete(&models.KPITemplate{}, templateId)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "删除模板失败",
			"message": result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "模板删除成功",
	})
}

// 获取模板的KPI项目
func GetTemplateItems(c *gin.Context) {
	id := c.Param("id")
	templateId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的模板ID",
		})
		return
	}

	var items []models.KPIItem
	result := models.DB.Where("template_id = ?", templateId).Order("`order`").Find(&items)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取KPI项目失败",
			"message": result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  items,
		"total": len(items),
	})
}
