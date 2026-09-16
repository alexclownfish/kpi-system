package handlers

import (
	"net/http"
	"strconv"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
)

func GetRoles(c *gin.Context) {
	var roles []models.Role
	if err := models.DB.Preload("Permissions").Order("id").Find(&roles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取角色失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": roles, "total": len(roles)})
}

func GetPermissions(c *gin.Context) {
	var permissions []models.Permission
	if err := models.DB.Order("resource, action").Find(&permissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取权限失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": permissions, "total": len(permissions)})
}

type AssignRoleRequest struct {
	RoleCode string `json:"role_code" binding:"required"`
}

func AssignEmployeeRole(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的员工ID"})
		return
	}
	var req AssignRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "message": err.Error()})
		return
	}
	var role models.Role
	if err := models.DB.Where("code = ?", req.RoleCode).First(&role).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "角色不存在"})
		return
	}
	var employee models.Employee
	if err := models.DB.First(&employee, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "员工不存在"})
		return
	}
	// Super administrator membership is protected by the role-management
	// capability, which is intentionally not granted to HR administrators.
	if (LegacyRoleCode(employee.Role) == "super_admin" || req.RoleCode == "super_admin") && !HasPermission(c, "role:edit") {
		c.JSON(http.StatusForbidden, gin.H{"error": "只有超级管理员可以变更超级管理员角色"})
		return
	}
	tx := models.DB.Begin()
	if err := ReplaceUserRole(tx, uint(userID), role.Code); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "角色更新失败"})
		return
	}
	// Keep the legacy field in sync for old clients and JWT consumers.
	legacy := LegacyRoleValue(req.RoleCode)
	if err := tx.Model(&employee).Update("role", legacy).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "角色更新失败"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "角色更新失败"})
		return
	}
	RecordAudit(c, "assign_role", "employee", strconv.FormatUint(userID, 10), "SUCCESS")
	c.JSON(http.StatusOK, gin.H{"message": "角色更新成功", "role": role})
}

func RecordAudit(c *gin.Context, action, resource, resourceID, result string) {
	userID, _ := c.Get("user_id")
	id, _ := userID.(uint)
	_ = models.DB.Create(&models.AuditLog{UserID: id, Action: action, Resource: resource, ResourceID: resourceID, IPAddress: c.ClientIP(), Result: result}).Error
}
