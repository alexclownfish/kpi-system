package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"dootask-kpi-server/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func hydrateTemplateSourceIssues(template *models.KPITemplate) {
	template.SourceIssues = nil
	if strings.TrimSpace(template.SourceIssuesJSON) == "" {
		return
	}
	_ = json.Unmarshal([]byte(template.SourceIssuesJSON), &template.SourceIssues)
}

type templateMutationRequest struct {
	Name         *string                    `json:"name"`
	Description  *string                    `json:"description"`
	Period       *string                    `json:"period"`
	IsActive     *bool                      `json:"is_active"`
	DepartmentID *uint                      `json:"department_id"`
	PositionType *string                    `json:"position_type"`
	ExportLayout *models.ResultExportLayout `json:"export_layout"`
}

func hydrateTemplateLayout(template *models.KPITemplate) {
	layout, _ := normalizeStoredExportLayout(template.ExportLayoutJSON)
	// Expose responsibility-driven defaults in template settings while keeping
	// the persisted JSON untouched. Explicit custom labels always win.
	scorerTypes := make([]string, 0)
	for _, item := range template.Items {
		types := append([]string(nil), item.ScorerTypes...)
		if len(types) == 0 && item.ScorerTypesJSON != "" {
			_ = json.Unmarshal([]byte(item.ScorerTypesJSON), &types)
		}
		for _, scorerType := range types {
			scorerTypes = appendUniqueString(scorerTypes, scorerType)
		}
	}
	template.ExportLayout = responsibilitySigningLabels(layout, scorerTypes)
}

func ensureTemplateDraftForMutation(db *gorm.DB, templateID uint) error {
	var template models.KPITemplate
	if err := db.Select("id", "status").First(&template, templateID).Error; err != nil {
		return err
	}
	if template.Status == "published" || template.Status == "retired" {
		return errors.New("已发布或已停用模板不可直接修改，请复制为新版本")
	}
	return nil
}

// 获取所有KPI模板
func GetTemplates(c *gin.Context) {
	var templates []models.KPITemplate

	query := models.DB.Preload("Dimensions").Preload("Items")
	if departmentID := c.Query("department_id"); departmentID != "" {
		query = query.Where("department_id = ?", departmentID)
	}
	if positionType := strings.TrimSpace(c.Query("position_type")); positionType != "" {
		query = query.Where("LOWER(position_type) = LOWER(?)", positionType)
	}
	if period := c.Query("period"); period != "" {
		query = query.Where("period = ?", period)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	result := query.Find(&templates)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取模板列表失败",
			"message": result.Error.Error(),
		})
		return
	}
	for i := range templates {
		hydrateTemplateSourceIssues(&templates[i])
		hydrateTemplateLayout(&templates[i])
		hydrateCalculationTemplate(&templates[i])
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
	template := models.KPITemplate{Name: strings.TrimSpace(*request.Name), Period: *request.Period, IsActive: false, Status: "draft", Version: 1, CalculationVersion: "v1", ExportLayoutJSON: raw, ExportLayout: layout}
	if request.Description != nil {
		template.Description = *request.Description
	}
	// Activation is controlled by PublishTemplate/RetireTemplate. Keep accepting
	// the legacy is_active field, but a newly created draft must stay inactive.
	if request.DepartmentID != nil {
		template.DepartmentID = request.DepartmentID
	}
	if request.PositionType != nil {
		template.PositionType = strings.TrimSpace(*request.PositionType)
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
	result := models.DB.Preload("Dimensions").Preload("Items").First(&template, templateId)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "模板不存在",
		})
		return
	}
	hydrateTemplateLayout(&template)
	hydrateTemplateSourceIssues(&template)
	hydrateCalculationTemplate(&template)

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
	if template.Status == "published" || template.Status == "retired" {
		respondError(c, http.StatusConflict, "PUBLISHED_TEMPLATE_IMMUTABLE", "已发布或已停用模板不可直接修改，请复制为新版本", nil)
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
	// Saving draft metadata cannot activate it or bypass publication validation.
	updates := map[string]any{"is_active": false}
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
	if updateData.DepartmentID != nil {
		updates["department_id"] = *updateData.DepartmentID
	}
	if updateData.PositionType != nil {
		updates["position_type"] = strings.TrimSpace(*updateData.PositionType)
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
	if err := models.DB.Preload("Dimensions").Preload("Items").First(&template, template.ID).Error; err == nil {
		hydrateTemplateLayout(&template)
		hydrateCalculationTemplate(&template)
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

	// 模板维度和指标都属于模板本身，必须在同一个事务中清理，避免
	// 删除模板后遗留无法显示的维度/指标孤儿记录。
	result := models.DB.Transaction(func(tx *gorm.DB) error {
		var existing models.KPITemplate
		if err := tx.First(&existing, templateId).Error; err != nil {
			return err
		}
		if existing.Status == "published" {
			return errPublishedTemplateDelete
		}
		var evaluationCount int64
		if err := tx.Model(&models.KPIEvaluation{}).Where("template_id = ?", templateId).Count(&evaluationCount).Error; err != nil {
			return err
		}
		if evaluationCount > 0 {
			return errTemplateHasEvaluations
		}
		if err := tx.Where("template_id = ?", templateId).Delete(&models.KPIItem{}).Error; err != nil {
			return err
		}
		if err := tx.Where("template_id = ?", templateId).Delete(&models.KPIDimension{}).Error; err != nil {
			return err
		}
		result := tx.Delete(&models.KPITemplate{}, templateId)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if result != nil {
		if errors.Is(result, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "模板不存在"})
			return
		}
		if errors.Is(result, errTemplateHasEvaluations) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "该模板有相关的评估记录，无法删除"})
			return
		}
		if errors.Is(result, errPublishedTemplateDelete) {
			c.JSON(http.StatusConflict, gin.H{"error": "当前发布模板不能直接删除，请先停用并切换新版本"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "删除模板失败",
			"message": result.Error(),
		})
		return
	}
	RecordAuditDetails(c, "delete_template", "kpi_template", strconv.FormatUint(templateId, 10), "SUCCESS", "items_and_dimensions_deleted")

	c.JSON(http.StatusOK, gin.H{
		"message": "模板删除成功",
	})
}

var errTemplateHasEvaluations = errors.New("模板存在评估记录")
var errPublishedTemplateDelete = errors.New("当前发布模板不能直接删除")

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
	result := models.DB.Where("template_id = ?", templateId).Order("\"order\"").Find(&items)
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
