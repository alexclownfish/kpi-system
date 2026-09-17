package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type roleTestFixture struct {
	admin       models.Employee
	superRole   models.Role
	employeeRole models.Role
	permissions []models.Permission
}

func setupRoleDB(t *testing.T) roleTestFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:role-test-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	models.DB = db
	if err := db.AutoMigrate(&models.Role{}, &models.Permission{}, &models.UserRole{}, &models.RolePermission{}, &models.DataScope{}, &models.RoleDataScope{}, &models.AuditLog{}, &models.Department{}, &models.Employee{}); err != nil {
		t.Fatal(err)
	}
	department := models.Department{Name: "测试部门"}
	db.Create(&department)
	permissions := []models.Permission{
		{Code: "role:view", Name: "查看角色", Resource: "role", Action: "view"},
		{Code: "role:create", Name: "创建角色", Resource: "role", Action: "create"},
		{Code: "role:edit", Name: "编辑角色", Resource: "role", Action: "edit"},
		{Code: "role:delete", Name: "删除角色", Resource: "role", Action: "delete"},
		{Code: "employee:view", Name: "查看员工", Resource: "employee", Action: "view"},
		{Code: "employee:assign_role", Name: "分配角色", Resource: "employee", Action: "assign_role"},
	}
	for index := range permissions {
		db.Create(&permissions[index])
	}
	scopes := []models.DataScope{{Code: "SELF", Name: "本人"}, {Code: "DEPARTMENT", Name: "本部门"}, {Code: "ALL", Name: "全公司"}}
	for index := range scopes {
		db.Create(&scopes[index])
	}
	superRole := models.Role{Code: "super_admin", Name: "超级管理员", IsSystem: true}
	employeeRole := models.Role{Code: "employee", Name: "普通员工", IsSystem: true, RequiresManager: true}
	db.Create(&superRole)
	db.Create(&employeeRole)
	for _, permission := range permissions {
		db.Create(&models.RolePermission{RoleID: superRole.ID, PermissionID: permission.ID})
	}
	for _, permission := range permissions {
		if permission.Code == "employee:view" {
			db.Create(&models.RolePermission{RoleID: employeeRole.ID, PermissionID: permission.ID})
		}
	}
	db.Create(&models.RoleDataScope{RoleID: superRole.ID, DataScopeID: scopes[2].ID})
	db.Create(&models.RoleDataScope{RoleID: employeeRole.ID, DataScopeID: scopes[0].ID})
	admin := models.Employee{Name: "管理员", Email: fmt.Sprintf("admin-%s@test.local", t.Name()), Password: "hash", DepartmentID: department.ID, Role: "super_admin", IsActive: true}
	db.Create(&admin)
	db.Create(&models.UserRole{UserID: admin.ID, RoleID: superRole.ID})
	return roleTestFixture{admin: admin, superRole: superRole, employeeRole: employeeRole, permissions: permissions}
}

func roleRouter(method, path string, userID uint, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Handle(method, path, func(c *gin.Context) { c.Set("user_id", userID) }, handler)
	return router
}

func performRoleRequest(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestCreateRoleCompletesViewDependencyAndRejectsDuplicate(t *testing.T) {
	fixture := setupRoleDB(t)
	var assignPermission models.Permission
	models.DB.Where("code = ?", "employee:assign_role").First(&assignPermission)
	body := fmt.Sprintf(`{"name":"绩效复核员","description":"测试角色","data_scope_code":"DEPARTMENT","requires_manager":true,"permission_ids":[%d]}`, assignPermission.ID)
	router := roleRouter(http.MethodPost, "/roles", fixture.admin.ID, CreateRole)
	response := performRoleRequest(router, http.MethodPost, "/roles", body)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var role models.Role
	if err := models.DB.Where("name = ?", "绩效复核员").First(&role).Error; err != nil {
		t.Fatal(err)
	}
	if role.IsSystem || !role.RequiresManager || !strings.HasPrefix(role.Code, "custom_") {
		t.Fatalf("unexpected role: %+v", role)
	}
	var permissionCount int64
	models.DB.Table("role_permissions rp").Joins("JOIN permissions p ON p.id = rp.permission_id").Where("rp.role_id = ? AND p.code IN ?", role.ID, []string{"employee:view", "employee:assign_role"}).Count(&permissionCount)
	if permissionCount != 2 {
		t.Fatalf("permission dependencies count=%d, want 2", permissionCount)
	}
	response = performRoleRequest(router, http.MethodPost, "/roles", body)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "已存在") {
		t.Fatalf("duplicate status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCreateRoleRejectsPrivilegeEscalationWithoutPartialWrite(t *testing.T) {
	fixture := setupRoleDB(t)
	limitedRole := models.Role{Code: "limited", Name: "有限管理员", IsSystem: false}
	models.DB.Create(&limitedRole)
	var roleCreate models.Permission
	models.DB.Where("code = ?", "role:create").First(&roleCreate)
	models.DB.Create(&models.RolePermission{RoleID: limitedRole.ID, PermissionID: roleCreate.ID})
	var departmentScope models.DataScope
	models.DB.Where("code = ?", "DEPARTMENT").First(&departmentScope)
	models.DB.Create(&models.RoleDataScope{RoleID: limitedRole.ID, DataScopeID: departmentScope.ID})
	limited := models.Employee{Name: "有限管理员", Email: "limited@test.local", Password: "hash", DepartmentID: fixture.admin.DepartmentID, Role: limitedRole.Code, IsActive: true}
	models.DB.Create(&limited)
	models.DB.Create(&models.UserRole{UserID: limited.ID, RoleID: limitedRole.ID})
	var forbiddenPermission models.Permission
	models.DB.Where("code = ?", "role:delete").First(&forbiddenPermission)
	body := fmt.Sprintf(`{"name":"越权角色","data_scope_code":"ALL","permission_ids":[%d]}`, forbiddenPermission.ID)
	response := performRoleRequest(roleRouter(http.MethodPost, "/roles", limited.ID, CreateRole), http.MethodPost, "/roles", body)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var count int64
	models.DB.Model(&models.Role{}).Where("name = ?", "越权角色").Count(&count)
	if count != 0 {
		t.Fatal("rejected role must not be written")
	}
}

func TestSystemAndInUseRoleProtection(t *testing.T) {
	fixture := setupRoleDB(t)
	updateBody := `{"name":"已修改系统角色","data_scope_code":"ALL","permission_ids":[]}`
	updatePath := fmt.Sprintf("/roles/%d", fixture.superRole.ID)
	response := performRoleRequest(roleRouter(http.MethodPut, "/roles/:id", fixture.admin.ID, UpdateRole), http.MethodPut, updatePath, updateBody)
	if response.Code != http.StatusForbidden {
		t.Fatalf("system update status=%d body=%s", response.Code, response.Body.String())
	}
	custom := models.Role{Code: "custom_in_use", Name: "在用角色", IsSystem: false}
	models.DB.Create(&custom)
	models.DB.Create(&models.RoleDataScope{RoleID: custom.ID, DataScopeID: 1})
	user := models.Employee{Name: "用户", Email: "user@test.local", Password: "hash", DepartmentID: fixture.admin.DepartmentID, Role: custom.Code, IsActive: true}
	models.DB.Create(&user)
	models.DB.Create(&models.UserRole{UserID: user.ID, RoleID: custom.ID})
	deletePath := fmt.Sprintf("/roles/%d", custom.ID)
	response = performRoleRequest(roleRouter(http.MethodDelete, "/roles/:id", fixture.admin.ID, DeleteRole), http.MethodDelete, deletePath, "")
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "1 名用户") {
		t.Fatalf("in-use delete status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCloneSystemRoleCreatesEditableCustomRole(t *testing.T) {
	fixture := setupRoleDB(t)
	path := fmt.Sprintf("/roles/%d/clone", fixture.superRole.ID)
	response := performRoleRequest(roleRouter(http.MethodPost, "/roles/:id/clone", fixture.admin.ID, CloneRole), http.MethodPost, path, `{"name":"超级管理员只读副本","description":"用于测试复制"}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var cloned models.Role
	if err := models.DB.Where("name = ?", "超级管理员只读副本").First(&cloned).Error; err != nil {
		t.Fatal(err)
	}
	if cloned.IsSystem || !strings.HasPrefix(cloned.Code, "custom_") {
		t.Fatalf("cloned role must be custom: %+v", cloned)
	}
	var sourceCount, cloneCount int64
	models.DB.Model(&models.RolePermission{}).Where("role_id = ?", fixture.superRole.ID).Count(&sourceCount)
	models.DB.Model(&models.RolePermission{}).Where("role_id = ?", cloned.ID).Count(&cloneCount)
	if sourceCount != cloneCount {
		t.Fatalf("permission count source=%d clone=%d", sourceCount, cloneCount)
	}
}

func TestRoleUsersHonorsEmployeeDataScope(t *testing.T) {
	fixture := setupRoleDB(t)
	var viewPermission models.Permission
	models.DB.Where("code = ?", "role:view").First(&viewPermission)
	var selfScope models.DataScope
	models.DB.Where("code = ?", "SELF").First(&selfScope)
	role := models.Role{Code: "custom_self_auditor", Name: "本人范围审计员", IsSystem: false}
	models.DB.Create(&role)
	models.DB.Create(&models.RolePermission{RoleID: role.ID, PermissionID: viewPermission.ID})
	models.DB.Create(&models.RoleDataScope{RoleID: role.ID, DataScopeID: selfScope.ID})
	actor := models.Employee{Name: "范围审计员", Email: "scope-actor@test.local", Password: "hash", DepartmentID: fixture.admin.DepartmentID, Role: role.Code, IsActive: true}
	other := models.Employee{Name: "范围外用户", Email: "scope-other@test.local", Password: "hash", DepartmentID: fixture.admin.DepartmentID, Role: role.Code, IsActive: true}
	models.DB.Create(&actor)
	models.DB.Create(&other)
	models.DB.Create(&models.UserRole{UserID: actor.ID, RoleID: role.ID})
	models.DB.Create(&models.UserRole{UserID: other.ID, RoleID: role.ID})
	path := fmt.Sprintf("/roles/%d/users", role.ID)
	response := performRoleRequest(roleRouter(http.MethodGet, "/roles/:id/users", actor.ID, GetRoleUsers), http.MethodGet, path, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), actor.Email) || strings.Contains(response.Body.String(), other.Email) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestBulkAssignmentIsAtomicAndProtectsLastSuperAdmin(t *testing.T) {
	fixture := setupRoleDB(t)
	manager := fixture.admin.ID
	user := models.Employee{Name: "待分配用户", Email: "assign@test.local", Password: "hash", DepartmentID: fixture.admin.DepartmentID, ManagerID: &manager, Role: "employee", IsActive: true}
	models.DB.Create(&user)
	models.DB.Create(&models.UserRole{UserID: user.ID, RoleID: fixture.employeeRole.ID})
	path := fmt.Sprintf("/roles/%d/users", fixture.employeeRole.ID)
	body := fmt.Sprintf(`{"user_ids":[%d,%d]}`, user.ID, fixture.admin.ID)
	response := performRoleRequest(roleRouter(http.MethodPut, "/roles/:id/users", fixture.admin.ID, AssignRoleUsers), http.MethodPut, path, body)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "最后一个") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	role, err := UserPrimaryRole(models.DB, user.ID)
	if err != nil || role.ID != fixture.employeeRole.ID {
		t.Fatalf("atomic rollback failed: role=%+v err=%v", role, err)
	}
	adminRole, err := UserPrimaryRole(models.DB, fixture.admin.ID)
	if err != nil || adminRole.ID != fixture.superRole.ID {
		t.Fatalf("super admin changed: role=%+v err=%v", adminRole, err)
	}
}
