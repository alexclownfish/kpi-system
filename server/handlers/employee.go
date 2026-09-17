package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"dootask-kpi-server/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// 获取所有员工
func GetEmployees(c *gin.Context) {
	var employees []models.Employee
	userID, _ := c.Get("user_id")
	currentUserID, _ := userID.(uint)

	// 解析分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	search := c.Query("search")
	departmentID := c.Query("department_id")
	role := c.Query("role")
	isActiveStr := c.Query("is_active")

	// 验证分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// 构建查询
	query := ApplyEmployeeScope(models.DB.Preload("Department").Preload("Manager"), currentUserID)

	// 添加搜索条件
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("name LIKE ? OR email LIKE ? OR position LIKE ?",
			searchPattern, searchPattern, searchPattern)
	}

	// 添加部门筛选
	if departmentID != "" {
		query = query.Where("department_id = ?", departmentID)
	}

	// 添加角色筛选（支持多个角色，用逗号分隔）
	if role != "" {
		if role == "manager,hr" {
			query = query.Where("role IN (?)", []string{"manager", "hr"})
		} else {
			query = query.Where("role = ?", role)
		}
	}

	// 添加状态筛选（is_active）
	if isActiveStr != "" {
		isActive := isActiveStr == "true"
		query = query.Where("is_active = ?", isActive)
	}

	// 获取总数
	var total int64
	countQuery := ApplyEmployeeScope(models.DB.Model(&models.Employee{}), currentUserID)
	if search != "" {
		searchPattern := "%" + search + "%"
		countQuery = countQuery.Where("name LIKE ? OR email LIKE ? OR position LIKE ?",
			searchPattern, searchPattern, searchPattern)
	}
	if departmentID != "" {
		countQuery = countQuery.Where("department_id = ?", departmentID)
	}
	if role != "" {
		if role == "manager,hr" {
			countQuery = countQuery.Where("role IN (?)", []string{"manager", "hr"})
		} else {
			countQuery = countQuery.Where("role = ?", role)
		}
	}
	if isActiveStr != "" {
		isActive := isActiveStr == "true"
		countQuery = countQuery.Where("is_active = ?", isActive)
	}

	if err := countQuery.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取员工总数失败",
			"message": err.Error(),
		})
		return
	}

	// 分页查询
	offset := (page - 1) * pageSize
	result := query.Offset(offset).Limit(pageSize).Find(&employees)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取员工列表失败",
			"message": result.Error.Error(),
		})
		return
	}

	// 计算分页信息
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))

	c.JSON(http.StatusOK, gin.H{
		"data":       employees,
		"total":      total,
		"page":       page,
		"pageSize":   pageSize,
		"totalPages": totalPages,
		"hasNext":    page < totalPages,
		"hasPrev":    page > 1,
	})
}

// CreateEmployeeRequest 单独承接创建参数，避免 Employee.Password 的 json:"-"
// 在解码请求时丢弃初始密码。
type CreateEmployeeRequest struct {
	Name         string `json:"name"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	Position     string `json:"position"`
	DepartmentID uint   `json:"department_id"`
	ManagerID    *uint  `json:"manager_id"`
	Role         string `json:"role"`
	IsActive     bool   `json:"is_active"`
}

// 创建员工
func CreateEmployee(c *gin.Context) {
	var request CreateEmployeeRequest

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}
	employee := models.Employee{
		Name:         strings.TrimSpace(request.Name),
		Email:        strings.TrimSpace(request.Email),
		Password:     request.Password,
		Position:     strings.TrimSpace(request.Position),
		DepartmentID: request.DepartmentID,
		ManagerID:    request.ManagerID,
		Role:         request.Role,
		IsActive:     request.IsActive,
	}

	if employee.Role == "" {
		employee.Role = "employee"
	}
	roleCode := LegacyRoleCode(employee.Role)
	var role models.Role
	if err := models.DB.Where("code = ?", roleCode).First(&role).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "角色不存在"})
		return
	}
	if roleCode == "super_admin" && !HasPermission(c, "role:edit") {
		c.JSON(http.StatusForbidden, gin.H{"error": "只有超级管理员可以创建超级管理员"})
		return
	}
	employee.Role = LegacyRoleValue(roleCode)
	if strings.TrimSpace(employee.Password) == "" || len(employee.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "初始密码长度不能少于6位"})
		return
	}
	if !strings.HasPrefix(employee.Password, "$2a$") && !strings.HasPrefix(employee.Password, "$2b$") && !strings.HasPrefix(employee.Password, "$2y$") {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(employee.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
			return
		}
		employee.Password = string(hashedPassword)
	}
	if employee.Role == "employee" && (employee.ManagerID == nil || *employee.ManagerID == 0) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "普通员工必须选择直属上级",
		})
		return
	}
	if employee.Role == "employee" {
		if validationError := validateManagerAssignment(employee.ManagerID, employee.DepartmentID); validationError != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": validationError})
			return
		}
	}
	var duplicateCount int64
	if err := models.DB.Model(&models.Employee{}).
		Where("LOWER(email) = LOWER(?)", employee.Email).
		Count(&duplicateCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "检查员工邮箱失败"})
		return
	}
	if duplicateCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "该邮箱已被使用，请更换邮箱"})
		return
	}

	tx := models.DB.Begin()
	result := tx.Create(&employee)
	if result.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "创建员工失败",
			"message": result.Error.Error(),
		})
		return
	}
	if err := ReplaceUserRole(tx, employee.ID, roleCode); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "员工角色初始化失败"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "员工创建失败"})
		return
	}
	RecordAudit(c, "create_employee", "employee", strconv.FormatUint(uint64(employee.ID), 10), "SUCCESS")

	// 获取完整的员工信息
	models.DB.Preload("Department").Preload("Manager").First(&employee, employee.ID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "员工创建成功",
		"data":    employee,
	})
}

// 获取单个员工
func GetEmployee(c *gin.Context) {
	id := c.Param("id")
	employeeId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的员工ID",
		})
		return
	}

	var employee models.Employee
	userID, _ := c.Get("user_id")
	currentUserID, _ := userID.(uint)
	result := ApplyEmployeeScope(models.DB.Preload("Department").Preload("Manager").Preload("Subordinates"), currentUserID).First(&employee, employeeId)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "员工不存在",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": employee,
	})
}

// 更新员工请求
// Password 只用于编辑员工时重置密码，不会在响应中返回。
type UpdateEmployeeRequest struct {
	Name         string `json:"name"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	Position     string `json:"position"`
	DepartmentID uint   `json:"department_id"`
	ManagerID    *uint  `json:"manager_id"`
	Role         string `json:"role"`
	IsActive     bool   `json:"is_active"`
}

// 更新员工
func UpdateEmployee(c *gin.Context) {
	id := c.Param("id")
	employeeId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的员工ID",
		})
		return
	}

	var employee models.Employee
	result := models.DB.First(&employee, employeeId)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "员工不存在",
		})
		return
	}
	if LegacyRoleCode(employee.Role) == "super_admin" && !HasPermission(c, "role:edit") {
		c.JSON(http.StatusForbidden, gin.H{"error": "只有超级管理员可以修改超级管理员"})
		return
	}

	var updateData UpdateEmployeeRequest
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}
	if updateData.Role != "" && LegacyRoleCode(updateData.Role) != LegacyRoleCode(employee.Role) {
		if !HasPermission(c, "employee:assign_role") {
			c.JSON(http.StatusForbidden, gin.H{"error": "无权修改员工角色"})
			return
		}
		if (LegacyRoleCode(employee.Role) == "super_admin" || LegacyRoleCode(updateData.Role) == "super_admin") && !HasPermission(c, "role:edit") {
			c.JSON(http.StatusForbidden, gin.H{"error": "只有超级管理员可以变更超级管理员角色"})
			return
		}
	}

	targetRole := updateData.Role
	if targetRole == "" {
		targetRole = employee.Role
	}
	targetDepartmentID := updateData.DepartmentID
	if targetDepartmentID == 0 {
		targetDepartmentID = employee.DepartmentID
	}
	targetManagerID := updateData.ManagerID
	if targetRole == "employee" && targetManagerID == nil {
		targetManagerID = employee.ManagerID
	}

	if targetRole == "employee" && (targetManagerID == nil || *targetManagerID == 0) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "普通员工必须选择直属上级",
		})
		return
	}
	if LegacyRoleValue(targetRole) == "employee" {
		if validationError := validateManagerAssignment(targetManagerID, targetDepartmentID); validationError != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": validationError})
			return
		}
	}

	// 密码重置属于 HR 管理操作，避免主管通过接口修改员工登录凭据。
	if updateData.Password != "" {
		if !HasPermission(c, "employee:edit") {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "只有HR可以修改员工密码",
			})
			return
		}
		if strings.TrimSpace(updateData.Password) == "" || len(updateData.Password) < 6 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "密码长度不能少于6位",
			})
			return
		}
	}

	roleValue := updateData.Role
	if roleValue == "" {
		roleValue = employee.Role
	} else {
		roleValue = LegacyRoleValue(roleValue)
	}
	managerValue := updateData.ManagerID
	if updateData.Role == "" && updateData.ManagerID == nil {
		managerValue = employee.ManagerID
	}

	// 使用 map 来确保 nil 值能够被正确更新
	updateMap := map[string]interface{}{
		"name":          updateData.Name,
		"email":         updateData.Email,
		"position":      updateData.Position,
		"department_id": targetDepartmentID,
		"manager_id":    managerValue, // 支持 nil 值以清空直属上级
		"role":          roleValue,
		"is_active":     updateData.IsActive,
	}

	if updateData.Password != "" {
		hashedPassword, hashErr := bcrypt.GenerateFromPassword([]byte(updateData.Password), bcrypt.DefaultCost)
		if hashErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "密码加密失败",
				"message": hashErr.Error(),
			})
			return
		}
		updateMap["password"] = string(hashedPassword)
	}

	tx := models.DB.Begin()
	result = tx.Model(&employee).Updates(updateMap)
	if result.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "更新员工失败",
			"message": result.Error.Error(),
		})
		return
	}
	if updateData.Role != "" && LegacyRoleCode(updateData.Role) != LegacyRoleCode(employee.Role) {
		if err := ReplaceUserRole(tx, employee.ID, updateData.Role); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "员工角色更新失败"})
			return
		}
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "员工更新失败"})
		return
	}
	RecordAudit(c, "update_employee", "employee", strconv.FormatUint(employeeId, 10), "SUCCESS")
	if updateData.Password != "" {
		RecordAudit(c, "reset_password", "employee", strconv.FormatUint(employeeId, 10), "SUCCESS")
	}

	// 获取完整的员工信息
	models.DB.Preload("Department").Preload("Manager").First(&employee, employee.ID)

	c.JSON(http.StatusOK, gin.H{
		"message": "员工更新成功",
		"data":    employee,
	})
}

func validateManagerAssignment(managerID *uint, departmentID uint) string {
	if managerID == nil || *managerID == 0 {
		return "普通员工必须选择直属上级"
	}
	var manager models.Employee
	if err := models.DB.Select("id", "department_id", "is_active").First(&manager, *managerID).Error; err != nil {
		return "直属上级不存在"
	}
	if !manager.IsActive {
		return "直属上级已停用，请选择在职上级"
	}
	if manager.DepartmentID != departmentID {
		return "直属上级不属于所选部门"
	}
	return ""
}

// 删除员工
func DeleteEmployee(c *gin.Context) {
	id := c.Param("id")
	employeeId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的员工ID",
		})
		return
	}
	var employee models.Employee
	if err := models.DB.Select("id", "role").First(&employee, employeeId).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "员工不存在"})
		return
	}
	if LegacyRoleCode(employee.Role) == "super_admin" && !HasPermission(c, "role:edit") {
		c.JSON(http.StatusForbidden, gin.H{"error": "只有超级管理员可以删除超级管理员"})
		return
	}

	// 检查是否有下属员工
	var subordinateCount int64
	models.DB.Model(&models.Employee{}).Where("manager_id = ?", employeeId).Count(&subordinateCount)
	if subordinateCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "该员工还有下属，无法删除",
		})
		return
	}

	// 检查是否有相关的KPI评估记录
	var evaluationCount int64
	models.DB.Model(&models.KPIEvaluation{}).Where("employee_id = ?", employeeId).Count(&evaluationCount)
	if evaluationCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "该员工有KPI评估记录，无法删除",
		})
		return
	}

	result := models.DB.Delete(&models.Employee{}, employeeId)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "删除员工失败",
			"message": result.Error.Error(),
		})
		return
	}
	RecordAudit(c, "delete_employee", "employee", strconv.FormatUint(employeeId, 10), "SUCCESS")

	c.JSON(http.StatusOK, gin.H{
		"message": "员工删除成功",
	})
}

// 获取员工的下属列表
func GetEmployeeSubordinates(c *gin.Context) {
	id := c.Param("id")
	employeeId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的员工ID",
		})
		return
	}
	currentUser, _ := c.Get("user_id")
	currentUserID, _ := currentUser.(uint)
	scope := DataScopeForUser(currentUserID)
	if scope == "SELF" || scope == "ASSIGNED" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权查看下属"})
		return
	}
	if scope == "DEPARTMENT" || scope == "DEPARTMENT_TREE" {
		var target, owner models.Employee
		if models.DB.Select("department_id").First(&target, employeeId).Error != nil || models.DB.Select("department_id").First(&owner, currentUserID).Error != nil || target.DepartmentID != owner.DepartmentID {
			c.JSON(http.StatusForbidden, gin.H{"error": "超出数据范围"})
			return
		}
	}

	var subordinates []models.Employee
	result := models.DB.Preload("Department").Where("manager_id = ?", employeeId).Find(&subordinates)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取下属列表失败",
			"message": result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  subordinates,
		"total": len(subordinates),
	})
}
