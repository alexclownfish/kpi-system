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

// 获取所有部门
func GetDepartments(c *gin.Context) {
	var departments []models.Department

	// 解析分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	search := c.Query("search")

	// 验证分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// 构建查询，只预加载在职员工
	query := models.DB.Preload("Employees", func(db *gorm.DB) *gorm.DB {
		return ApplyEmployeeScope(db, actorID(c)).Where("is_active = ?", true)
	})

	// 添加搜索条件
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("LOWER(name) LIKE LOWER(?) OR LOWER(description) LIKE LOWER(?)",
			searchPattern, searchPattern)
	}

	// 获取总数
	var total int64
	countQuery := models.DB.Model(&models.Department{})
	if search != "" {
		searchPattern := "%" + search + "%"
		countQuery = countQuery.Where("LOWER(name) LIKE LOWER(?) OR LOWER(description) LIKE LOWER(?)",
			searchPattern, searchPattern)
	}

	if err := countQuery.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取部门总数失败",
			"message": err.Error(),
		})
		return
	}

	// 分页查询
	offset := (page - 1) * pageSize
	result := query.Offset(offset).Limit(pageSize).Find(&departments)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "获取部门列表失败",
			"message": result.Error.Error(),
		})
		return
	}

	departmentPointers := make([]*models.Department, len(departments))
	for i := range departments {
		departmentPointers[i] = &departments[i]
	}
	hydrateDepartmentLeaders(c, departmentPointers)
	// 计算分页信息
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))

	c.JSON(http.StatusOK, gin.H{
		"data":       departments,
		"total":      total,
		"page":       page,
		"pageSize":   pageSize,
		"totalPages": totalPages,
		"hasNext":    page < totalPages,
		"hasPrev":    page > 1,
	})
}

// 创建部门
func CreateDepartment(c *gin.Context) {
	var request struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		ParentID    *uint  `json:"parent_id"`
		LeaderID    *uint  `json:"leader_id"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}
	if strings.TrimSpace(request.Name) == "" {
		c.JSON(400, gin.H{"error": "部门名称不能为空"})
		return
	}
	if request.LeaderID != nil {
		if err := validateDepartmentLeader(models.DB, actorID(c), *request.LeaderID); err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "数据范围") {
				status = http.StatusForbidden
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
	}
	if err := validateDepartmentParent(models.DB, 0, request.ParentID); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	department := models.Department{Name: strings.TrimSpace(request.Name), Description: request.Description, ParentID: request.ParentID, LeaderID: request.LeaderID}

	result := models.DB.Create(&department)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "创建部门失败",
			"message": result.Error.Error(),
		})
		return
	}

	hydrateDepartmentLeaders(c, []*models.Department{&department})
	c.JSON(http.StatusCreated, gin.H{
		"message": "部门创建成功",
		"data":    department,
	})
	RecordAuditDetails(c, "create_department", "department", strconv.FormatUint(uint64(department.ID), 10), "SUCCESS", "parent_id="+stringParentID(department.ParentID)+";leader_id="+stringParentID(department.LeaderID))
}

// 获取单个部门
func GetDepartment(c *gin.Context) {
	id := c.Param("id")
	departmentId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的部门ID",
		})
		return
	}

	var department models.Department
	result := models.DB.Preload("Employees", func(db *gorm.DB) *gorm.DB {
		return ApplyEmployeeScope(db, actorID(c)).Where("is_active = ?", true)
	}).First(&department, departmentId)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "部门不存在",
		})
		return
	}

	hydrateDepartmentLeaders(c, []*models.Department{&department})

	c.JSON(http.StatusOK, gin.H{
		"data": department,
	})
}

// 更新部门
func UpdateDepartment(c *gin.Context) {
	id := c.Param("id")
	departmentId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的部门ID",
		})
		return
	}

	var department models.Department
	result := models.DB.First(&department, departmentId)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "部门不存在",
		})
		return
	}

	var updateData map[string]json.RawMessage
	if !departmentWithinActorScope(actorID(c), department.ID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "部门超出数据范围"})
		return
	}
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"message": err.Error(),
		})
		return
	}
	updates := map[string]any{}
	for _, field := range []string{"name", "description"} {
		if raw, ok := updateData[field]; ok {
			var value string
			if err := json.Unmarshal(raw, &value); err != nil || field == "name" && strings.TrimSpace(value) == "" {
				c.JSON(400, gin.H{"error": "部门名称或描述无效"})
				return
			}
			updates[field] = value
		}
	}
	if raw, ok := updateData["parent_id"]; ok {
		var parentID *uint
		if err := json.Unmarshal(raw, &parentID); err != nil {
			c.JSON(400, gin.H{"error": "上级部门无效"})
			return
		}
		if err := validateDepartmentParent(models.DB, department.ID, parentID); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		updates["parent_id"] = parentID
	}
	if raw, ok := updateData["leader_id"]; ok {
		var leaderID *uint
		if err := json.Unmarshal(raw, &leaderID); err != nil || leaderID != nil && *leaderID == 0 {
			c.JSON(400, gin.H{"error": "部门负责人无效"})
			return
		}
		if leaderID != nil {
			if err := validateDepartmentLeader(models.DB, actorID(c), *leaderID); err != nil {
				status := http.StatusBadRequest
				if strings.Contains(err.Error(), "数据范围") {
					status = http.StatusForbidden
				}
				c.JSON(status, gin.H{"error": err.Error()})
				return
			}
		}
		updates["leader_id"] = leaderID
	}
	oldLeaderID := department.LeaderID
	previousLeaderDirectReports := int64(0)
	if raw, changed := updateData["leader_id"]; changed && oldLeaderID != nil {
		var requested *uint
		_ = json.Unmarshal(raw, &requested)
		if requested == nil || *requested != *oldLeaderID {
			if err := models.DB.Model(&models.Employee{}).Where("department_id = ? AND manager_id = ? AND is_active = ?", department.ID, *oldLeaderID, true).Count(&previousLeaderDirectReports).Error; err != nil {
				c.JSON(500, gin.H{"error": "统计负责人交接信息失败"})
				return
			}
		}
	}
	result = models.DB.Model(&department).Updates(updates)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新部门失败", "message": result.Error.Error()})
		return
	}
	department = models.Department{}
	if err := models.DB.First(&department, departmentId).Error; err != nil {
		c.JSON(500, gin.H{"error": "读取更新后的部门失败"})
		return
	}
	hydrateDepartmentLeaders(c, []*models.Department{&department})
	RecordAuditDetails(c, "update_department", "department", strconv.FormatUint(departmentId, 10), "SUCCESS", "parent_id="+stringParentID(department.ParentID)+";old_leader_id="+stringParentID(oldLeaderID)+";leader_id="+stringParentID(department.LeaderID)+";previous_leader_direct_reports="+strconv.FormatInt(previousLeaderDirectReports, 10))

	c.JSON(http.StatusOK, gin.H{
		"message":                        "部门更新成功",
		"data":                           department,
		"previous_leader_direct_reports": previousLeaderDirectReports,
	})
}

func validateDepartmentLeader(db *gorm.DB, actorID, leaderID uint) error {
	var leader models.Employee
	if err := db.Where("id = ?", leaderID).First(&leader).Error; err != nil || !leader.IsActive {
		return errors.New("部门负责人必须是在职员工")
	}
	if !CanAccessEmployee(actorID, leaderID) {
		return errors.New("部门负责人超出数据范围")
	}
	return nil
}

// GetDepartmentLeaderCandidates exposes only active employees in the actor's
// existing data scope; it never grants a new permission or scope.
func GetDepartmentLeaderCandidates(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "100"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 100
	}
	query := ApplyEmployeeScope(models.DB.Model(&models.Employee{}), actorID(c)).Where("is_active = ?", true)
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		pattern := "%" + search + "%"
		query = query.Where("LOWER(name) LIKE LOWER(?) OR LOWER(email) LIKE LOWER(?) OR LOWER(position) LIKE LOWER(?)", pattern, pattern, pattern)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"error": "负责人候选统计失败"})
		return
	}
	employees := []models.Employee{}
	if err := query.Select("id", "name", "email", "position", "department_id", "is_active").Order("name ASC, id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&employees).Error; err != nil {
		c.JSON(500, gin.H{"error": "负责人候选加载失败"})
		return
	}
	c.JSON(200, gin.H{"data": employees, "total": total, "page": page, "pageSize": pageSize, "totalPages": (total + int64(pageSize) - 1) / int64(pageSize)})
}

func hydrateDepartmentLeaders(c *gin.Context, departments []*models.Department) {
	actor := actorID(c)
	if actor == 0 {
		return
	} // Public registration catalog has no staff details.
	var employee models.Employee
	_ = models.DB.Select("department_id").First(&employee, actor).Error
	ids := []uint{}
	for _, department := range departments {
		if department.LeaderID != nil && (department.ID == employee.DepartmentID || departmentWithinActorScope(actor, department.ID)) {
			ids = append(ids, *department.LeaderID)
		}
	}
	if len(ids) == 0 {
		return
	}
	var leaders []models.DepartmentLeaderSummary
	if models.DB.Table("employees").Select("id", "name", "position", "is_active").Where("id IN ?", ids).Scan(&leaders).Error != nil {
		return
	}
	byID := map[uint]*models.DepartmentLeaderSummary{}
	for i := range leaders {
		byID[leaders[i].ID] = &leaders[i]
	}
	for _, department := range departments {
		if department.LeaderID != nil && (department.ID == employee.DepartmentID || departmentWithinActorScope(actor, department.ID)) {
			department.Leader = byID[*department.LeaderID]
		}
	}
}

func stringParentID(id *uint) string {
	if id == nil {
		return "null"
	}
	return strconv.FormatUint(uint64(*id), 10)
}

// 删除部门
func DeleteDepartment(c *gin.Context) {
	id := c.Param("id")
	departmentId, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的部门ID",
		})
		return
	}
	var childCount int64
	if err := models.DB.Model(&models.Department{}).Where("parent_id = ?", departmentId).Count(&childCount).Error; err != nil {
		c.JSON(500, gin.H{"error": "检查子部门失败"})
		return
	}
	if childCount > 0 {
		c.JSON(400, gin.H{"error": "该部门仍有子部门，请先调整子部门的上级关系"})
		return
	}

	// 检查是否有在职员工在该部门
	var employeeCount int64
	models.DB.Model(&models.Employee{}).Where("department_id = ? AND is_active = ?", departmentId, true).Count(&employeeCount)
	if employeeCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "该部门下还有在职员工，无法删除",
		})
		return
	}
	var templateCount int64
	if err := models.DB.Model(&models.KPITemplate{}).Where("department_id = ?", departmentId).Count(&templateCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "检查部门模板失败"})
		return
	}
	if templateCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该部门仍关联考核模板，请先调整或删除模板后再删除部门"})
		return
	}

	err = models.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("department_id = ?", departmentId).Delete(&models.JobPosition{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Department{}, departmentId).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "删除部门失败",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "部门删除成功",
	})
}
