package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"dootask-kpi-server/models"
	"dootask-kpi-server/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// KPI项目管理

// 创建KPI项目
func CreateItem(c *gin.Context) {
	var item models.KPIItem

	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}
	if item.TemplateID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "必须指定所属模板"})
		return
	}
	if err := ensureTemplateDraftForMutation(models.DB, item.TemplateID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "模板不存在"})
		} else {
			respondError(c, http.StatusConflict, "PUBLISHED_TEMPLATE_IMMUTABLE", err.Error(), nil)
		}
		return
	}

	result := models.DB.Create(&item)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "创建KPI项目失败",
			"message": result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "KPI项目创建成功",
		"data":    item,
	})
}

// 获取KPI项目
func GetItem(c *gin.Context) {
	id := c.Param("id")
	itemId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的项目ID",
		})
		return
	}

	var item models.KPIItem
	result := models.DB.Preload("Template").First(&item, itemId)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "KPI项目不存在",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": item,
	})
}

// 更新KPI项目
func UpdateItem(c *gin.Context) {
	id := c.Param("id")
	itemId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的项目ID",
		})
		return
	}

	var item models.KPIItem
	result := models.DB.First(&item, itemId)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "KPI项目不存在",
		})
		return
	}
	if err := ensureTemplateDraftForMutation(models.DB, item.TemplateID); err != nil {
		respondError(c, http.StatusConflict, "PUBLISHED_TEMPLATE_IMMUTABLE", err.Error(), nil)
		return
	}

	var updateData models.KPIItem
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}

	result = models.DB.Model(&item).Updates(updateData)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "更新KPI项目失败",
			"message": result.Error.Error(),
		})
		return
	}

	if updateData.MaxScore == 0 {
		result = models.DB.Model(&item).Update("max_score", 0)
		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "更新MaxScore失败",
				"message": result.Error.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "KPI项目更新成功",
		"data":    item,
	})
}

// 删除KPI项目
func DeleteItem(c *gin.Context) {
	id := c.Param("id")
	itemId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的项目ID",
		})
		return
	}

	var item models.KPIItem
	if err := models.DB.First(&item, itemId).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "KPI项目不存在"})
		return
	}
	if err := ensureTemplateDraftForMutation(models.DB, item.TemplateID); err != nil {
		respondError(c, http.StatusConflict, "PUBLISHED_TEMPLATE_IMMUTABLE", err.Error(), nil)
		return
	}
	result := models.DB.Delete(&models.KPIItem{}, itemId)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "删除KPI项目失败",
			"message": result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "KPI项目删除成功",
	})
}

// KPI评估管理

// 获取所有评估
func GetEvaluations(c *gin.Context) {
	var evaluations []models.KPIEvaluation

	// 解析分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	status := c.Query("status")
	employeeID := c.Query("employee_id")
	departmentID := c.Query("department_id")
	managerID := c.Query("manager_id")
	period := c.Query("period")
	year := c.Query("year")
	month := c.Query("month")
	quarter := c.Query("quarter")

	// 验证分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 1000 {
		pageSize = 10
	}

	// 构建查询
	userID := c.GetUint("user_id")
	query := ApplyEvaluationScope(models.DB.Preload("Employee.Department").Preload("Template").Preload("Scores"), userID)
	assignedOnly := c.Query("assigned_to_me") == "true"
	filterAssigned := func(query *gorm.DB) *gorm.DB {
		if !assignedOnly {
			return query
		}
		return query.Where("kpi_evaluations.status IN ? AND kpi_evaluations.id IN (SELECT evaluation_id FROM evaluation_scoring_tasks WHERE scorer_user_id = ? AND status IN ?)", []string{"scoring", "awaiting_review"}, userID, []string{"pending", "in_progress", "rejected"})
	}
	query = filterAssigned(query)

	// 添加筛选条件
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if employeeID != "" {
		query = query.Where("employee_id = ?", employeeID)
	}
	// 如果同时提供了departmentID和managerID，使用OR关系（本部门员工 OR 直属下级）
	if departmentID != "" && managerID != "" {
		query = query.Where("employee_id IN (SELECT id FROM employees WHERE department_id = ? OR manager_id = ?)", departmentID, managerID)
	} else {
		if departmentID != "" {
			query = query.Where("employee_id IN (SELECT id FROM employees WHERE department_id = ?)", departmentID)
		}
		if managerID != "" {
			query = query.Where("employee_id IN (SELECT id FROM employees WHERE manager_id = ?)", managerID)
		}
	}
	if period != "" {
		query = query.Where("period = ?", period)
	}
	if year != "" {
		query = query.Where("year = ?", year)
	}
	if month != "" {
		query = query.Where("month = ?", month)
	}
	if quarter != "" {
		query = query.Where("quarter = ?", quarter)
	}

	// 构建基础统计查询（不含 status 筛选，用于统计卡片）
	// 只统计在职员工的评估
	buildStatsQuery := func() *gorm.DB {
		statsQuery := ApplyEvaluationScope(models.DB.Model(&models.KPIEvaluation{}), userID).
			Joins("JOIN employees ON kpi_evaluations.employee_id = employees.id").
			Where("employees.is_active = ?", true)
		statsQuery = filterAssigned(statsQuery)
		if employeeID != "" {
			statsQuery = statsQuery.Where("kpi_evaluations.employee_id = ?", employeeID)
		}
		// 如果同时提供了departmentID和managerID，使用OR关系（本部门员工 OR 直属下级）
		if departmentID != "" && managerID != "" {
			statsQuery = statsQuery.Where("(employees.department_id = ? OR employees.manager_id = ?)", departmentID, managerID)
		} else {
			if departmentID != "" {
				statsQuery = statsQuery.Where("employees.department_id = ?", departmentID)
			}
			if managerID != "" {
				statsQuery = statsQuery.Where("employees.manager_id = ?", managerID)
			}
		}
		if period != "" {
			statsQuery = statsQuery.Where("kpi_evaluations.period = ?", period)
		}
		if year != "" {
			statsQuery = statsQuery.Where("kpi_evaluations.year = ?", year)
		}
		if month != "" {
			statsQuery = statsQuery.Where("kpi_evaluations.month = ?", month)
		}
		if quarter != "" {
			statsQuery = statsQuery.Where("kpi_evaluations.quarter = ?", quarter)
		}
		return statsQuery
	}

	// 获取统计数据
	var statsTotal int64
	var statsPending int64
	var statsCompleted int64
	var statsAvgScore float64

	// 总评估数
	if err := buildStatsQuery().Count(&statsTotal).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取统计数据失败",
			"message": err.Error(),
		})
		return
	}

	// 待处理数同时覆盖旧流程和新版流程状态。
	if err := buildStatsQuery().Where("kpi_evaluations.status IN ?", []string{"pending", "draft", "self_evaluating", "self_evaluated", "scoring", "awaiting_review", "manager_evaluated", "pending_confirm", "awaiting_employee_confirmation", "rejected"}).Count(&statsPending).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取统计数据失败",
			"message": err.Error(),
		})
		return
	}

	// 已完成数
	if err := buildStatsQuery().Where("kpi_evaluations.status = ?", "completed").Count(&statsCompleted).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取统计数据失败",
			"message": err.Error(),
		})
		return
	}

	// Only finalized assessments contribute; zero is a valid final score.
	var avgResult struct {
		AvgScore float64
	}
	if err := buildStatsQuery().Where("kpi_evaluations.status = ?", "completed").Select("COALESCE(AVG(kpi_evaluations.total_score), 0) as avg_score").Scan(&avgResult).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取统计数据失败",
			"message": err.Error(),
		})
		return
	}
	statsAvgScore = avgResult.AvgScore

	// 获取总数（用于分页，受 status 筛选影响）
	var total int64
	countQuery := ApplyEvaluationScope(models.DB.Model(&models.KPIEvaluation{}), userID)
	countQuery = filterAssigned(countQuery)
	if status != "" {
		countQuery = countQuery.Where("status = ?", status)
	}
	if employeeID != "" {
		countQuery = countQuery.Where("employee_id = ?", employeeID)
	}
	// 如果同时提供了departmentID和managerID，使用OR关系（本部门员工 OR 直属下级）
	if departmentID != "" && managerID != "" {
		countQuery = countQuery.Where("employee_id IN (SELECT id FROM employees WHERE department_id = ? OR manager_id = ?)", departmentID, managerID)
	} else {
		if departmentID != "" {
			countQuery = countQuery.Where("employee_id IN (SELECT id FROM employees WHERE department_id = ?)", departmentID)
		}
		if managerID != "" {
			countQuery = countQuery.Where("employee_id IN (SELECT id FROM employees WHERE manager_id = ?)", managerID)
		}
	}
	if period != "" {
		countQuery = countQuery.Where("period = ?", period)
	}
	if year != "" {
		countQuery = countQuery.Where("year = ?", year)
	}
	if month != "" {
		countQuery = countQuery.Where("month = ?", month)
	}
	if quarter != "" {
		countQuery = countQuery.Where("quarter = ?", quarter)
	}
	if err := countQuery.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取评估总数失败",
			"message": err.Error(),
		})
		return
	}

	// 分页查询，按创建时间倒序
	offset := (page - 1) * pageSize
	result := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&evaluations)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取评估列表失败",
			"message": result.Error.Error(),
		})
		return
	}

	// 计算分页信息
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))

	c.JSON(http.StatusOK, gin.H{
		"data":       evaluations,
		"total":      total,
		"page":       page,
		"pageSize":   pageSize,
		"totalPages": totalPages,
		"hasNext":    page < totalPages,
		"hasPrev":    page > 1,
		"stats": gin.H{
			"total":     statsTotal,
			"pending":   statsPending,
			"completed": statsCompleted,
			"avgScore":  statsAvgScore,
		},
	})
}

// 创建评估
func CreateEvaluation(c *gin.Context) {
	var evaluation models.KPIEvaluation

	if err := c.ShouldBindJSON(&evaluation); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}
	if err := validateEvaluationPeriod(evaluation); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_EVALUATION_PERIOD", err.Error(), nil)
		return
	}
	if evaluation.TotalScore != 0 || (evaluation.Status != "" && evaluation.Status != "pending" && evaluation.Status != workflowStateSelfEvaluating) {
		respondError(c, http.StatusBadRequest, "SERVER_OWNED_RESULT", "新建考核不能指定最终分数或已完成状态", nil)
		return
	}
	projectName := strings.TrimSpace(evaluation.ProjectName)
	allocationRatio := evaluation.AllocationRatio
	if allocationRatio != nil && (math.IsNaN(*allocationRatio) || math.IsInf(*allocationRatio, 0) || *allocationRatio < 0 || *allocationRatio > 1) {
		respondError(c, http.StatusBadRequest, "INVALID_ALLOCATION_RATIO", "项目投入占比必须为0到1之间的有限数值", nil)
		return
	}
	// Retain only authoring inputs. Embedded associations, IDs, result and
	// snapshot metadata are server-owned and must never be mass-assigned.
	evaluation = models.KPIEvaluation{
		EmployeeID: evaluation.EmployeeID, TemplateID: evaluation.TemplateID,
		Period: evaluation.Period, Year: evaluation.Year, Month: evaluation.Month, Quarter: evaluation.Quarter,
		ProjectName: projectName, AllocationRatio: allocationRatio,
	}
	if !CanAccessEmployee(c.GetUint("user_id"), evaluation.EmployeeID) {
		respondError(c, http.StatusForbidden, "ASSESSMENT_SCOPE_FORBIDDEN", "被考核员工超出数据范围", nil)
		return
	}
	var employee models.Employee
	if err := models.DB.First(&employee, evaluation.EmployeeID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "被考核员工不存在"})
		return
	}
	if !employee.IsActive {
		respondError(c, http.StatusBadRequest, "EMPLOYEE_INACTIVE", "不能为已停用员工创建考核", nil)
		return
	}
	// 新流程不再要求前端硬编码模板 ID；按员工部门、岗位和周期自动匹配。
	var template models.KPITemplate
	var err error
	if evaluation.TemplateID == 0 {
		template, err = findPublishedTemplate(models.DB, employee, evaluation.Period)
	} else {
		template, err = loadCalculationTemplate(models.DB, evaluation.TemplateID)
		if err == nil && (template.Status != "published" || !template.IsActive) {
			err = fmt.Errorf("所选模板不是已发布模板")
		}
	}
	if err != nil {
		status := http.StatusBadRequest
		code := "TEMPLATE_NOT_FOUND"
		if strings.Contains(err.Error(), "TEMPLATE_MATCH_AMBIGUOUS") {
			status = http.StatusConflict
			code = "TEMPLATE_MATCH_AMBIGUOUS"
		}
		respondError(c, status, code, "无法匹配考核模板", err.Error())
		return
	}
	if !templateMatchesEmployee(models.DB, template, employee, evaluation.Period) {
		respondError(c, http.StatusBadRequest, "TEMPLATE_SCOPE_MISMATCH", "模板的部门、岗位或周期与员工考核不匹配", nil)
		return
	}
	workflowWrites := models.EvaluationWorkflowWritesEnabled()
	if template.CalculationVersion == canonicalCalculationVersion && !workflowWrites {
		respondError(c, http.StatusConflict, "WORKFLOW_WRITES_DISABLED", "当前为 "+models.EvaluationWorkflowMode()+" 模式，新模板考核需管理员显式启用 EVALUATION_WORKFLOW_MODE=enabled", nil)
		return
	}
	var snapshot string
	if workflowWrites {
		assignments, resolveErr := resolveTemplateScoringAssignments(models.DB, employee, template)
		err = resolveErr
		if err == nil {
			snapshot, err = buildWorkflowTemplateSnapshot(template, assignments)
		}
	} else {
		// Audit/disabled rollout modes preserve the legacy creation path. New
		// workflow writes only begin after an administrator explicitly enables
		// EVALUATION_WORKFLOW_MODE=enabled.
		var scorers map[string]uint
		scorers, err = resolveTemplateScorers(models.DB, employee, template)
		if err == nil {
			snapshot, err = buildEvaluationTemplateSnapshotWithScorers(template, scorers)
		}
	}
	if err != nil {
		var resolutionErr *scorerResolutionError
		if errors.As(err, &resolutionErr) {
			respondError(c, http.StatusUnprocessableEntity, "SCORER_RESOLUTION_INCOMPLETE", "无法解析完整评分职责", gin.H{"missing": resolutionErr.Missing})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法解析评分人", "message": err.Error()})
		return
	}
	evaluation.TemplateID = template.ID
	evaluation.TemplateVersion = template.Version
	evaluation.CalculationVersion = template.CalculationVersion
	if workflowWrites {
		evaluation.CalculationVersion = canonicalCalculationVersion
		evaluation.WorkSummaryRequired = true
	}
	evaluation.TemplateSnapshotJSON = snapshot
	if evaluation.Status == "" {
		if workflowWrites {
			evaluation.Status = "self_evaluating"
		} else {
			evaluation.Status = "pending"
		}
	}

	// 开始数据库事务
	tx := models.DB.Begin()

	// 创建评估记录
	result := tx.Create(&evaluation)
	if result.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "创建评估失败",
			"message": result.Error.Error(),
		})
		return
	}

	// 获取模板的所有KPI项目
	var items []models.KPIItem
	if workflowWrites {
		parsedSnapshot, parseErr := parseEvaluationSnapshot(evaluation.TemplateSnapshotJSON)
		if parseErr != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取新版模板快照失败", "message": parseErr.Error()})
			return
		}
		for _, item := range parsedSnapshot.Items {
			items = append(items, models.KPIItem{ID: item.ID, Name: item.Name, Description: item.Description, MaxScore: item.MaxScore, ItemWeight: item.ItemWeight, CompositeWeight: item.CompositeWeight, Required: item.Required, ScoringGuidance: item.ScoringGuidance, DataSource: item.DataSource})
		}
	} else if err := json.Unmarshal([]byte(evaluation.TemplateSnapshotJSON), &struct {
		Items *[]models.KPIItem `json:"items"`
	}{Items: &items}); err != nil || len(items) == 0 {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取模板快照失败"})
		return
	}
	if len(items) == 0 {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "模板快照没有考核指标"})
		return
	}

	// 为每个KPI项目创建评分记录
	for _, item := range items {
		score := models.KPIScore{
			EvaluationID: evaluation.ID,
			ItemID:       item.ID,
		}
		if err := tx.Create(&score).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "创建评分记录失败",
				"message": err.Error(),
			})
			return
		}
	}
	if workflowWrites {
		parsedSnapshot, parseErr := parseEvaluationSnapshot(evaluation.TemplateSnapshotJSON)
		if parseErr != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取新版模板快照失败", "message": parseErr.Error()})
			return
		}
		if _, taskErr := generateEvaluationScoringTasks(tx, evaluation, parsedSnapshot); taskErr != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建评分任务失败", "message": taskErr.Error()})
			return
		}
	} else if _, invitationErr := ensureTemplateInvitations(tx, evaluation, c.GetUint("user_id")); invitationErr != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建模板评分任务失败", "message": invitationErr.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
		respondError(c, http.StatusInternalServerError, "ASSESSMENT_CREATE_FAILED", "创建考核失败", nil)
		return
	}

	// 获取完整的评估信息
	models.DB.Preload("Employee.Department").Preload("Template").Preload("Scores").First(&evaluation, evaluation.ID)

	// 发送 DooTask 机器人通知：创建考核 -> 员工待自评
	if evaluation.Employee.DooTaskUserID != nil {
		dooTaskClient := utils.NewDooTaskClient(c.GetHeader("DooTaskAuth"))
		periodValue := utils.GetPeriodValue(evaluation.Period, evaluation.Year, evaluation.Month, evaluation.Quarter)

		appConfigJSON := utils.BuildKPIAppConfig(evaluation.ID)

		message := fmt.Sprintf(
			"**你有一条新的绩效考核**\n- 考核模板：%s\n- 考核周期：%s\n- 被评估员工：%s\n\n> <div class=\"open-micro-app\" data-app-config='%s'>查看详情：点击查看详情</div>",
			evaluation.Template.Name,
			periodValue,
			evaluation.Employee.Name,
			appConfigJSON,
		)

		_ = dooTaskClient.SendBotMessage(evaluation.Employee.DooTaskUserID, message)
	}

	// 发送实时通知
	operatorID := c.GetUint("user_id")
	GetNotificationService().SendNotification(operatorID, EventEvaluationCreated, &evaluation)

	response := gin.H{"evaluation": evaluation}
	if parsed, parseErr := parseEvaluationSnapshot(evaluation.TemplateSnapshotJSON); parseErr == nil {
		response["template_version"] = parsed.TemplateVersion
		response["source_workbook_sha256"] = parsed.SourceSHA256
		response["scorer_snapshot"] = parsed.ResolvedScorers
		response["signoff_snapshot"] = parsed.SignoffRules
		response["calculation_version"] = parsed.CalculationVersion
		response["snapshot_checksum"] = parsed.Checksum
	}
	// Keep the historical `data.id`, `data.status`, ... shape while exposing
	// immutable snapshot metadata for clients that need an audit link.
	data := map[string]any{}
	rawEvaluation, _ := json.Marshal(evaluation)
	_ = json.Unmarshal(rawEvaluation, &data)
	for key, value := range response {
		if key != "evaluation" {
			data[key] = value
		}
	}
	c.JSON(http.StatusCreated, gin.H{"message": "评估创建成功", "data": data})
}

func validateEvaluationPeriod(evaluation models.KPIEvaluation) error {
	if evaluation.Year < 1900 || evaluation.Year > 9999 {
		return errors.New("考核年份必须在1900到9999之间")
	}
	switch evaluation.Period {
	case "monthly":
		if evaluation.Month == nil || *evaluation.Month < 1 || *evaluation.Month > 12 || evaluation.Quarter != nil {
			return errors.New("月度考核必须提供1到12的月份，且不能提供季度")
		}
	case "quarterly":
		if evaluation.Quarter == nil || *evaluation.Quarter < 1 || *evaluation.Quarter > 4 || evaluation.Month != nil {
			return errors.New("季度考核必须提供1到4的季度，且不能提供月份")
		}
	case "yearly":
		if evaluation.Month != nil || evaluation.Quarter != nil {
			return errors.New("年度考核不能提供月份或季度")
		}
	default:
		return errors.New("考核周期必须为monthly、quarterly或yearly")
	}
	return nil
}

// 获取单个评估
func GetEvaluation(c *gin.Context) {
	id := c.Param("id")
	evaluationId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的评估ID",
		})
		return
	}

	var evaluation models.KPIEvaluation
	result := ApplyEvaluationScope(models.DB.Preload("Employee.Department").Preload("Template").Preload("Scores.Item"), c.GetUint("user_id")).First(&evaluation, evaluationId)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "评估不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": evaluation,
	})
}

// 更新评估
func UpdateEvaluation(c *gin.Context) {
	id := c.Param("id")
	evaluationId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的评估ID",
		})
		return
	}

	var evaluation models.KPIEvaluation
	result := ApplyEvaluationScope(models.DB.Preload("Employee"), c.GetUint("user_id")).First(&evaluation, evaluationId)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "评估不存在",
		})
		return
	}
	if evaluation.Status == "completed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "已完成的考核不能修改"})
		return
	}

	var updateData models.KPIEvaluation
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}
	if isNewWorkflowEvaluation(evaluation) {
		// New workflow assessments advance through the backend state machine and
		// derive totals from scoring tasks. Keep this compatibility endpoint
		// limited to the non-state metadata that older clients may still send.
		if updateData.Status != "" || updateData.TotalScore != 0 || updateData.EmployeeID != 0 || updateData.TemplateID != 0 {
			respondError(c, http.StatusConflict, "WORKFLOW_UPDATE_NOT_ALLOWED", "新版考核不能直接修改状态或总分，请使用流程推进和评分任务接口", nil)
			return
		}
		updates := map[string]any{"final_comment": updateData.FinalComment, "has_objection": updateData.HasObjection, "objection_reason": updateData.ObjectionReason}
		if projectName := strings.TrimSpace(updateData.ProjectName); projectName != "" {
			updates["project_name"] = projectName
		}
		if updateData.AllocationRatio != nil {
			if math.IsNaN(*updateData.AllocationRatio) || math.IsInf(*updateData.AllocationRatio, 0) || *updateData.AllocationRatio < 0 || *updateData.AllocationRatio > 1 {
				respondError(c, http.StatusBadRequest, "INVALID_ALLOCATION_RATIO", "项目投入占比必须为0到1之间的有限数值", nil)
				return
			}
			updates["allocation_ratio"] = *updateData.AllocationRatio
		}
		if err := models.DB.Model(&evaluation).Updates(updates).Error; err != nil {
			respondError(c, http.StatusInternalServerError, "WORKFLOW_UPDATE_FAILED", "更新考核信息失败", nil)
			return
		}
		respondData(c, http.StatusOK, evaluation, "考核信息已更新")
		return
	}
	if updateData.Status == "completed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请使用最终结果确认接口完成考核"})
		return
	}

	// 特殊处理：如果状态是self_evaluated，检查员工是否有主管
	// 注意：由于在UpdateSelfScore中已经限制了所有员工（包括HR）必须有直属上级才能开始自评
	// 所以这里理论上不应该出现没有主管的情况，但为了数据一致性，仍然进行检查
	if updateData.Status == "self_evaluated" {
		// 检查员工是否有直属主管
		if evaluation.Employee.ManagerID == nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "暂无直属上级，请联系HR",
			})
			return
		}
	}

	result = models.DB.Model(&evaluation).Updates(updateData)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "更新评估失败",
			"message": result.Error.Error(),
		})
		return
	}

	// 当流程进入等待HR审核阶段时，根据配置自动计算HR评分
	if updateData.Status == "manager_evaluated" {
		// 检查绩效规则是否启用
		var rule models.PerformanceRule
		if err := models.DB.First(&rule).Error; err == nil && rule.Enabled {
			// 如果规则已启用，检查是否所有邀请都已完成
			allInvitationsCompleted, err := areAllInvitationsCompleted(evaluation.ID)
			if err == nil && allInvitationsCompleted {
				// 所有邀请都已完成，自动计算HR评分
				if err := applyPerformanceRuleForEvaluation(evaluation.ID); err == nil {
					// 如果绩效规则应用成功，自动将状态推进到pending_confirm
					models.DB.Model(&evaluation).Update("status", "pending_confirm")
					updateData.Status = "pending_confirm"
				}
			}
			// 如果有未完成的邀请，保持manager_evaluated状态，等待所有邀请完成
		}
		// 如果规则未启用或应用失败，保持manager_evaluated状态，等待HR手动审核
	}

	// 如果状态变为completed，自动计算最终得分
	if updateData.Status == "completed" {
		var scores []models.KPIScore
		if err := models.DB.Where("evaluation_id = ?", evaluation.ID).Find(&scores).Error; err == nil {
			// 更新各项目的final_score
			for _, s := range scores {
				var final float64
				if s.HRScore != nil {
					final = *s.HRScore
				} else if s.ManagerScore != nil {
					final = *s.ManagerScore
				} else if s.SelfScore != nil {
					final = *s.SelfScore
				}
				models.DB.Model(&s).Update("final_score", final)
			}

			// 如果前端发送了total_score（说明可能已经过异议处理调整），使用前端发送的值
			// 否则，如果存在异议处理（有final_comment），保持现有的total_score
			// 否则，重新计算total_score
			if updateData.TotalScore > 0 {
				// 使用前端发送的total_score（已通过异议处理调整）
				models.DB.Model(&evaluation).Update("total_score", updateData.TotalScore)
			} else if evaluation.FinalComment != "" {
				// 存在异议处理，保持现有的total_score
				// 不更新total_score
			} else {
				// 没有异议处理，正常计算最终得分
				total := 0.0
				for _, s := range scores {
					var final float64
					if s.HRScore != nil {
						final = *s.HRScore
					} else if s.ManagerScore != nil {
						final = *s.ManagerScore
					} else if s.SelfScore != nil {
						final = *s.SelfScore
					}
					total += final
				}
				models.DB.Model(&evaluation).Update("total_score", total)
			}
		}
	}

	// 重新加载更新后的数据
	models.DB.Preload("Employee.Manager").Preload("Template").First(&evaluation, evaluationId)

	// 根据状态变更自动创建绩效评论
	if updateData.Status != "" {
		var scores []models.KPIScore
		if err := models.DB.Where("evaluation_id = ?", evaluationId).Find(&scores).Error; err == nil {
			switch updateData.Status {
			case "self_evaluated":
				// 计算员工自评总分
				totalSelfScore := 0.0
				for _, s := range scores {
					if s.SelfScore != nil {
						totalSelfScore += *s.SelfScore
					}
				}
				// 创建自动评论：员工自评，总分X
				commentContent := fmt.Sprintf("员工自评，总分%s", formatScore(totalSelfScore))
				if err := createAutoComment(evaluation.ID, evaluation.EmployeeID, commentContent); err != nil {
					// 评论创建失败不影响主流程，仅记录错误
					fmt.Printf("创建员工自评自动评论失败: %v\n", err)
				}

			case "manager_evaluated":
				// 计算主管评分总分
				totalManagerScore := 0.0
				for _, s := range scores {
					if s.ManagerScore != nil {
						totalManagerScore += *s.ManagerScore
					}
				}
				// 创建自动评论：主管评分，总分X
				if evaluation.Employee.ManagerID != nil {
					commentContent := fmt.Sprintf("主管评分，总分%s", formatScore(totalManagerScore))
					if err := createAutoComment(evaluation.ID, *evaluation.Employee.ManagerID, commentContent); err != nil {
						fmt.Printf("创建主管评分自动评论失败: %v\n", err)
					}
				}

			case "pending_confirm":
				// 只有在HR手动审核时才创建评论（绩效规则自动计算的情况在applyPerformanceRuleForEvaluation中处理）
				// 检查是否是通过绩效规则自动计算的（通过检查是否有HR评分且HR评论是自动计算的）
				isAutoCalculated := false
				for _, s := range scores {
					if s.HRScore != nil && s.HRComment == autoHRScoreComment {
						isAutoCalculated = true
						break
					}
				}
				// 如果不是自动计算的，说明是HR手动审核，创建评论
				if !isAutoCalculated {
					// 计算HR评分总分（使用TotalScore或HRScore之和）
					totalHRScore := evaluation.TotalScore
					if totalHRScore == 0 {
						// 如果TotalScore为0，计算HRScore之和
						for _, s := range scores {
							if s.HRScore != nil {
								totalHRScore += *s.HRScore
							}
						}
					}
					// 创建自动评论：HR评分，总分X
					operatorID := c.GetUint("user_id")
					commentContent := fmt.Sprintf("HR评分，总分%s", formatScore(totalHRScore))
					if err := createAutoComment(evaluation.ID, operatorID, commentContent); err != nil {
						fmt.Printf("创建HR评分自动评论失败: %v\n", err)
					}
				}
			}
		}
	}

	// 发送DooTask机器人通知（根据状态变更）
	if updateData.Status != "" {
		dooTaskClient := utils.NewDooTaskClient(c.GetHeader("DooTaskAuth"))
		periodValue := utils.GetPeriodValue(evaluation.Period, evaluation.Year, evaluation.Month, evaluation.Quarter)

		switch updateData.Status {
		case "self_evaluated":
			// 完成自评：通知主管
			if evaluation.Employee.Manager != nil && evaluation.Employee.Manager.DooTaskUserID != nil {
				appConfigJSON := utils.BuildKPIAppConfig(evaluation.ID)

				message := fmt.Sprintf(
					"**你有一条绩效考核待评估**\n- 考核模板：%s\n- 考核周期：%s\n- 部门员工：%s\n\n> <div class=\"open-micro-app\" data-app-config='%s'>查看详情：点击查看详情</div>",
					evaluation.Template.Name,
					periodValue,
					evaluation.Employee.Name,
					appConfigJSON,
				)
				_ = dooTaskClient.SendBotMessage(evaluation.Employee.Manager.DooTaskUserID, message)
			}

		case "manager_evaluated":
			// 完成主管评分：如仍处于待HR审核阶段，则通知HR
			// （当启用了绩效规则且自动推进到 pending_confirm 时，这里不会进入）
			appConfigJSON := utils.BuildKPIAppConfig(evaluation.ID)

			hrUserIDs := GetNotificationService().GetAllHRUsers()
			for _, hrID := range hrUserIDs {
				var hr models.Employee
				if err := models.DB.First(&hr, hrID).Error; err != nil || hr.DooTaskUserID == nil {
					continue
				}

				message := fmt.Sprintf(
					"**你有一条绩效考核待审核**\n- 考核模板：%s\n- 考核周期：%s\n- 部门员工：%s\n\n> <div class=\"open-micro-app\" data-app-config='%s'>查看详情：点击查看详情</div>",
					evaluation.Template.Name,
					periodValue,
					evaluation.Employee.Name,
					appConfigJSON,
				)

				_ = dooTaskClient.SendBotMessage(hr.DooTaskUserID, message)
			}

		case "pending_confirm":
			// 完成审核（HR人工或规则自动）：通知员工确认
			if evaluation.Employee.DooTaskUserID != nil {
				appConfigJSON := utils.BuildKPIAppConfig(evaluation.ID)

				message := fmt.Sprintf(
					"**你的绩效已审核完成，请确认结果**\n- 考核模板：%s\n- 考核周期：%s\n- 总分：%.1f\n\n> <div class=\"open-micro-app\" data-app-config='%s'>查看详情：点击查看详情</div>",
					evaluation.Template.Name,
					periodValue,
					evaluation.TotalScore,
					appConfigJSON,
				)
				_ = dooTaskClient.SendBotMessage(evaluation.Employee.DooTaskUserID, message)
			}

		case "completed":
			// 员工确认完成：通知 HR
			appConfigJSON := utils.BuildKPIAppConfig(evaluation.ID)

			hrUserIDs := GetNotificationService().GetAllHRUsers()
			for _, hrID := range hrUserIDs {
				var hr models.Employee
				if err := models.DB.First(&hr, hrID).Error; err != nil || hr.DooTaskUserID == nil {
					continue
				}

				message := fmt.Sprintf(
					"**绩效已完成**\n- 考核模板：%s\n- 考核周期：%s\n- 部门员工：%s\n- 总分：%.1f\n\n> <div class=\"open-micro-app\" data-app-config='%s'>查看详情：点击查看详情</div>",
					evaluation.Template.Name,
					periodValue,
					evaluation.Employee.Name,
					evaluation.TotalScore,
					appConfigJSON,
				)

				_ = dooTaskClient.SendBotMessage(hr.DooTaskUserID, message)
			}
		}
	}

	// 发送实时通知
	operatorID := c.GetUint("user_id")
	if updateData.Status != "" {
		// 状态变更通知
		GetNotificationService().SendNotification(operatorID, EventEvaluationStatusChange, &evaluation)
	} else {
		// 一般更新通知
		GetNotificationService().SendNotification(operatorID, EventEvaluationUpdated, &evaluation)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "评估更新成功",
		"data":    evaluation,
	})
}

// 删除评估
func DeleteEvaluation(c *gin.Context) {
	id := c.Param("id")
	evaluationId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的评估ID",
		})
		return
	}

	userID := c.GetUint("user_id")
	if !CanAccessEvaluation(userID, uint(evaluationId)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "该评估超出数据范围"})
		return
	}

	// 在删除前获取评估信息用于通知和清理纸质签字附件
	var evaluation models.KPIEvaluation
	if err := models.DB.Preload("Employee").First(&evaluation, evaluationId).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "评估不存在",
		})
		return
	}

	type attachmentBackup struct {
		original string
		staged   string
		content  []byte
		mode     os.FileMode
	}
	var backups []attachmentBackup
	stageDir := ""
	tx := models.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除评估失败", "message": tx.Error.Error()})
		return
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		tx.Rollback()
		for _, backup := range backups {
			if _, statErr := os.Stat(backup.original); statErr == nil {
				continue
			}
			if _, statErr := os.Stat(backup.staged); statErr == nil {
				if renameErr := os.Rename(backup.staged, backup.original); renameErr == nil {
					continue
				}
			}
			_ = os.WriteFile(backup.original, backup.content, backup.mode)
		}
		if stageDir != "" {
			_ = os.RemoveAll(stageDir)
		}
	}()

	var confirmations []models.EvaluationConfirmation
	if err = tx.Where("evaluation_id = ?", evaluationId).Find(&confirmations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除评估失败", "message": err.Error()})
		return
	}
	attachmentNames := make([]string, 0, len(confirmations))
	for _, confirmation := range confirmations {
		if name := strings.TrimSpace(filepath.Base(confirmation.AttachmentStoredName)); name != "" && name != "." {
			attachmentNames = append(attachmentNames, name)
		}
	}
	var supplements []models.EvaluationSignoffSupplement
	if err = tx.Where("evaluation_id = ?", evaluationId).Find(&supplements).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除评估失败", "message": err.Error()})
		return
	}
	for _, supplement := range supplements {
		if name := strings.TrimSpace(filepath.Base(supplement.AttachmentStoredName)); name != "" && name != "." {
			attachmentNames = append(attachmentNames, name)
		}
	}
	if len(attachmentNames) > 0 {
		uploadDir := signoffUploadDir()
		stageDir = filepath.Join(uploadDir, ".deleting-"+uuid.NewString())
		if err = os.MkdirAll(stageDir, 0750); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "删除评估附件失败", "message": err.Error()})
			return
		}
		for _, name := range attachmentNames {
			original := filepath.Join(uploadDir, name)
			info, statErr := os.Stat(original)
			if errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			if statErr != nil || !info.Mode().IsRegular() {
				if statErr == nil {
					statErr = fmt.Errorf("附件不是普通文件: %s", name)
				}
				err = statErr
				c.JSON(http.StatusInternalServerError, gin.H{"error": "删除评估附件失败", "message": err.Error()})
				return
			}
			content, readErr := os.ReadFile(original)
			if readErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "删除评估附件失败", "message": readErr.Error()})
				return
			}
			staged := filepath.Join(stageDir, uuid.NewString())
			if renameErr := os.Rename(original, staged); renameErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "删除评估附件失败", "message": renameErr.Error()})
				return
			}
			backups = append(backups, attachmentBackup{original: original, staged: staged, content: content, mode: info.Mode().Perm()})
		}
	}

	// Delete children explicitly because SQLite does not enable foreign-key
	// cascades for every existing deployment. Keep the operation transactional
	// so a failed child delete cannot leave an orphaned assessment.
	childDeletes := []struct {
		model any
		where string
		args  []any
	}{
		{&models.EvaluationScoringTask{}, "evaluation_id = ?", []any{evaluationId}},
		{&models.EvaluationCalculationResult{}, "evaluation_id = ?", []any{evaluationId}},
		{&models.EvaluationResultSnapshot{}, "evaluation_id = ?", []any{evaluationId}},
		{&models.EvaluationConfirmation{}, "evaluation_id = ?", []any{evaluationId}},
		{&models.EvaluationSignoffSupplement{}, "evaluation_id = ?", []any{evaluationId}},
		{&models.EvaluationComment{}, "evaluation_id = ?", []any{evaluationId}},
		{&models.KPIScore{}, "evaluation_id = ?", []any{evaluationId}},
		{&models.InvitedScore{}, "invitation_id IN (SELECT id FROM evaluation_invitations WHERE evaluation_id = ?)", []any{evaluationId}},
		{&models.EvaluationInvitation{}, "evaluation_id = ?", []any{evaluationId}},
	}
	for _, child := range childDeletes {
		if result := tx.Where(child.where, child.args...).Delete(child.model); result.Error != nil {
			err = result.Error
			c.JSON(http.StatusInternalServerError, gin.H{"error": "删除评估失败", "message": err.Error()})
			return
		}
	}
	if result := tx.Delete(&models.KPIEvaluation{}, evaluationId); result.Error != nil || result.RowsAffected != 1 {
		err = result.Error
		if err == nil {
			err = gorm.ErrRecordNotFound
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "评估不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除评估失败", "message": err.Error()})
		return
	}
	for _, backup := range backups {
		if removeErr := os.Remove(backup.staged); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "删除评估附件失败", "message": removeErr.Error()})
			return
		}
	}
	if err = tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除评估失败", "message": err.Error()})
		return
	}
	committed = true
	if stageDir != "" {
		_ = os.RemoveAll(stageDir)
	}

	RecordAuditDetails(c, "delete_evaluation", "evaluation", strconv.FormatUint(evaluationId, 10), "SUCCESS", fmt.Sprintf("attachments=%d", len(backups)))

	// 发送实时通知
	operatorID := c.GetUint("user_id")
	GetNotificationService().SendNotification(operatorID, EventEvaluationDeleted, &evaluation)

	c.JSON(http.StatusOK, gin.H{
		"message": "评估删除成功",
	})
}

// 获取员工的评估记录
func GetEmployeeEvaluations(c *gin.Context) {
	employeeId := c.Param("employeeId")
	empId, err := strconv.ParseUint(employeeId, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的员工ID",
		})
		return
	}

	var evaluations []models.KPIEvaluation
	result := ApplyEvaluationScope(models.DB.Preload("Template").Preload("Scores"), c.GetUint("user_id")).Where("kpi_evaluations.employee_id = ?", empId).Find(&evaluations)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取员工评估记录失败",
			"message": result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  evaluations,
		"total": len(evaluations),
	})
}

// 获取待处理的评估
func GetPendingEvaluations(c *gin.Context) {
	employeeId := c.Param("employeeId")
	empId, err := strconv.ParseUint(employeeId, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的员工ID",
		})
		return
	}

	var evaluations []models.KPIEvaluation

	// 获取需要当前员工处理的评估
	if uint(empId) != c.GetUint("user_id") {
		respondError(c, http.StatusForbidden, "ASSESSMENT_SCOPE_FORBIDDEN", "只能查询自己的待办考核", nil)
		return
	}
	result := ApplyEvaluationScope(models.DB.Preload("Employee.Department").Preload("Template"), uint(empId)).Where("((kpi_evaluations.employee_id = ? AND status IN ?) OR (status IN ? AND kpi_evaluations.id IN (SELECT evaluation_id FROM evaluation_scoring_tasks WHERE scorer_user_id = ? AND status IN ?)))", empId, []string{"pending", "self_evaluating", "pending_confirm", "awaiting_employee_confirmation"}, []string{"scoring", "awaiting_review"}, empId, []string{"pending", "in_progress", "rejected"}).Find(&evaluations)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取待处理评估失败",
			"message": result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  evaluations,
		"total": len(evaluations),
	})
}

// 获取待确认评估数量
func GetPendingCountEvaluations(c *gin.Context) {
	userID := c.GetUint("user_id")

	var totalCount int64

	// 所有角色：自己的待自评 + 待确认
	if err := models.DB.Model(&models.KPIEvaluation{}).
		Where("employee_id = ? AND status IN ?", userID, []string{"pending", "self_evaluating", "pending_confirm", "awaiting_employee_confirmation"}).
		Count(&totalCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取待确认评估数量失败"})
		return
	}
	var assignedCount int64
	if err := models.DB.Model(&models.KPIEvaluation{}).Where("status IN ? AND id IN (SELECT evaluation_id FROM evaluation_scoring_tasks WHERE scorer_user_id = ? AND status IN ?)", []string{"scoring", "awaiting_review"}, userID, []string{"pending", "in_progress", "rejected"}).Count(&assignedCount).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "PENDING_COUNT_FAILED", "获取评分待办数量失败", nil)
		return
	}
	totalCount += assignedCount

	// 具备评分权限的用户：增加其数据范围内的待评分记录。
	if UserHasPermission(userID, "assessment:review") {
		var deptSelfEvaluatedCount int64
		if err := ApplyEvaluationScope(models.DB.Model(&models.KPIEvaluation{}), userID).
			Where("kpi_evaluations.employee_id <> ? AND kpi_evaluations.status = ?", userID, "self_evaluated").
			Count(&deptSelfEvaluatedCount).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取待确认评估数量失败"})
			return
		}
		totalCount += deptSelfEvaluatedCount
	}

	// 具备审核权限的用户：增加其数据范围内的待审核记录。
	if UserHasPermission(userID, "assessment:approve") {
		var managerEvaluatedCount int64
		if err := ApplyEvaluationScope(models.DB.Model(&models.KPIEvaluation{}), userID).
			Where("kpi_evaluations.status = ?", "manager_evaluated").
			Count(&managerEvaluatedCount).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取待确认评估数量失败"})
			return
		}
		totalCount += managerEvaluatedCount
	}

	c.JSON(http.StatusOK, gin.H{
		"count": totalCount,
	})
}

// 获取评估的评分记录
func GetEvaluationScores(c *gin.Context) {
	evaluationId := c.Param("evaluationId")
	evalId, err := strconv.ParseUint(evaluationId, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的评估ID",
		})
		return
	}

	var scores []models.KPIScore
	if !CanAccessEvaluation(c.GetUint("user_id"), uint(evalId)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "超出数据范围"})
		return
	}
	result := models.DB.Preload("Item").Where("evaluation_id = ?", evalId).Find(&scores)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取评分记录失败",
			"message": result.Error.Error(),
		})
		return
	}
	var evaluation models.KPIEvaluation
	if models.DB.First(&evaluation, evalId).Error == nil && isNewWorkflowEvaluation(evaluation) {
		snapshot, err := parseWorkflowSnapshotForEvaluation(evaluation)
		if err != nil {
			respondError(c, http.StatusConflict, "SNAPSHOT_INVALID", err.Error(), nil)
			return
		}
		items := map[uint]workflowSnapshotItem{}
		for _, item := range snapshot.Items {
			items[item.ID] = item
		}
		for i := range scores {
			if item, ok := items[scores[i].ItemID]; ok {
				scores[i].Item = models.KPIItem{ID: item.ID, Name: item.Name, Description: item.Description, MaxScore: item.MaxScore, Required: item.Required, ItemWeight: item.ItemWeight, CompositeWeight: item.CompositeWeight, ScorerTypes: item.ScorerTypes}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  scores,
		"total": len(scores),
	})
}

// 更新自评分数
func UpdateSelfScore(c *gin.Context) {
	id := c.Param("id")
	scoreId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的评分ID",
		})
		return
	}

	var score models.KPIScore
	result := models.DB.Preload("Evaluation.Employee").Preload("Item").First(&score, scoreId)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "评分记录不存在",
		})
		return
	}
	if score.Evaluation.EmployeeID != c.GetUint("user_id") {
		respondError(c, http.StatusForbidden, "SELF_SCORE_FORBIDDEN", "只能填写自己的考核自评", nil)
		return
	}
	weighted := isNewWorkflowEvaluation(score.Evaluation)
	editableState := "pending"
	if weighted {
		editableState = workflowStateSelfEvaluating
	}
	if score.Evaluation.Status != editableState {
		respondError(c, http.StatusConflict, "SELF_SCORE_LOCKED", "当前考核已不在自评阶段，不能修改自评分数", nil)
		return
	}
	if weighted && score.Evaluation.WorkSummaryRequired && score.Evaluation.WorkSummarySubmittedAt == nil {
		respondError(c, http.StatusConflict, "WORK_SUMMARY_REQUIRED", "请先提交工作总结与后续计划", nil)
		return
	}

	var updateData struct {
		SelfScore   *float64 `json:"self_score"`
		SelfComment string   `json:"self_comment"`
	}

	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}

	maxScore := score.Item.MaxScore
	if weighted {
		snapshot, snapshotErr := parseWorkflowSnapshotForEvaluation(score.Evaluation)
		if snapshotErr != nil {
			respondError(c, http.StatusConflict, "SNAPSHOT_INVALID", snapshotErr.Error(), nil)
			return
		}
		found := false
		for _, item := range snapshot.Items {
			if item.ID == score.ItemID {
				maxScore = item.MaxScore
				found = true
				break
			}
		}
		if !found {
			respondError(c, http.StatusConflict, "SNAPSHOT_ITEM_MISSING", "自评指标不在考核快照内", nil)
			return
		}
	}
	if updateData.SelfScore != nil && (math.IsNaN(*updateData.SelfScore) || math.IsInf(*updateData.SelfScore, 0) || *updateData.SelfScore < 0 || *updateData.SelfScore > maxScore) {
		respondError(c, http.StatusBadRequest, "SCORE_OUT_OF_RANGE", fmt.Sprintf("评分必须在0到%g之间", maxScore), nil)
		return
	}
	if !weighted && score.SelfScore == nil && updateData.SelfScore != nil {
		// 检查被评估员工是否有直属上级（所有角色都需要检查）
		if score.Evaluation.Employee.ManagerID == nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "暂无直属上级，请联系HR",
			})
			return
		}
	}

	result = models.DB.Model(&score).Where("EXISTS (SELECT 1 FROM kpi_evaluations WHERE kpi_evaluations.id = kpi_scores.evaluation_id AND employee_id = ? AND status = ?)", c.GetUint("user_id"), editableState).Updates(map[string]interface{}{
		"self_score":   updateData.SelfScore,
		"self_comment": updateData.SelfComment,
	})

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "更新自评分数失败",
			"message": result.Error.Error(),
		})
		return
	}
	if result.RowsAffected == 0 {
		respondError(c, http.StatusConflict, "SELF_SCORE_LOCKED", "考核状态已变化，请刷新后重试", nil)
		return
	}
	score.SelfScore, score.SelfComment = updateData.SelfScore, updateData.SelfComment

	// 发送实时通知
	operatorID := c.GetUint("user_id")
	GetNotificationService().SendNotification(operatorID, EventSelfScoreUpdated, &score)

	c.JSON(http.StatusOK, gin.H{
		"message": "自评分数更新成功",
		"data":    score,
	})
}

// 更新上级评分
func UpdateManagerScore(c *gin.Context) {
	id := c.Param("id")
	scoreId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的评分ID",
		})
		return
	}

	var score models.KPIScore
	result := models.DB.First(&score, scoreId)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "评分记录不存在",
		})
		return
	}
	if err := ensureEvaluationNotCompleted(score.EvaluationID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var updateData struct {
		ManagerScore   *float64 `json:"manager_score"`
		ManagerComment string   `json:"manager_comment"`
	}

	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}

	result = models.DB.Model(&score).Updates(map[string]interface{}{
		"manager_score":   updateData.ManagerScore,
		"manager_comment": updateData.ManagerComment,
	})

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "更新上级评分失败",
			"message": result.Error.Error(),
		})
		return
	}

	// 发送实时通知
	operatorID := c.GetUint("user_id")
	GetNotificationService().SendNotification(operatorID, EventManagerScoreUpdated, &score)

	c.JSON(http.StatusOK, gin.H{
		"message": "上级评分更新成功",
		"data":    score,
	})
}

// 更新HR评分
func UpdateHRScore(c *gin.Context) {
	id := c.Param("id")
	scoreId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的评分ID",
		})
		return
	}

	var score models.KPIScore
	result := models.DB.First(&score, scoreId)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "评分记录不存在",
		})
		return
	}
	if err := ensureEvaluationNotCompleted(score.EvaluationID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var updateData struct {
		HRScore   *float64 `json:"hr_score"`
		HRComment string   `json:"hr_comment"`
	}

	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}

	result = models.DB.Model(&score).Updates(map[string]interface{}{
		"hr_score":   updateData.HRScore,
		"hr_comment": updateData.HRComment,
	})

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "更新HR评分失败",
			"message": result.Error.Error(),
		})
		return
	}

	// 发送实时通知
	operatorID := c.GetUint("user_id")
	GetNotificationService().SendNotification(operatorID, EventHRScoreUpdated, &score)

	c.JSON(http.StatusOK, gin.H{
		"message": "HR评分更新成功",
		"data":    score,
	})
}

// 更新最终得分
func UpdateFinalScore(c *gin.Context) {
	id := c.Param("id")
	scoreId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的评分ID",
		})
		return
	}

	var score models.KPIScore
	result := models.DB.First(&score, scoreId)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "评分记录不存在",
		})
		return
	}
	if err := ensureEvaluationNotCompleted(score.EvaluationID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var updateData struct {
		FinalScore   *float64 `json:"final_score"`
		FinalComment string   `json:"final_comment"`
	}

	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}

	result = models.DB.Model(&score).Updates(map[string]interface{}{
		"final_score":   updateData.FinalScore,
		"final_comment": updateData.FinalComment,
	})

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "更新最终得分失败",
			"message": result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "最终得分更新成功",
		"data":    score,
	})
}

// 员工提交异议
func SubmitObjection(c *gin.Context) {
	id := c.Param("id")
	evaluationId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的评估ID",
		})
		return
	}

	var evaluation models.KPIEvaluation
	result := models.DB.Preload("Employee").Preload("Employee.Manager").First(&evaluation, evaluationId)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "评估不存在",
		})
		return
	}

	// 检查权限：只有被考核员工本人可以提交异议
	userID := c.GetUint("user_id")
	if evaluation.EmployeeID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "无权操作此评估",
		})
		return
	}

	// 检查状态：旧流程只能在 pending_confirm 提交，新流程只能在统一确认态提交。
	if !objectionSubmissionAllowed(evaluation) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "只有在待确认状态才能提交异议",
		})
		return
	}

	// 检查是否已有异议（包括已处理的情况）
	if evaluation.HasObjection || evaluation.ObjectionReason != "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "已提交过异议，不可重复提交",
		})
		return
	}

	var objectionData struct {
		Reason string `json:"reason" binding:"required"`
	}

	if err := c.ShouldBindJSON(&objectionData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}

	// 更新评估，添加异议
	objectionUpdates := map[string]interface{}{
		"has_objection":    true,
		"objection_reason": objectionData.Reason,
	}
	if isNewWorkflowEvaluation(evaluation) {
		objectionUpdates["status"] = workflowStateRejected
	}
	result = models.DB.Model(&models.KPIEvaluation{}).
		Where("id = ? AND has_objection = ? AND status = ?", evaluation.ID, false, evaluation.Status).
		Updates(objectionUpdates)
	if result.Error == nil && result.RowsAffected == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "该考核状态已变化或异议已提交，请刷新后重试"})
		return
	}

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "提交异议失败",
			"message": result.Error.Error(),
		})
		return
	}
	RecordAuditDetails(c, "submit_objection", "evaluation", strconv.FormatUint(evaluationId, 10), "SUCCESS", "workflow_state="+evaluation.Status)

	// 重新加载评估数据
	models.DB.Preload("Employee").Preload("Employee.Manager").Preload("Template").First(&evaluation, evaluationId)

	// 发送 DooTask 机器人通知给主管和所有HR
	dooTaskClient := utils.NewDooTaskClient(c.GetHeader("DooTaskAuth"))
	periodValue := utils.GetPeriodValue(evaluation.Period, evaluation.Year, evaluation.Month, evaluation.Quarter)

	appConfigJSON := utils.BuildKPIAppConfig(evaluation.ID)

	// 通知主管（如果有）
	if evaluation.Employee.Manager != nil && evaluation.Employee.Manager.DooTaskUserID != nil {
		message := fmt.Sprintf(
			"**有绩效异议待处理**\n- 部门员工：%s\n- 考核模板：%s\n- 考核周期：%s\n- 异议原因：%s\n\n> <div class=\"open-micro-app\" data-app-config='%s'>查看详情：点击查看详情</div>",
			evaluation.Employee.Name,
			evaluation.Template.Name,
			periodValue,
			objectionData.Reason,
			appConfigJSON,
		)
		_ = dooTaskClient.SendBotMessage(evaluation.Employee.Manager.DooTaskUserID, message)
	}

	// 通知所有HR
	hrUserIDsForBot := GetNotificationService().GetAllHRUsers()
	for _, hrID := range hrUserIDsForBot {
		var hr models.Employee
		if err := models.DB.First(&hr, hrID).Error; err != nil || hr.DooTaskUserID == nil {
			continue
		}

		message := fmt.Sprintf(
			"**有绩效异议待处理**\n- 部门员工：%s\n- 考核模板：%s\n- 考核周期：%s\n- 异议原因：%s\n\n> <div class=\"open-micro-app\" data-app-config='%s'>查看详情：点击查看详情</div>",
			evaluation.Employee.Name,
			evaluation.Template.Name,
			periodValue,
			objectionData.Reason,
			appConfigJSON,
		)
		_ = dooTaskClient.SendBotMessage(hr.DooTaskUserID, message)
	}

	// 发送通知给上级和HR
	notificationService := GetNotificationService()

	// 通知上级（如果有）
	if evaluation.Employee.Manager != nil {
		notificationService.SendNotification(evaluation.Employee.Manager.ID, EventObjectionSubmitted, &evaluation)
	}

	// 通知所有HR
	hrUserIDs := notificationService.GetAllHRUsers()
	for _, hrID := range hrUserIDs {
		notificationService.SendNotification(hrID, EventObjectionSubmitted, &evaluation)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "异议提交成功",
		"data":    evaluation,
	})
}

// HR处理异议
func HandleObjection(c *gin.Context) {
	id := c.Param("id")
	evaluationId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的评估ID",
		})
		return
	}

	var evaluation models.KPIEvaluation
	result := models.DB.Preload("Employee").First(&evaluation, evaluationId)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "评估不存在",
		})
		return
	}

	// 检查是否有异议
	if !CanAccessEvaluation(c.GetUint("user_id"), evaluation.ID) {
		respondError(c, http.StatusForbidden, "ASSESSMENT_SCOPE_FORBIDDEN", "该考核超出数据范围", nil)
		return
	}
	if !evaluation.HasObjection {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "该评估没有异议需要处理",
		})
		return
	}
	if !objectionHandlingAllowed(evaluation) {
		c.JSON(http.StatusConflict, gin.H{"error": "当前考核状态不能处理异议"})
		return
	}

	var handleData struct {
		TotalScore   *float64 `json:"total_score"`
		FinalComment string   `json:"final_comment" binding:"required"`
	}

	if err := c.ShouldBindJSON(&handleData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}
	if !isNewWorkflowEvaluation(evaluation) && handleData.TotalScore == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "旧流程处理异议必须提供总分"})
		return
	}

	if isNewWorkflowEvaluation(evaluation) {
		// 新流程的异议只改变流程状态和说明，不能由客户端覆盖权威计算总分。
		confirmationCandidate := evaluation
		confirmationCandidate.Status = workflowStateAwaitingEmployeeConfirmation
		confirmationCandidate.HasObjection = false
		if failure := guardEvaluationConfirmation(models.DB, confirmationCandidate); failure != nil {
			respondError(c, failure.Status, failure.Code, failure.Message, failure.Details)
			return
		}
		tx := models.DB.Begin()
		if tx.Error != nil {
			respondError(c, http.StatusInternalServerError, "OBJECTION_HANDLE_FAILED", "处理异议失败", nil)
			return
		}
		if _, _, calcErr := persistCanonicalEvaluationResult(tx, evaluation.ID, c.GetUint("user_id")); calcErr != nil {
			tx.Rollback()
			respondError(c, http.StatusUnprocessableEntity, "RESULT_CALCULATION_FAILED", calcErr.Error(), nil)
			return
		}
		update := tx.Model(&models.KPIEvaluation{}).Where("id = ? AND status = ? AND has_objection = ?", evaluation.ID, workflowStateRejected, true).Updates(map[string]any{
			"has_objection": false,
			"final_comment": handleData.FinalComment,
			"status":        workflowStateAwaitingEmployeeConfirmation,
		})
		if update.Error != nil || update.RowsAffected == 0 || tx.Commit().Error != nil {
			tx.Rollback()
			if update.Error == nil && update.RowsAffected == 0 {
				respondError(c, http.StatusConflict, "OBJECTION_STATE_CONFLICT", "异议已被其他操作处理，请刷新后重试", nil)
				return
			}
			respondError(c, http.StatusInternalServerError, "OBJECTION_HANDLE_FAILED", "处理异议失败", nil)
			return
		}
		models.DB.Preload("Employee").Preload("Template").First(&evaluation, evaluation.ID)
		RecordAuditDetails(c, "handle_objection", "evaluation", strconv.FormatUint(evaluationId, 10), "SUCCESS", "workflow=awaiting_employee_confirmation;client_total_ignored=true")
		respondData(c, http.StatusOK, evaluation, "异议已处理，考核已回到员工确认")
		return
	}

	// 更新评估：处理异议，清除异议状态，更新最终得分和处理原因
	result = models.DB.Model(&models.KPIEvaluation{}).
		Where("id = ? AND status = ? AND has_objection = ?", evaluation.ID, "pending_confirm", true).
		Updates(map[string]interface{}{
			"has_objection": false,
			"total_score":   *handleData.TotalScore,
			"final_comment": handleData.FinalComment,
		})
	if result.Error == nil && result.RowsAffected == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "该考核状态已变化，请刷新后重试"})
		return
	}

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "处理异议失败",
			"message": result.Error.Error(),
		})
		return
	}
	RecordAuditDetails(c, "handle_objection", "evaluation", strconv.FormatUint(evaluationId, 10), "SUCCESS", "workflow=legacy_pending_confirm")

	// 重新加载评估数据
	models.DB.Preload("Employee").Preload("Template").First(&evaluation, evaluationId)

	// 发送 DooTask 机器人通知给员工（和可选主管）
	dooTaskClient := utils.NewDooTaskClient(c.GetHeader("DooTaskAuth"))
	periodValue := utils.GetPeriodValue(evaluation.Period, evaluation.Year, evaluation.Month, evaluation.Quarter)

	appConfigJSON := utils.BuildKPIAppConfig(evaluation.ID)

	// 通知员工
	if evaluation.Employee.DooTaskUserID != nil {
		message := fmt.Sprintf(
			"**你的绩效异议已处理，请重新确认**\n- 考核模板：%s\n- 考核周期：%s\n- 调整后总分：%.2f\n- 处理说明：%s\n\n> <div class=\"open-micro-app\" data-app-config='%s'>查看详情：点击查看详情</div>",
			evaluation.Template.Name,
			periodValue,
			*handleData.TotalScore,
			handleData.FinalComment,
			appConfigJSON,
		)
		_ = dooTaskClient.SendBotMessage(evaluation.Employee.DooTaskUserID, message)
	}

	// 通知主管（如果有）
	var employee models.Employee
	if err := models.DB.Preload("Manager").First(&employee, evaluation.EmployeeID).Error; err == nil {
		if employee.Manager != nil && employee.Manager.DooTaskUserID != nil {
			message := fmt.Sprintf(
				"**部门员工的绩效异议已处理**\n- 部门员工：%s\n- 考核模板：%s\n- 考核周期：%s\n- 调整后总分：%.2f\n\n> <div class=\"open-micro-app\" data-app-config='%s'>查看详情：点击查看详情</div>",
				employee.Name,
				evaluation.Template.Name,
				periodValue,
				*handleData.TotalScore,
				appConfigJSON,
			)
			_ = dooTaskClient.SendBotMessage(employee.Manager.DooTaskUserID, message)
		}
	}

	// 发送通知给员工
	GetNotificationService().SendNotification(evaluation.EmployeeID, EventObjectionHandled, &evaluation)

	c.JSON(http.StatusOK, gin.H{
		"message": "异议处理成功",
		"data":    evaluation,
	})
}

const (
	performanceScenarioNoInvitation       = "no_invitation"
	performanceScenarioEmployeeInvitation = "employee_invitation"

	autoHRScoreComment = "系统根据绩效规则自动计算"
)

// formatScore 格式化分数：整数显示为整数，小数保留2位小数
func formatScore(score float64) string {
	if score == float64(int64(score)) {
		return fmt.Sprintf("%.0f", score)
	}
	return fmt.Sprintf("%.2f", score)
}

// createAutoComment 自动创建绩效评论
func createAutoComment(evaluationID uint, userID uint, content string) error {
	comment := models.EvaluationComment{
		EvaluationID: evaluationID,
		UserID:       userID,
		Content:      content,
		IsPrivate:    false,
	}
	return models.DB.Create(&comment).Error
}

// areAllInvitationsCompleted 检查评估的所有邀请是否都已完成
// 完成状态包括：completed（已完成）、declined（已拒绝）、cancelled（已撤销）
// 未完成状态包括：pending（待接受）、accepted（已接受，进行中）
func areAllInvitationsCompleted(evaluationID uint) (bool, error) {
	var totalInvitations int64
	var completedInvitations int64

	// 统计所有邀请数量
	if err := models.DB.Model(&models.EvaluationInvitation{}).
		Where("evaluation_id = ?", evaluationID).
		Count(&totalInvitations).Error; err != nil {
		return false, err
	}

	// 如果没有邀请，认为已完成
	if totalInvitations == 0 {
		return true, nil
	}

	// 统计已完成的邀请数量（completed、declined、cancelled）
	if err := models.DB.Model(&models.EvaluationInvitation{}).
		Where("evaluation_id = ? AND status IN ?", evaluationID, []string{"completed", "declined", "cancelled"}).
		Count(&completedInvitations).Error; err != nil {
		return false, err
	}

	// 所有邀请都已完成
	return completedInvitations == totalInvitations, nil
}

type invitationAggregate struct {
	Average float64
	Count   int
}

type scoreComponent struct {
	weight  float64
	value   float64
	present bool
}

// applyPerformanceRuleForEvaluation 根据启用的绩效规则自动计算HR评分和总分
func applyPerformanceRuleForEvaluation(evaluationID uint) error {
	var rule models.PerformanceRule
	if err := models.DB.First(&rule).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	if !rule.Enabled {
		return nil
	}

	var evaluation models.KPIEvaluation
	if err := models.DB.Preload("Employee").First(&evaluation, evaluationID).Error; err != nil {
		return err
	}

	var scores []models.KPIScore
	if err := models.DB.Where("evaluation_id = ?", evaluationID).Find(&scores).Error; err != nil {
		return err
	}
	if len(scores) == 0 {
		return nil
	}

	var invitations []models.EvaluationInvitation
	if err := models.DB.
		Preload("Invitee").
		Preload("Scores").
		Where("evaluation_id = ? AND status = ?", evaluationID, "completed").
		Find(&invitations).Error; err != nil {
		return err
	}

	scenario, relevantInvitations := determinePerformanceRuleScenario(invitations)
	invitationAverages := buildInvitationAverages(relevantInvitations)

	tx := models.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	totalScore := 0.0
	updatedCount := 0

	for _, score := range scores {
		aggregate := invitationAverages[score.ItemID]
		hrScore, ok := calculateHRScoreByScenario(score, aggregate, scenario, rule)
		if !ok {
			continue
		}

		comment := score.HRComment
		if strings.TrimSpace(comment) == "" {
			comment = autoHRScoreComment
		}

		if err := tx.Model(&models.KPIScore{}).Where("id = ?", score.ID).Updates(map[string]interface{}{
			"hr_score":   hrScore,
			"hr_comment": comment,
		}).Error; err != nil {
			tx.Rollback()
			return err
		}

		totalScore += hrScore
		updatedCount++
	}

	if updatedCount > 0 {
		totalScore = math.Round(totalScore*100) / 100
		if err := tx.Model(&models.KPIEvaluation{}).Where("id = ?", evaluationID).Update("total_score", totalScore).Error; err != nil {
			tx.Rollback()
			return err
		}

		// 自动创建HR评分评论（系统自动计算）
		commentContent := fmt.Sprintf("HR评分，总分%s", formatScore(totalScore))
		comment := models.EvaluationComment{
			EvaluationID: evaluationID,
			UserID:       0, // 系统用户ID
			Content:      commentContent,
			IsPrivate:    false,
		}
		if err := tx.Create(&comment).Error; err != nil {
			// 评论创建失败不影响主流程，仅记录错误
			fmt.Printf("创建系统自动HR评分评论失败: %v\n", err)
		}
	}

	return tx.Commit().Error
}

// determinePerformanceRuleScenario 根据邀请情况确定使用的绩效规则场景
func determinePerformanceRuleScenario(invitations []models.EvaluationInvitation) (string, []models.EvaluationInvitation) {
	validInvitations := make([]models.EvaluationInvitation, 0, len(invitations))
	for _, invitation := range invitations {
		if invitationHasCompletedScore(invitation) {
			validInvitations = append(validInvitations, invitation)
		}
	}

	if len(validInvitations) == 0 {
		return performanceScenarioNoInvitation, nil
	}

	return performanceScenarioEmployeeInvitation, validInvitations
}

func invitationHasCompletedScore(invitation models.EvaluationInvitation) bool {
	for _, score := range invitation.Scores {
		if score.Score != nil {
			return true
		}
	}
	return false
}

func buildInvitationAverages(invitations []models.EvaluationInvitation) map[uint]invitationAggregate {
	aggregates := make(map[uint]invitationAggregate)

	for _, invitation := range invitations {
		for _, score := range invitation.Scores {
			if score.Score == nil {
				continue
			}

			aggregate := aggregates[score.ItemID]
			aggregate.Average += *score.Score
			aggregate.Count++
			aggregates[score.ItemID] = aggregate
		}
	}

	for itemID, aggregate := range aggregates {
		if aggregate.Count > 0 {
			aggregate.Average = aggregate.Average / float64(aggregate.Count)
			aggregates[itemID] = aggregate
		} else {
			delete(aggregates, itemID)
		}
	}

	return aggregates
}

func calculateHRScoreByScenario(score models.KPIScore, aggregate invitationAggregate, scenario string, rule models.PerformanceRule) (float64, bool) {
	selfValue, hasSelf := valueFromPointer(score.SelfScore)
	managerValue, hasManager := valueFromPointer(score.ManagerScore)
	inviteValue := aggregate.Average
	hasInvite := aggregate.Count > 0

	var components []scoreComponent

	switch scenario {
	case performanceScenarioEmployeeInvitation:
		components = []scoreComponent{
			{weight: rule.WithInvitation.Employee.SelfWeight, value: selfValue, present: hasSelf},
			{weight: rule.WithInvitation.Employee.InviteSuperiorWeight, value: inviteValue, present: hasInvite},
			{weight: rule.WithInvitation.Employee.SuperiorWeight, value: managerValue, present: hasManager},
		}
	default:
		components = []scoreComponent{
			{weight: rule.NoInvitation.SelfWeight, value: selfValue, present: hasSelf},
			{weight: rule.NoInvitation.SuperiorWeight, value: managerValue, present: hasManager},
		}
	}

	result, ok := weightedAverage(components)
	if !ok {
		return 0, false
	}

	result = math.Round(result*100) / 100
	return result, true
}

func valueFromPointer(val *float64) (float64, bool) {
	if val == nil {
		return 0, false
	}
	return *val, true
}

func weightedAverage(components []scoreComponent) (float64, bool) {
	totalWeight := 0.0
	for _, component := range components {
		if component.present && component.weight > 0 {
			totalWeight += component.weight
		}
	}

	if totalWeight == 0 {
		return 0, false
	}

	totalValue := 0.0
	for _, component := range components {
		if component.present && component.weight > 0 {
			totalValue += (component.weight / totalWeight) * component.value
		}
	}

	return totalValue, true
}
