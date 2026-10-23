package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type evaluationResponsibilityRequest struct {
	Code          string  `json:"code" binding:"required"`
	Name          string  `json:"name" binding:"required"`
	ScopeType     string  `json:"scope_type" binding:"required"`
	DepartmentID  *uint   `json:"department_id"`
	PositionType  string  `json:"position_type"`
	ProjectID     *uint   `json:"project_id"`
	UserID        uint    `json:"user_id" binding:"required"`
	Active        *bool   `json:"active"`
	EffectiveFrom *string `json:"effective_from"`
	EffectiveTo   *string `json:"effective_to"`
}

type evaluationResponsibilityResponse struct {
	models.EvaluationResponsibility
	UserName       string `json:"user_name,omitempty"`
	UserEmail      string `json:"user_email,omitempty"`
	DepartmentName string `json:"department_name,omitempty"`
}

func normalizeResponsibilityCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func parseResponsibilityTime(value *string) (*time.Time, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*value))
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func validateResponsibilityRequest(db *gorm.DB, request evaluationResponsibilityRequest, existingID uint) (models.Employee, error) {
	request.Code = normalizeResponsibilityCode(request.Code)
	if request.Code == "" || request.Name == "" {
		return models.Employee{}, errors.New("评分职责编码和名称不能为空")
	}
	if request.Code != "PM" && request.Code != "DEPARTMENT_MANAGER" && request.Code != "PRODUCT_MANAGER" && request.Code != "CTO" {
		return models.Employee{}, errors.New("不支持的评分职责编码")
	}
	request.ScopeType = strings.ToLower(strings.TrimSpace(request.ScopeType))
	if request.ScopeType != "company" && request.ScopeType != "department" && request.ScopeType != "position" {
		return models.Employee{}, errors.New("作用域必须是 company、department 或 position")
	}
	if request.ScopeType == "department" && (request.DepartmentID == nil || *request.DepartmentID == 0) {
		return models.Employee{}, errors.New("部门作用域必须选择部门")
	}
	if request.ScopeType == "position" && strings.TrimSpace(request.PositionType) == "" {
		return models.Employee{}, errors.New("岗位作用域必须填写岗位")
	}
	if request.ScopeType == "company" {
		request.DepartmentID = nil
		request.PositionType = ""
		request.ProjectID = nil
	}
	var employee models.Employee
	if err := db.Where("id = ?", request.UserID).First(&employee).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Employee{}, errors.New("评分人不存在")
		}
		return models.Employee{}, err
	}
	if !employee.IsActive {
		return models.Employee{}, errors.New("评分人已停用，不能绑定新职责")
	}
	var duplicate int64
	query := db.Model(&models.EvaluationResponsibility{}).
		Where("code = ? AND scope_type = ? AND user_id = ?", request.Code, request.ScopeType, request.UserID)
	if existingID > 0 {
		query = query.Where("id <> ?", existingID)
	}
	if request.DepartmentID == nil {
		query = query.Where("department_id IS NULL")
	} else {
		query = query.Where("department_id = ?", *request.DepartmentID)
	}
	if strings.TrimSpace(request.PositionType) == "" {
		query = query.Where("position_type = ''")
	} else {
		query = query.Where("position_type = ?", strings.TrimSpace(request.PositionType))
	}
	if request.ProjectID == nil {
		query = query.Where("project_id IS NULL")
	} else {
		query = query.Where("project_id = ?", *request.ProjectID)
	}
	if err := query.Count(&duplicate).Error; err != nil {
		return models.Employee{}, err
	}
	if duplicate > 0 {
		return models.Employee{}, gorm.ErrDuplicatedKey
	}
	return employee, nil
}

func responsibilityResponse(item models.EvaluationResponsibility) evaluationResponsibilityResponse {
	response := evaluationResponsibilityResponse{EvaluationResponsibility: item}
	response.UserName = item.User.Name
	response.UserEmail = item.User.Email
	if item.Department != nil {
		response.DepartmentName = item.Department.Name
	}
	return response
}

func applyResponsibilityAdministrationScope(query *gorm.DB, actorID uint) *gorm.DB {
	switch DataScopeForUser(actorID) {
	case "ALL":
		return query
	case "DEPARTMENT":
		return query.Where("department_id = (SELECT department_id FROM employees WHERE id = ?)", actorID)
	case "DEPARTMENT_TREE":
		return query.Where("department_id IN ("+models.UserDepartmentSubtreeSQL+")", actorID)
	default:
		return query.Where("1 = 0")
	}
}

func canAdministerResponsibility(actorID uint, request evaluationResponsibilityRequest) bool {
	if DataScopeForUser(actorID) == "ALL" {
		return true
	}
	if !CanAccessEmployee(actorID, request.UserID) || request.ScopeType == "company" {
		return false
	}
	if request.DepartmentID == nil {
		return false
	}
	return departmentWithinActorScope(actorID, *request.DepartmentID)
}

func departmentWithinActorScope(actorID, departmentID uint) bool {
	query := models.DB.Model(&models.Department{}).Where("id = ?", departmentID)
	switch DataScopeForUser(actorID) {
	case "ALL":
	case "DEPARTMENT":
		query = query.Where("id = (SELECT department_id FROM employees WHERE id = ?)", actorID)
	case "DEPARTMENT_TREE":
		query = query.Where("id IN ("+models.UserDepartmentSubtreeSQL+")", actorID)
	default:
		return false
	}
	var count int64
	return query.Count(&count).Error == nil && count == 1
}

func GetEvaluationResponsibilities(c *gin.Context) {
	query := applyResponsibilityAdministrationScope(models.DB.Preload("User").Preload("Department").Model(&models.EvaluationResponsibility{}), actorID(c))
	if code := normalizeResponsibilityCode(c.Query("code")); code != "" {
		query = query.Where("UPPER(code) = ?", code)
	}
	if scope := strings.TrimSpace(c.Query("scope_type")); scope != "" {
		query = query.Where("scope_type = ?", strings.ToLower(scope))
	}
	if active := c.Query("active"); active == "true" || active == "false" {
		query = query.Where("active = ?", active == "true")
	}
	var items []models.EvaluationResponsibility
	if err := query.Order("active DESC, code ASC, id ASC").Find(&items).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "RESPONSIBILITY_LIST_FAILED", "评分职责加载失败", nil)
		return
	}
	data := make([]evaluationResponsibilityResponse, 0, len(items))
	for _, item := range items {
		data = append(data, responsibilityResponse(item))
	}
	respondData(c, http.StatusOK, gin.H{"items": data, "total": len(data)}, "评分职责加载成功")
}

func CreateEvaluationResponsibility(c *gin.Context) {
	var request evaluationResponsibilityRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_RESPONSIBILITY", "评分职责参数不完整", err.Error())
		return
	}
	request.Code = normalizeResponsibilityCode(request.Code)
	request.ScopeType = strings.ToLower(strings.TrimSpace(request.ScopeType))
	if !canAdministerResponsibility(actorID(c), request) {
		respondError(c, http.StatusForbidden, "RESPONSIBILITY_SCOPE_FORBIDDEN", "超出评分职责管理范围", nil)
		return
	}
	user, err := validateResponsibilityRequest(models.DB, request, 0)
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		respondError(c, http.StatusConflict, "RESPONSIBILITY_DUPLICATE", "相同职责、作用域和评分人已存在", nil)
		return
	}
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_RESPONSIBILITY_SCOPE", err.Error(), nil)
		return
	}
	from, err := parseResponsibilityTime(request.EffectiveFrom)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_RESPONSIBILITY_DATE", "生效时间格式必须是 RFC3339", nil)
		return
	}
	to, err := parseResponsibilityTime(request.EffectiveTo)
	if err != nil || (from != nil && to != nil && to.Before(*from)) {
		respondError(c, http.StatusBadRequest, "INVALID_RESPONSIBILITY_DATE", "失效时间必须晚于生效时间且使用 RFC3339", nil)
		return
	}
	active := true
	if request.Active != nil {
		active = *request.Active
	}
	item := models.EvaluationResponsibility{Code: request.Code, Name: strings.TrimSpace(request.Name), ScopeType: request.ScopeType, DepartmentID: request.DepartmentID, PositionType: strings.TrimSpace(request.PositionType), ProjectID: request.ProjectID, UserID: user.ID, Active: active, EffectiveFrom: from, EffectiveTo: to, CreatedBy: actorID(c)}
	if err := models.DB.Create(&item).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "RESPONSIBILITY_CREATE_FAILED", "评分职责创建失败", nil)
		return
	}
	models.DB.Preload("User").Preload("Department").First(&item, item.ID)
	RecordAuditDetails(c, "create_evaluation_responsibility", "evaluation_responsibility", strconv.FormatUint(uint64(item.ID), 10), "SUCCESS", fmt.Sprintf("code=%s user_id=%d", item.Code, item.UserID))
	respondData(c, http.StatusCreated, responsibilityResponse(item), "评分职责创建成功")
}

func UpdateEvaluationResponsibility(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_RESPONSIBILITY_ID", "评分职责ID无效", nil)
		return
	}
	var request evaluationResponsibilityRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_RESPONSIBILITY", "评分职责参数不完整", err.Error())
		return
	}
	request.Code = normalizeResponsibilityCode(request.Code)
	request.ScopeType = strings.ToLower(strings.TrimSpace(request.ScopeType))
	if !canAdministerResponsibility(actorID(c), request) {
		respondError(c, http.StatusForbidden, "RESPONSIBILITY_SCOPE_FORBIDDEN", "超出评分职责管理范围", nil)
		return
	}
	if _, err := validateResponsibilityRequest(models.DB, request, uint(id)); errors.Is(err, gorm.ErrDuplicatedKey) {
		respondError(c, http.StatusConflict, "RESPONSIBILITY_DUPLICATE", "相同职责、作用域和评分人已存在", nil)
		return
	} else if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_RESPONSIBILITY_SCOPE", err.Error(), nil)
		return
	}
	from, err := parseResponsibilityTime(request.EffectiveFrom)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_RESPONSIBILITY_DATE", "生效时间格式必须是 RFC3339", nil)
		return
	}
	to, err := parseResponsibilityTime(request.EffectiveTo)
	if err != nil || (from != nil && to != nil && to.Before(*from)) {
		respondError(c, http.StatusBadRequest, "INVALID_RESPONSIBILITY_DATE", "失效时间必须晚于生效时间且使用 RFC3339", nil)
		return
	}
	updates := map[string]any{"code": request.Code, "name": strings.TrimSpace(request.Name), "scope_type": request.ScopeType, "department_id": request.DepartmentID, "position_type": strings.TrimSpace(request.PositionType), "project_id": request.ProjectID, "user_id": request.UserID, "effective_from": from, "effective_to": to}
	if request.Active != nil {
		updates["active"] = *request.Active
	}
	if err := models.DB.Model(&models.EvaluationResponsibility{}).Where("id = ?", uint(id)).Updates(updates).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, "RESPONSIBILITY_NOT_FOUND", "评分职责不存在", nil)
			return
		}
		respondError(c, http.StatusInternalServerError, "RESPONSIBILITY_UPDATE_FAILED", "评分职责更新失败", nil)
		return
	}
	var item models.EvaluationResponsibility
	if err := models.DB.Preload("User").Preload("Department").First(&item, uint(id)).Error; err != nil {
		respondError(c, http.StatusNotFound, "RESPONSIBILITY_NOT_FOUND", "评分职责不存在", nil)
		return
	}
	RecordAuditDetails(c, "update_evaluation_responsibility", "evaluation_responsibility", strconv.FormatUint(id, 10), "SUCCESS", fmt.Sprintf("active=%t", item.Active))
	respondData(c, http.StatusOK, responsibilityResponse(item), "评分职责更新成功")
}

func DeactivateEvaluationResponsibility(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_RESPONSIBILITY_ID", "评分职责ID无效", nil)
		return
	}
	result := applyResponsibilityAdministrationScope(models.DB.Model(&models.EvaluationResponsibility{}), actorID(c)).Where("id = ?", uint(id)).Update("active", false)
	if result.Error != nil {
		respondError(c, http.StatusInternalServerError, "RESPONSIBILITY_DEACTIVATE_FAILED", "评分职责停用失败", nil)
		return
	}
	if result.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, "RESPONSIBILITY_NOT_FOUND", "评分职责不存在", nil)
		return
	}
	RecordAudit(c, "deactivate_evaluation_responsibility", "evaluation_responsibility", strconv.FormatUint(id, 10), "SUCCESS")
	respondData(c, http.StatusOK, gin.H{"id": uint(id), "active": false}, "评分职责已停用；已创建考核的评分任务不会被改写")
}
