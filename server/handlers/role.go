package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RoleSummary struct {
	models.Role
	DataScope       *models.DataScope `json:"data_scope,omitempty"`
	PermissionCount int64             `json:"permission_count"`
	UserCount       int64             `json:"user_count"`
	Assignable      bool              `json:"assignable"`
}

type RoleMutationRequest struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	DataScopeCode   string `json:"data_scope_code"`
	RequiresManager bool   `json:"requires_manager"`
	PermissionIDs   []uint `json:"permission_ids"`
}

type CloneRoleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type AssignRoleRequest struct {
	RoleCode string `json:"role_code" binding:"required"`
}

type AssignRoleUsersRequest struct {
	UserIDs []uint `json:"user_ids" binding:"required"`
}

func actorID(c *gin.Context) uint {
	value, _ := c.Get("user_id")
	id, _ := value.(uint)
	return id
}

func loadRoleSummary(db *gorm.DB, roleID uint, withPermissions bool) (RoleSummary, error) {
	var role models.Role
	query := db
	if withPermissions {
		query = query.Preload("Permissions", func(query *gorm.DB) *gorm.DB { return query.Order("resource, action") })
	}
	if err := query.First(&role, roleID).Error; err != nil {
		return RoleSummary{}, err
	}
	var permissionCount, userCount int64
	db.Model(&models.RolePermission{}).Where("role_id = ?", role.ID).Count(&permissionCount)
	db.Model(&models.UserRole{}).Where("role_id = ?", role.ID).Count(&userCount)
	var scope models.DataScope
	err := db.Table("data_scopes ds").Select("ds.*").
		Joins("JOIN role_data_scopes rds ON rds.data_scope_id = ds.id").
		Where("rds.role_id = ?", role.ID).First(&scope).Error
	var scopePtr *models.DataScope
	if err == nil {
		scopePtr = &scope
	}
	return RoleSummary{Role: role, DataScope: scopePtr, PermissionCount: permissionCount, UserCount: userCount, Assignable: true}, nil
}

func GetRoles(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "100"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 100
	}
	query := models.DB.Model(&models.Role{})
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		pattern := "%" + search + "%"
		query = query.Where("name LIKE ? OR code LIKE ?", pattern, pattern)
	}
	switch c.Query("type") {
	case "system":
		query = query.Where("is_system = ?", true)
	case "custom":
		query = query.Where("is_system = ?", false)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取角色失败"})
		return
	}
	var roles []models.Role
	if err := query.Order("is_system DESC, id").Offset((page - 1) * pageSize).Limit(pageSize).Find(&roles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取角色失败"})
		return
	}
	items := make([]RoleSummary, 0, len(roles))
	for _, role := range roles {
		item, err := loadRoleSummary(models.DB, role.ID, true)
		if err == nil {
			item.Assignable = CanGrantExistingRole(models.DB, actorID(c), role) == nil
			items = append(items, item)
		}
	}
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	c.JSON(http.StatusOK, gin.H{"data": items, "total": total, "page": page, "pageSize": pageSize, "totalPages": totalPages, "hasNext": page < totalPages, "hasPrev": page > 1})
}

func GetRole(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的角色ID"})
		return
	}
	role, err := loadRoleSummary(models.DB, uint(id), true)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "角色不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取角色失败"})
		return
	}
	role.Assignable = CanGrantExistingRole(models.DB, actorID(c), role.Role) == nil
	c.JSON(http.StatusOK, gin.H{"data": role})
}

func GetPermissions(c *gin.Context) {
	var permissions []models.Permission
	if err := models.DB.Order("resource, action").Find(&permissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取权限失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": permissions, "total": len(permissions)})
}

func GetDataScopes(c *gin.Context) {
	var scopes []models.DataScope
	if err := models.DB.Where("code IN ?", []string{"SELF", "DIRECT_SUBORDINATES", "DEPARTMENT", "ASSIGNED", "ALL"}).Order("id").Find(&scopes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取数据范围失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": scopes, "total": len(scopes)})
}

func validateRoleRequest(db *gorm.DB, c *gin.Context, request RoleMutationRequest, excludeID uint) (RoleMutationRequest, []models.Permission, models.DataScope, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	request.DataScopeCode = strings.ToUpper(strings.TrimSpace(request.DataScopeCode))
	if len([]rune(request.Name)) < 2 || len([]rune(request.Name)) > 50 {
		return request, nil, models.DataScope{}, errors.New("角色名称长度必须为2到50个字符")
	}
	var duplicate int64
	query := db.Model(&models.Role{}).Where("LOWER(TRIM(name)) = LOWER(?)", request.Name)
	if excludeID != 0 {
		query = query.Where("id <> ?", excludeID)
	}
	if err := query.Count(&duplicate).Error; err != nil {
		return request, nil, models.DataScope{}, err
	}
	if duplicate > 0 {
		return request, nil, models.DataScope{}, errors.New("角色名称已存在")
	}
	permissions, err := ValidateGrantableRole(db, actorID(c), request.PermissionIDs, request.DataScopeCode)
	if err != nil {
		return request, nil, models.DataScope{}, err
	}
	var scope models.DataScope
	if err := db.Where("code = ?", request.DataScopeCode).First(&scope).Error; err != nil {
		return request, nil, models.DataScope{}, errors.New("数据范围不存在")
	}
	return request, permissions, scope, nil
}

func replaceRoleAccess(tx *gorm.DB, roleID uint, permissions []models.Permission, scope models.DataScope) error {
	if err := tx.Where("role_id = ?", roleID).Delete(&models.RolePermission{}).Error; err != nil {
		return err
	}
	for _, permission := range permissions {
		if err := tx.Create(&models.RolePermission{RoleID: roleID, PermissionID: permission.ID}).Error; err != nil {
			return err
		}
	}
	if err := tx.Where("role_id = ?", roleID).Delete(&models.RoleDataScope{}).Error; err != nil {
		return err
	}
	return tx.Create(&models.RoleDataScope{RoleID: roleID, DataScopeID: scope.ID}).Error
}

func roleRequestStatus(err error) int {
	if strings.Contains(err.Error(), "无权") || strings.Contains(err.Error(), "自身") || strings.Contains(err.Error(), "范围") {
		return http.StatusForbidden
	}
	return http.StatusBadRequest
}

func CreateRole(c *gin.Context) {
	var request RoleMutationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	request, permissions, scope, err := validateRoleRequest(models.DB, c, request, 0)
	if err != nil {
		RecordAuditDetails(c, "create_role", "role", "", "DENIED", err.Error())
		c.JSON(roleRequestStatus(err), gin.H{"error": err.Error()})
		return
	}
	role := models.Role{Code: "custom_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12], Name: request.Name, Description: request.Description, IsSystem: false, RequiresManager: request.RequiresManager}
	err = models.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&role).Error; err != nil {
			return err
		}
		return replaceRoleAccess(tx, role.ID, permissions, scope)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建角色失败"})
		return
	}
	RecordAuditDetails(c, "create_role", "role", strconv.Itoa(int(role.ID)), "SUCCESS", fmt.Sprintf("name=%s scope=%s permissions=%d", role.Name, scope.Code, len(permissions)))
	result, _ := loadRoleSummary(models.DB, role.ID, true)
	c.JSON(http.StatusCreated, gin.H{"message": "角色创建成功", "data": result})
}

func UpdateRole(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的角色ID"})
		return
	}
	var role models.Role
	if err := models.DB.First(&role, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "角色不存在"})
		return
	}
	if role.IsSystem {
		c.JSON(http.StatusForbidden, gin.H{"error": "系统角色不可编辑，请复制后修改"})
		return
	}
	var request RoleMutationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	request, permissions, scope, err := validateRoleRequest(models.DB, c, request, role.ID)
	if err != nil {
		RecordAuditDetails(c, "update_role", "role", strconv.Itoa(int(role.ID)), "DENIED", err.Error())
		c.JSON(roleRequestStatus(err), gin.H{"error": err.Error()})
		return
	}
	err = models.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&role).Updates(map[string]any{"name": request.Name, "description": request.Description, "requires_manager": request.RequiresManager}).Error; err != nil {
			return err
		}
		return replaceRoleAccess(tx, role.ID, permissions, scope)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新角色失败"})
		return
	}
	RecordAuditDetails(c, "update_role", "role", strconv.Itoa(int(role.ID)), "SUCCESS", fmt.Sprintf("name=%s scope=%s permissions=%d", request.Name, scope.Code, len(permissions)))
	result, _ := loadRoleSummary(models.DB, role.ID, true)
	c.JSON(http.StatusOK, gin.H{"message": "角色更新成功", "data": result})
}

func CloneRole(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的角色ID"})
		return
	}
	var source models.Role
	if err := models.DB.Preload("Permissions").First(&source, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "角色不存在"})
		return
	}
	var request CloneRoleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	permissionIDs := make([]uint, 0, len(source.Permissions))
	for _, permission := range source.Permissions {
		permissionIDs = append(permissionIDs, permission.ID)
	}
	mutation := RoleMutationRequest{Name: request.Name, Description: request.Description, DataScopeCode: RoleScopeCode(models.DB, source.ID), RequiresManager: source.RequiresManager, PermissionIDs: permissionIDs}
	mutation, permissions, scope, err := validateRoleRequest(models.DB, c, mutation, 0)
	if err != nil {
		c.JSON(roleRequestStatus(err), gin.H{"error": err.Error()})
		return
	}
	role := models.Role{Code: "custom_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12], Name: mutation.Name, Description: mutation.Description, IsSystem: false, RequiresManager: mutation.RequiresManager}
	err = models.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&role).Error; err != nil {
			return err
		}
		return replaceRoleAccess(tx, role.ID, permissions, scope)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "复制角色失败"})
		return
	}
	RecordAuditDetails(c, "clone_role", "role", strconv.Itoa(int(role.ID)), "SUCCESS", fmt.Sprintf("source_role_id=%d", source.ID))
	result, _ := loadRoleSummary(models.DB, role.ID, true)
	c.JSON(http.StatusCreated, gin.H{"message": "角色复制成功", "data": result})
}

func DeleteRole(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的角色ID"})
		return
	}
	var role models.Role
	if err := models.DB.First(&role, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "角色不存在"})
		return
	}
	if role.IsSystem {
		c.JSON(http.StatusConflict, gin.H{"error": "系统角色不可删除"})
		return
	}
	var userCount int64
	models.DB.Model(&models.UserRole{}).Where("role_id = ?", role.ID).Count(&userCount)
	if userCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("该角色仍绑定 %d 名用户，请先完成改派", userCount), "user_count": userCount})
		return
	}
	if err := models.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", role.ID).Delete(&models.RolePermission{}).Error; err != nil {
			return err
		}
		if err := tx.Where("role_id = ?", role.ID).Delete(&models.RoleDataScope{}).Error; err != nil {
			return err
		}
		return tx.Delete(&role).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除角色失败"})
		return
	}
	RecordAuditDetails(c, "delete_role", "role", strconv.Itoa(int(role.ID)), "SUCCESS", role.Name)
	c.JSON(http.StatusOK, gin.H{"message": "角色删除成功"})
}

func GetRoleUsers(c *gin.Context) {
	roleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的角色ID"})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	query := ApplyEmployeeScope(models.DB.Model(&models.Employee{}).Preload("Department").Preload("Manager"), actorID(c)).Joins("JOIN user_roles ur ON ur.user_id = employees.id").Where("ur.role_id = ?", roleID)
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		pattern := "%" + search + "%"
		query = query.Where("employees.name LIKE ? OR employees.email LIKE ?", pattern, pattern)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取角色用户失败"})
		return
	}
	var users []models.Employee
	if err := query.Order("employees.id").Offset((page - 1) * pageSize).Limit(pageSize).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取角色用户失败"})
		return
	}
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	c.JSON(http.StatusOK, gin.H{"data": users, "total": total, "page": page, "pageSize": pageSize, "totalPages": totalPages, "hasNext": page < totalPages, "hasPrev": page > 1})
}

func validateRoleAssignment(db *gorm.DB, c *gin.Context, employee models.Employee, role models.Role) error {
	if err := CanGrantExistingRole(db, actorID(c), role); err != nil {
		return err
	}
	last, err := WouldRemoveLastSuperAdmin(db, employee.ID, role.Code)
	if err != nil {
		return err
	}
	if last {
		return errors.New("不能改派最后一个有效超级管理员")
	}
	if (role.RequiresManager || role.Code == "employee") && (employee.ManagerID == nil || *employee.ManagerID == 0) {
		return errors.New("该角色要求用户必须设置直属上级")
	}
	return nil
}

func AssignRoleUsers(c *gin.Context) {
	roleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的角色ID"})
		return
	}
	var role models.Role
	if err := models.DB.First(&role, roleID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "角色不存在"})
		return
	}
	var request AssignRoleUsersRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.UserIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择至少一个用户"})
		return
	}
	var employees []models.Employee
	if err := ApplyEmployeeScope(models.DB.Model(&models.Employee{}), actorID(c)).Where("id IN ?", request.UserIDs).Find(&employees).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取用户失败"})
		return
	}
	unique := make(map[uint]struct{}, len(request.UserIDs))
	for _, id := range request.UserIDs {
		unique[id] = struct{}{}
	}
	if len(employees) != len(unique) {
		c.JSON(http.StatusForbidden, gin.H{"error": "包含不存在或超出数据范围的用户"})
		return
	}
	if err := models.DB.Transaction(func(tx *gorm.DB) error {
		for _, employee := range employees {
			if err := validateRoleAssignment(tx, c, employee, role); err != nil {
				return err
			}
			if err := ReplaceUserRole(tx, employee.ID, role.Code); err != nil {
				return err
			}
			if err := tx.Model(&models.Employee{}).Where("id = ?", employee.ID).Update("role", LegacyRoleValue(role.Code)).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		RecordAuditDetails(c, "assign_role_users", "role", strconv.Itoa(int(role.ID)), "DENIED", err.Error())
		c.JSON(roleRequestStatus(err), gin.H{"error": err.Error()})
		return
	}
	RecordAuditDetails(c, "assign_role_users", "role", strconv.Itoa(int(role.ID)), "SUCCESS", fmt.Sprintf("users=%d", len(employees)))
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("已为 %d 名用户分配角色", len(employees))})
}

func AssignEmployeeRole(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的员工ID"})
		return
	}
	var request AssignRoleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	var role models.Role
	if err := models.DB.Where("code = ?", LegacyRoleCode(request.RoleCode)).First(&role).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "角色不存在"})
		return
	}
	var employee models.Employee
	if err := ApplyEmployeeScope(models.DB.Model(&models.Employee{}), actorID(c)).First(&employee, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "员工不存在或超出数据范围"})
		return
	}
	if err := validateRoleAssignment(models.DB, c, employee, role); err != nil {
		c.JSON(roleRequestStatus(err), gin.H{"error": err.Error()})
		return
	}
	err = models.DB.Transaction(func(tx *gorm.DB) error {
		if err := ReplaceUserRole(tx, uint(userID), role.Code); err != nil {
			return err
		}
		return tx.Model(&models.Employee{}).Where("id = ?", userID).Update("role", LegacyRoleValue(role.Code)).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "角色更新失败"})
		return
	}
	RecordAuditDetails(c, "assign_role", "employee", strconv.FormatUint(userID, 10), "SUCCESS", "role="+role.Code)
	c.JSON(http.StatusOK, gin.H{"message": "角色更新成功", "role": role})
}

func RecordAudit(c *gin.Context, action, resource, resourceID, result string) {
	RecordAuditDetails(c, action, resource, resourceID, result, "")
}

func RecordAuditDetails(c *gin.Context, action, resource, resourceID, result, details string) {
	entry := models.AuditLog{UserID: actorID(c), Action: action, Resource: resource, ResourceID: resourceID, IPAddress: c.ClientIP(), Result: result, Details: details}
	_ = models.DB.Create(&entry).Error
}
