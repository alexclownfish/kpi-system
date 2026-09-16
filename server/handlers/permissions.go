package handlers

import (
	"net/http"
	"strings"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PermissionMiddleware enforces a catalog permission after authentication.
// Deny-by-default is intentional: a missing role mapping never grants access.
func PermissionMiddleware(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !HasPermission(c, permission) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "权限不足", "permission": permission})
			return
		}
		c.Next()
	}
}

// PermissionAnyMiddleware allows a route shared by related capabilities, such
// as viewing roles while assigning employees or managing role definitions.
func PermissionAnyMiddleware(permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, permission := range permissions {
			if HasPermission(c, permission) {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "权限不足", "permissions": permissions})
	}
}

func HasPermission(c *gin.Context, permission string) bool {
	userID, ok := c.Get("user_id")
	if !ok {
		return false
	}
	id, ok := userID.(uint)
	return ok && UserHasPermission(id, permission)
}

func UserHasPermission(userID uint, permission string) bool {
	var count int64
	models.DB.Table("role_permissions rp").
		Joins("JOIN user_roles ur ON ur.role_id = rp.role_id").
		Joins("JOIN permissions p ON p.id = rp.permission_id").
		Where("ur.user_id = ? AND p.code = ?", userID, permission).Count(&count)
	if count > 0 {
		return true
	}
	// Compatibility for records created before the RBAC migration.
	var employee models.Employee
	if models.DB.Select("role").First(&employee, userID).Error != nil {
		return false
	}
	legacy := LegacyRoleCode(employee.Role)
	var roleID, permissionID uint
	if models.DB.Table("roles").Where("code = ?", legacy).Pluck("id", &roleID).Error != nil {
		return false
	}
	if models.DB.Table("permissions").Where("code = ?", permission).Pluck("id", &permissionID).Error != nil {
		return false
	}
	return models.DB.Table("role_permissions").Where("role_id = ? AND permission_id = ?", roleID, permissionID).Count(&count).Error == nil && count > 0
}

func UserPermissions(userID uint) []string {
	var codes []string
	models.DB.Table("permissions p").Select("p.code").Distinct().
		Joins("JOIN role_permissions rp ON rp.permission_id = p.id").
		Joins("JOIN user_roles ur ON ur.role_id = rp.role_id").Where("ur.user_id = ?", userID).Order("p.code").Pluck("p.code", &codes)
	if len(codes) == 0 {
		var employee models.Employee
		if models.DB.Select("role").First(&employee, userID).Error == nil {
			legacy := LegacyRoleCode(employee.Role)
			models.DB.Table("permissions p").Select("p.code").Distinct().
				Joins("JOIN role_permissions rp ON rp.permission_id = p.id").Joins("JOIN roles r ON r.id = rp.role_id").Where("r.code = ?", legacy).Order("p.code").Pluck("p.code", &codes)
		}
	}
	return codes
}

func LegacyRoleCode(role string) string {
	switch strings.ToLower(role) {
	case "hr":
		return "hr_admin"
	case "manager":
		return "department_manager"
	case "", "employee":
		return "employee"
	case "hr_admin", "super_admin", "performance_admin", "department_manager", "reviewer", "analyst":
		return strings.ToLower(role)
	default:
		return strings.ToLower(role)
	}
}

func LegacyRoleValue(roleCode string) string {
	switch LegacyRoleCode(roleCode) {
	case "hr_admin":
		return "hr"
	case "department_manager":
		return "manager"
	default:
		return LegacyRoleCode(roleCode)
	}
}

// AssignDefaultRole ensures existing and newly created users participate in RBAC.
func AssignDefaultRole(userID uint, legacyRole string) error {
	var role models.Role
	if err := models.DB.Where("code = ?", LegacyRoleCode(legacyRole)).First(&role).Error; err != nil {
		return err
	}
	return models.DB.Where("user_id = ? AND role_id = ?", userID, role.ID).FirstOrCreate(&models.UserRole{UserID: userID, RoleID: role.ID}).Error
}

func ReplaceUserRole(db *gorm.DB, userID uint, roleCode string) error {
	var role models.Role
	if err := db.Where("code = ?", LegacyRoleCode(roleCode)).First(&role).Error; err != nil {
		return err
	}
	if err := db.Where("user_id = ?", userID).Delete(&models.UserRole{}).Error; err != nil {
		return err
	}
	return db.Create(&models.UserRole{UserID: userID, RoleID: role.ID}).Error
}

func DataScopeForUser(userID uint) string {
	var scope string
	err := models.DB.Table("data_scopes ds").Select("ds.code").
		Joins("JOIN role_data_scopes rds ON rds.data_scope_id = ds.id").
		Joins("JOIN user_roles ur ON ur.role_id = rds.role_id").Where("ur.user_id = ?", userID).
		Order("CASE ds.code WHEN 'ALL' THEN 1 WHEN 'DEPARTMENT_TREE' THEN 2 WHEN 'DEPARTMENT' THEN 3 WHEN 'DIRECT_SUBORDINATES' THEN 4 WHEN 'ASSIGNED' THEN 5 ELSE 6 END").
		Limit(1).Scan(&scope).Error
	if err == nil && scope != "" {
		return scope
	}
	var employee models.Employee
	if models.DB.Select("role").First(&employee, userID).Error == nil {
		if employee.Role == "hr" {
			return "ALL"
		}
		if employee.Role == "manager" {
			return "DEPARTMENT"
		}
	}
	return "SELF"
}

// ApplyEmployeeScope adds the server-side employee visibility predicate.
func ApplyEmployeeScope(query *gorm.DB, userID uint) *gorm.DB {
	switch DataScopeForUser(userID) {
	case "ALL":
		return query
	case "DEPARTMENT":
		return query.Where("department_id = (SELECT department_id FROM employees WHERE id = ?)", userID)
	case "DIRECT_SUBORDINATES":
		return query.Where("manager_id = ?", userID)
	case "DEPARTMENT_TREE":
		// Department hierarchy is not yet modeled; department scope is the safe MVP fallback.
		return query.Where("department_id = (SELECT department_id FROM employees WHERE id = ?)", userID)
	case "ASSIGNED":
		return query.Where("1 = 0")
	default:
		return query.Where("id = ?", userID)
	}
}

// ApplyEvaluationScope constrains KPI evaluations through their employee.
// ASSIGNED uses the review invitation table instead of exposing employee data.
func ApplyEvaluationScope(query *gorm.DB, userID uint) *gorm.DB {
	switch DataScopeForUser(userID) {
	case "ALL":
		return query
	case "DEPARTMENT", "DEPARTMENT_TREE":
		return query.Where("kpi_evaluations.employee_id IN (SELECT department_members.id FROM employees department_members WHERE department_members.department_id = (SELECT department_id FROM employees WHERE id = ?))", userID)
	case "DIRECT_SUBORDINATES":
		return query.Where("kpi_evaluations.employee_id IN (SELECT direct_reports.id FROM employees direct_reports WHERE direct_reports.manager_id = ?)", userID)
	case "ASSIGNED":
		return query.Where("kpi_evaluations.id IN (SELECT evaluation_id FROM evaluation_invitations WHERE invitee_id = ?)", userID)
	default:
		return query.Where("kpi_evaluations.employee_id = ?", userID)
	}
}

func CanAccessEmployee(userID, employeeID uint) bool {
	var count int64
	return ApplyEmployeeScope(models.DB.Model(&models.Employee{}), userID).Where("id = ?", employeeID).Count(&count).Error == nil && count > 0
}

func CanAccessEvaluation(userID, evaluationID uint) bool {
	var count int64
	return ApplyEvaluationScope(models.DB.Model(&models.KPIEvaluation{}), userID).Where("kpi_evaluations.id = ?", evaluationID).Count(&count).Error == nil && count > 0
}
