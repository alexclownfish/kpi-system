package handlers

import (
	"encoding/json"
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
		c.JSON(400, gin.H{"error": "请先创建部门并添加员工，再设置部门负责人"})
		return
	}
	if err := validateDepartmentParent(models.DB, 0, request.ParentID); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	department := models.Department{Name: strings.TrimSpace(request.Name), Description: request.Description, ParentID: request.ParentID}

	result := models.DB.Create(&department)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "创建部门失败",
			"message": result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "部门创建成功",
		"data":    department,
	})
	RecordAuditDetails(c, "create_department", "department", strconv.FormatUint(uint64(department.ID), 10), "SUCCESS", "parent_id="+stringParentID(department.ParentID))
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
			var leader models.Employee
			if err := models.DB.First(&leader, *leaderID).Error; err != nil || !leader.IsActive || leader.DepartmentID != department.ID {
				c.JSON(400, gin.H{"error": "部门负责人必须是本部门在职员工"})
				return
			}
			if !CanAccessEmployee(actorID(c), *leaderID) {
				c.JSON(403, gin.H{"error": "部门负责人超出数据范围"})
				return
			}
		}
		updates["leader_id"] = leaderID
	}
	result = models.DB.Model(&department).Updates(updates)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "更新部门失败",
			"message": result.Error.Error(),
		})
		return
	}
	models.DB.First(&department, departmentId)
	RecordAuditDetails(c, "update_department", "department", strconv.FormatUint(departmentId, 10), "SUCCESS", "parent_id="+stringParentID(department.ParentID)+";leader_id="+stringParentID(department.LeaderID))

	c.JSON(http.StatusOK, gin.H{
		"message": "部门更新成功",
		"data":    department,
	})
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
