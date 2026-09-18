package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"dootask-kpi-server/models"

	"github.com/gin-gonic/gin"
)

type templateMutationRequest struct {
	Name         *string                    `json:"name"`
	Description  *string                    `json:"description"`
	Period       *string                    `json:"period"`
	IsActive     *bool                      `json:"is_active"`
	ExportLayout *models.ResultExportLayout `json:"export_layout"`
}

func hydrateTemplateLayout(template *models.KPITemplate) {
	layout, _ := normalizeStoredExportLayout(template.ExportLayoutJSON)
	template.ExportLayout = layout
}

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
	for i := range templates {
		hydrateTemplateLayout(&templates[i])
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  templates,
		"total": len(templates),
	})
}

// 创建KPI模板
func CreateTemplate(c *gin.Context) {
	var request templateMutationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}
	if request.Name == nil || strings.TrimSpace(*request.Name) == "" || request.Period == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "message": "模板名称和考核周期不能为空"})
		return
	}
	layout := defaultExportLayout(exportPresetFinalSignoff)
	if request.ExportLayout != nil {
		layout = *request.ExportLayout
	}
	raw, layout, err := marshalExportLayout(layout)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "导出版式配置无效", "message": err.Error()})
		return
	}
	template := models.KPITemplate{Name: strings.TrimSpace(*request.Name), Period: *request.Period, IsActive: true, ExportLayoutJSON: raw, ExportLayout: layout}
	if request.Description != nil {
		template.Description = *request.Description
	}
	if request.IsActive != nil {
		template.IsActive = *request.IsActive
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
	hydrateTemplateLayout(&template)

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

	var updateData templateMutationRequest
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}
	updates := map[string]any{}
	if updateData.Name != nil {
		name := strings.TrimSpace(*updateData.Name)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "message": "模板名称不能为空"})
			return
		}
		updates["name"] = name
	}
	if updateData.Description != nil {
		updates["description"] = *updateData.Description
	}
	if updateData.Period != nil {
		updates["period"] = *updateData.Period
	}
	if updateData.IsActive != nil {
		updates["is_active"] = *updateData.IsActive
	}
	layoutChanged := false
	if updateData.ExportLayout != nil {
		raw, layout, err := marshalExportLayout(*updateData.ExportLayout)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "导出版式配置无效", "message": err.Error()})
			return
		}
		updates["export_layout_json"] = raw
		template.ExportLayout = layout
		layoutChanged = raw != template.ExportLayoutJSON
	}
	result = models.DB.Model(&template).Updates(updates)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "更新模板失败",
			"message": result.Error.Error(),
		})
		return
	}
	if err := models.DB.First(&template, template.ID).Error; err == nil {
		hydrateTemplateLayout(&template)
	}
	if layoutChanged {
		details, _ := json.Marshal(template.ExportLayout)
		RecordAuditDetails(c, "update_template_export_layout", "kpi_template", strconv.FormatUint(uint64(template.ID), 10), "SUCCESS", string(details))
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
