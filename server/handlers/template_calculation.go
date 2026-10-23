package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func hydrateCalculationTemplate(template *models.KPITemplate) {
	for index := range template.Items {
		if template.Items[index].ScorerTypesJSON == "" {
			template.Items[index].ScorerTypes = []string{}
			continue
		}
		_ = json.Unmarshal([]byte(template.Items[index].ScorerTypesJSON), &template.Items[index].ScorerTypes)
	}
}

func templateWeightGroups(template models.KPITemplate) ([]float64, [][]float64, error) {
	dimensions := make(map[uint]int, len(template.Dimensions))
	dimensionWeights := make([]float64, len(template.Dimensions))
	itemGroups := make([][]float64, len(template.Dimensions))
	for index, dimension := range template.Dimensions {
		dimensions[dimension.ID] = index
		dimensionWeights[index] = dimension.Weight
	}
	for _, item := range template.Items {
		if item.DimensionID == nil {
			return nil, nil, errors.New("每个考核指标必须选择所属维度")
		}
		index, ok := dimensions[*item.DimensionID]
		if !ok {
			return nil, nil, errors.New("考核指标引用了无效维度")
		}
		itemGroups[index] = append(itemGroups[index], item.ItemWeight)
		if item.Required && len(item.ScorerTypes) == 0 {
			return nil, nil, fmt.Errorf("必填指标“%s”必须配置评分人类型", item.Name)
		}
	}
	return dimensionWeights, itemGroups, nil
}

func validateTemplateForPublish(template models.KPITemplate) error {
	if template.SourceBlocked {
		return errors.New("模板包含导入来源异常，请由HR修正指标权重并保存后再发布")
	}
	if template.DepartmentID == nil || *template.DepartmentID == 0 {
		return errors.New("模板必须选择适用部门")
	}
	if strings.TrimSpace(template.PositionType) == "" {
		return errors.New("模板必须配置岗位类型")
	}
	if len(template.Dimensions) == 0 || len(template.Items) == 0 {
		return errors.New("模板必须包含考核维度和指标")
	}
	dimensionWeights, itemGroups, err := templateWeightGroups(template)
	if err != nil {
		return err
	}
	return ValidateTemplateWeights(dimensionWeights, itemGroups)
}

func loadCalculationTemplate(db *gorm.DB, id uint) (models.KPITemplate, error) {
	var template models.KPITemplate
	err := db.Preload("Dimensions", func(query *gorm.DB) *gorm.DB { return query.Order("sort_order, id") }).
		Preload("Items", func(query *gorm.DB) *gorm.DB { return query.Order("\"order\", id") }).First(&template, id).Error
	if err == nil {
		hydrateTemplateSourceIssues(&template)
		hydrateTemplateLayout(&template)
		hydrateCalculationTemplate(&template)
	}
	return template, err
}

func PublishTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_TEMPLATE_ID", "无效的模板ID", nil)
		return
	}
	template, err := loadCalculationTemplate(models.DB, uint(id))
	if err != nil {
		respondError(c, http.StatusNotFound, "TEMPLATE_NOT_FOUND", "模板不存在", nil)
		return
	}
	if template.Status == "retired" {
		respondError(c, http.StatusConflict, "RETIRED_TEMPLATE_IMMUTABLE", "已停用模板不能发布，请复制为新版本", nil)
		return
	}
	if err := validateTemplateForPublish(template); err != nil {
		respondError(c, http.StatusBadRequest, "TEMPLATE_PUBLISH_BLOCKED", "模板发布校验失败", map[string]any{"reason": err.Error()})
		return
	}
	var replaced []models.KPITemplate
	if err := models.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where(
			"id <> ? AND department_id = ? AND LOWER(position_type) = LOWER(?) AND period = ? AND status = ? AND is_active = ?",
			template.ID, template.DepartmentID, template.PositionType, template.Period, "published", true,
		).Order("version DESC, id DESC").Find(&replaced).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.KPITemplate{}).Where(
			"id <> ? AND department_id = ? AND LOWER(position_type) = LOWER(?) AND period = ? AND status = ? AND is_active = ?",
			template.ID, template.DepartmentID, template.PositionType, template.Period, "published", true,
		).Updates(map[string]any{"status": "retired", "is_active": false}).Error; err != nil {
			return err
		}
		return tx.Model(&template).Updates(map[string]any{"status": "published", "is_active": true}).Error
	}); err != nil {
		respondError(c, http.StatusInternalServerError, "TEMPLATE_PUBLISH_FAILED", "模板发布失败", nil)
		return
	}
	for _, retired := range replaced {
		models.DB.Create(&models.AuditLog{
			Action:     "template_version_switch",
			Resource:   "kpi_template",
			ResourceID: strconv.FormatUint(uint64(template.ID), 10),
			Result:     "SUCCESS",
			Details:    "retired_id=" + strconv.FormatUint(uint64(retired.ID), 10),
		})
	}
	RecordAuditDetails(c, "template_publish", "kpi_template", strconv.FormatUint(id, 10), "SUCCESS", "version="+strconv.Itoa(template.Version))
	template.Status = "published"
	template.IsActive = true
	c.JSON(http.StatusOK, gin.H{"message": "模板发布成功", "data": template})
}

func RetireTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_TEMPLATE_ID", "无效的模板ID", nil)
		return
	}
	result := models.DB.Model(&models.KPITemplate{}).Where("id = ?", id).Updates(map[string]any{"status": "retired", "is_active": false})
	if result.Error != nil {
		respondError(c, http.StatusInternalServerError, "TEMPLATE_RETIRE_FAILED", "模板停用失败", nil)
		return
	}
	if result.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, "TEMPLATE_NOT_FOUND", "模板不存在", nil)
		return
	}
	RecordAuditDetails(c, "template_retire", "kpi_template", strconv.FormatUint(id, 10), "SUCCESS", "")
	c.JSON(http.StatusOK, gin.H{"message": "模板已停用"})
}

type cloneTemplateRequest struct {
	Name string `json:"name"`
}

func CloneTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_TEMPLATE_ID", "无效的模板ID", nil)
		return
	}
	var request cloneTemplateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_TEMPLATE_CLONE_REQUEST", "请求参数错误", nil)
		return
	}
	source, err := loadCalculationTemplate(models.DB, uint(id))
	if err != nil {
		respondError(c, http.StatusNotFound, "TEMPLATE_NOT_FOUND", "模板不存在", nil)
		return
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		name = source.Name + " 副本"
	}
	var cloned models.KPITemplate
	err = models.DB.Transaction(func(tx *gorm.DB) error {
		cloned = source
		cloned.ID = 0
		cloned.Name = name
		cloned.Version = source.Version + 1
		cloned.Status = "draft"
		cloned.IsActive = false
		cloned.Items = nil
		cloned.Dimensions = nil
		if err := tx.Create(&cloned).Error; err != nil {
			return err
		}
		dimensionIDs := map[uint]uint{}
		for _, dimension := range source.Dimensions {
			oldID := dimension.ID
			dimension.ID = 0
			dimension.TemplateID = cloned.ID
			if err := tx.Create(&dimension).Error; err != nil {
				return err
			}
			dimensionIDs[oldID] = dimension.ID
		}
		for _, item := range source.Items {
			item.ID = 0
			item.TemplateID = cloned.ID
			if item.DimensionID != nil {
				newID := dimensionIDs[*item.DimensionID]
				item.DimensionID = &newID
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "TEMPLATE_CLONE_FAILED", "复制模板失败", map[string]any{"reason": err.Error()})
		return
	}
	RecordAuditDetails(c, "clone_kpi_template", "kpi_template", strconv.FormatUint(uint64(cloned.ID), 10), "SUCCESS", "source="+strconv.FormatUint(id, 10))
	cloned, _ = loadCalculationTemplate(models.DB, cloned.ID)
	c.JSON(http.StatusCreated, gin.H{"message": "模板复制成功", "data": cloned})
}

type templateDimensionInput struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Weight      float64             `json:"weight"`
	SortOrder   int                 `json:"sort_order"`
	Items       []templateItemInput `json:"items"`
}

type templateItemInput struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	ItemWeight      float64  `json:"item_weight"`
	ScoringGuidance string   `json:"scoring_guidance"`
	DataSource      string   `json:"data_source"`
	ScorerTypes     []string `json:"scorer_types"`
	Required        bool     `json:"required"`
	Order           int      `json:"order"`
}

type templateStructureRequest struct {
	Dimensions []templateDimensionInput `json:"dimensions" binding:"required"`
}

func SaveTemplateCalculationStructure(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的模板ID"})
		return
	}
	var request templateStructureRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.Dimensions) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "模板至少需要一个考核维度"})
		return
	}
	var template models.KPITemplate
	if err := models.DB.First(&template, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "模板不存在"})
		return
	}
	if template.Status == "published" || template.Status == "retired" {
		c.JSON(http.StatusConflict, gin.H{"error": "已发布或已停用模板不可直接修改，请复制为新版本"})
		return
	}
	dimensionWeights := make([]float64, 0, len(request.Dimensions))
	itemGroups := make([][]float64, 0, len(request.Dimensions))
	for _, dimension := range request.Dimensions {
		if strings.TrimSpace(dimension.Name) == "" || len(dimension.Items) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "每个维度必须有名称和至少一个指标"})
			return
		}
		dimensionWeights = append(dimensionWeights, dimension.Weight)
		weights := make([]float64, 0, len(dimension.Items))
		for _, item := range dimension.Items {
			if strings.TrimSpace(item.Name) == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "指标名称不能为空"})
				return
			}
			if item.Required && len(item.ScorerTypes) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "必填指标必须配置评分人类型", "item": item.Name})
				return
			}
			weights = append(weights, item.ItemWeight)
		}
		itemGroups = append(itemGroups, weights)
	}
	if err := ValidateTemplateWeights(dimensionWeights, itemGroups); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "模板权重配置无效", "message": err.Error()})
		return
	}
	err = models.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("template_id = ?", template.ID).Delete(&models.KPIItem{}).Error; err != nil {
			return err
		}
		if err := tx.Where("template_id = ?", template.ID).Delete(&models.KPIDimension{}).Error; err != nil {
			return err
		}
		for dimensionIndex, input := range request.Dimensions {
			dimension := models.KPIDimension{TemplateID: template.ID, Name: strings.TrimSpace(input.Name), Description: input.Description, Weight: input.Weight, SortOrder: input.SortOrder}
			if dimension.SortOrder == 0 {
				dimension.SortOrder = dimensionIndex + 1
			}
			if err := tx.Create(&dimension).Error; err != nil {
				return err
			}
			for itemIndex, itemInput := range input.Items {
				scorers, err := json.Marshal(itemInput.ScorerTypes)
				if err != nil {
					return err
				}
				order := itemInput.Order
				if order == 0 {
					order = itemIndex + 1
				}
				item := models.KPIItem{TemplateID: template.ID, DimensionID: &dimension.ID, Name: strings.TrimSpace(itemInput.Name), Description: itemInput.Description, MaxScore: 100, Order: order, ItemWeight: itemInput.ItemWeight, CompositeWeight: CompositeWeight(input.Weight, itemInput.ItemWeight), ScoringGuidance: itemInput.ScoringGuidance, DataSource: itemInput.DataSource, ScorerTypesJSON: string(scorers), Required: itemInput.Required}
				if err := tx.Create(&item).Error; err != nil {
					return err
				}
			}
		}
		// The current weights have passed validation. Preserve the original
		// source cells and warnings; saving a draft cannot acknowledge unrelated
		// source failures just by submitting the same structure again.
		issues, blocked, err := sourceIssuesAfterValidStructure(template)
		if err != nil {
			return err
		}
		if err := tx.Model(&models.KPITemplate{}).Where("id = ?", template.ID).Updates(map[string]any{"source_blocked": blocked, "source_issues_json": issues}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "模板结构保存失败", "message": err.Error()})
		return
	}
	RecordAudit(c, "update_kpi_template_structure", "kpi_template", strconv.FormatUint(id, 10), "SUCCESS")
	updated, _ := loadCalculationTemplate(models.DB, template.ID)
	c.JSON(http.StatusOK, gin.H{"message": "模板结构保存成功", "data": updated})
}

func sourceIssuesAfterValidStructure(template models.KPITemplate) (string, bool, error) {
	if strings.TrimSpace(template.SourceIssuesJSON) == "" {
		return template.SourceIssuesJSON, template.SourceBlocked, nil
	}
	var issues []map[string]any
	if err := json.Unmarshal([]byte(template.SourceIssuesJSON), &issues); err != nil {
		return "", false, fmt.Errorf("来源诊断格式无效: %w", err)
	}
	blocked := template.SourceBlocked && len(issues) == 0
	for _, issue := range issues {
		code, _ := issue["code"].(string)
		switch strings.ToLower(code) {
		case "dimension_item_weight_sum":
			issue["severity"] = "warning"
		case "composite_weight_total":
			issue["severity"] = "warning"
			issue["resolved_in_draft"] = true
		default:
			severity, _ := issue["severity"].(string)
			blocked = blocked || severity != "warning"
		}
	}
	encoded, err := json.Marshal(issues)
	return string(encoded), blocked, err
}
