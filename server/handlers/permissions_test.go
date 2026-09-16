package handlers

import (
	"encoding/json"
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

func setupPermissionDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:permissions-test-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	models.DB = db
	if err := db.AutoMigrate(&models.Role{}, &models.Permission{}, &models.UserRole{}, &models.RolePermission{}, &models.DataScope{}, &models.RoleDataScope{}, &models.AuditLog{}, &models.Department{}, &models.Employee{}); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceUserRoleRemovesPreviousRole(t *testing.T) {
	setupPermissionDB(t)
	user := models.Employee{Name: "用户", Email: "roles@test", Role: "employee", IsActive: true}
	employeeRole := models.Role{Code: "employee", Name: "普通员工"}
	managerRole := models.Role{Code: "department_manager", Name: "部门负责人"}
	models.DB.Create(&user)
	models.DB.Create(&employeeRole)
	models.DB.Create(&managerRole)
	models.DB.Create(&models.UserRole{UserID: user.ID, RoleID: employeeRole.ID})

	if err := ReplaceUserRole(models.DB, user.ID, managerRole.Code); err != nil {
		t.Fatal(err)
	}
	var assignments []models.UserRole
	models.DB.Where("user_id = ?", user.ID).Find(&assignments)
	if len(assignments) != 1 || assignments[0].RoleID != managerRole.ID {
		t.Fatalf("unexpected role assignments: %+v", assignments)
	}
}

func TestProtectedPermissionRouteDeniesUnassignedPermission(t *testing.T) {
	setupPermissionDB(t)
	gin.SetMode(gin.TestMode)
	user := models.Employee{Name: "员工", Email: "auth@test", Role: "employee", IsActive: true}
	role := models.Role{Code: "employee", Name: "普通员工"}
	models.DB.Create(&user)
	models.DB.Create(&role)
	token, err := generateToken(&user)
	if err != nil {
		t.Fatal(err)
	}

	called := false
	r := gin.New()
	r.PUT("/settings", AuthMiddleware(), PermissionMiddleware("system:edit"), func(c *gin.Context) {
		called = true
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPut, "/settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	if response.Code != http.StatusForbidden || called {
		t.Fatalf("status=%d called=%v, want 403 and no handler execution", response.Code, called)
	}
}

func TestHRCannotMutateSuperAdmin(t *testing.T) {
	setupPermissionDB(t)
	gin.SetMode(gin.TestMode)
	hr := models.Employee{Name: "HR", Email: "hr@test", Role: "hr", IsActive: true}
	target := models.Employee{Name: "超级管理员", Email: "target@test", Role: "super_admin", IsActive: true}
	hrRole := models.Role{Code: "hr_admin", Name: "HR管理员"}
	superRole := models.Role{Code: "super_admin", Name: "超级管理员"}
	assignPermission := models.Permission{Code: "employee:assign_role", Name: "分配角色", Resource: "employee", Action: "assign_role"}
	models.DB.Create(&hr)
	models.DB.Create(&target)
	models.DB.Create(&hrRole)
	models.DB.Create(&superRole)
	models.DB.Create(&assignPermission)
	models.DB.Create(&models.UserRole{UserID: hr.ID, RoleID: hrRole.ID})
	models.DB.Create(&models.RolePermission{RoleID: hrRole.ID, PermissionID: assignPermission.ID})

	r := gin.New()
	r.PUT("/employees/:id/roles", func(c *gin.Context) {
		c.Set("user_id", hr.ID)
		c.Next()
	}, PermissionMiddleware("employee:assign_role"), AssignEmployeeRole)
	r.PUT("/employees/:id", func(c *gin.Context) {
		c.Set("user_id", hr.ID)
		c.Next()
	}, UpdateEmployee)
	r.DELETE("/employees/:id", func(c *gin.Context) {
		c.Set("user_id", hr.ID)
		c.Next()
	}, DeleteEmployee)
	body := `{"role_code":"super_admin"}`
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/employees/%d/roles", target.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		var payload map[string]any
		_ = json.Unmarshal(response.Body.Bytes(), &payload)
		t.Fatalf("status=%d body=%v, want 403", response.Code, payload)
	}
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		req = httptest.NewRequest(method, fmt.Sprintf("/employees/%d", target.ID), strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		response = httptest.NewRecorder()
		r.ServeHTTP(response, req)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s status=%d, want 403", method, response.Code)
		}
	}
}

func TestUserHasPermissionDenyByDefault(t *testing.T) {
	setupPermissionDB(t)
	user := models.Employee{Name: "员工", Email: "employee@test", Role: "employee", IsActive: true}
	models.DB.Create(&user)
	role := models.Role{Code: "employee", Name: "普通员工"}
	models.DB.Create(&role)
	permission := models.Permission{Code: "employee:view", Name: "查看员工", Resource: "employee", Action: "view"}
	models.DB.Create(&permission)
	if UserHasPermission(user.ID, permission.Code) {
		t.Fatal("permission must be denied without a user_roles assignment")
	}
	models.DB.Create(&models.UserRole{UserID: user.ID, RoleID: role.ID})
	models.DB.Create(&models.RolePermission{RoleID: role.ID, PermissionID: permission.ID})
	if !UserHasPermission(user.ID, permission.Code) {
		t.Fatal("assigned permission must be allowed")
	}
}

func TestApplyEmployeeScope(t *testing.T) {
	setupPermissionDB(t)
	deptA := models.Department{Name: "A"}
	deptB := models.Department{Name: "B"}
	models.DB.Create(&deptA)
	models.DB.Create(&deptB)
	manager := models.Employee{Name: "经理", Email: "manager@test", Role: "manager", DepartmentID: deptA.ID, IsActive: true}
	peer := models.Employee{Name: "同事", Email: "peer@test", Role: "employee", DepartmentID: deptA.ID, ManagerID: &manager.ID, IsActive: true}
	other := models.Employee{Name: "他部", Email: "other@test", Role: "employee", DepartmentID: deptB.ID, IsActive: true}
	models.DB.Create(&manager)
	models.DB.Create(&peer)
	models.DB.Create(&other)
	role := models.Role{Code: "department_manager", Name: "部门负责人"}
	scope := models.DataScope{Code: "DEPARTMENT", Name: "本部门"}
	models.DB.Create(&role)
	models.DB.Create(&scope)
	models.DB.Create(&models.UserRole{UserID: manager.ID, RoleID: role.ID})
	models.DB.Create(&models.RoleDataScope{RoleID: role.ID, DataScopeID: scope.ID})
	var visible []models.Employee
	ApplyEmployeeScope(models.DB.Model(&models.Employee{}), manager.ID).Find(&visible)
	if len(visible) != 2 {
		t.Fatalf("department scope returned %d employees, want 2", len(visible))
	}
	for _, e := range visible {
		if e.DepartmentID != deptA.ID {
			t.Fatalf("scope leaked employee %d from department %d", e.ID, e.DepartmentID)
		}
	}

	employeeRole := models.Role{Code: "employee", Name: "普通员工"}
	selfScope := models.DataScope{Code: "SELF", Name: "本人"}
	models.DB.Create(&employeeRole)
	models.DB.Create(&selfScope)
	models.DB.Create(&models.UserRole{UserID: peer.ID, RoleID: employeeRole.ID})
	models.DB.Create(&models.RoleDataScope{RoleID: employeeRole.ID, DataScopeID: selfScope.ID})
	visible = nil
	ApplyEmployeeScope(models.DB.Model(&models.Employee{}), peer.ID).Find(&visible)
	if len(visible) != 1 || visible[0].ID != peer.ID {
		t.Fatalf("self scope returned %+v", visible)
	}
}

func TestApplyEvaluationScopePreventsCrossEmployeeAccess(t *testing.T) {
	setupPermissionDB(t)
	if err := models.DB.AutoMigrate(&models.KPITemplate{}, &models.KPIEvaluation{}); err != nil {
		t.Fatal(err)
	}
	dept := models.Department{Name: "A"}
	models.DB.Create(&dept)
	owner := models.Employee{Name: "本人", Email: "owner@test", Role: "employee", DepartmentID: dept.ID, IsActive: true}
	other := models.Employee{Name: "他人", Email: "other-eval@test", Role: "employee", DepartmentID: dept.ID, IsActive: true}
	models.DB.Create(&owner)
	models.DB.Create(&other)
	role := models.Role{Code: "employee", Name: "普通员工"}
	scope := models.DataScope{Code: "SELF", Name: "本人"}
	models.DB.Create(&role)
	models.DB.Create(&scope)
	models.DB.Create(&models.UserRole{UserID: owner.ID, RoleID: role.ID})
	models.DB.Create(&models.RoleDataScope{RoleID: role.ID, DataScopeID: scope.ID})
	template := models.KPITemplate{Name: "模板", Period: "monthly"}
	models.DB.Create(&template)
	ownEvaluation := models.KPIEvaluation{EmployeeID: owner.ID, TemplateID: template.ID, Period: "monthly"}
	otherEvaluation := models.KPIEvaluation{EmployeeID: other.ID, TemplateID: template.ID, Period: "monthly"}
	models.DB.Create(&ownEvaluation)
	models.DB.Create(&otherEvaluation)

	if !CanAccessEvaluation(owner.ID, ownEvaluation.ID) {
		t.Fatal("owner should access own evaluation")
	}
	if CanAccessEvaluation(owner.ID, otherEvaluation.ID) {
		t.Fatal("self scope leaked another employee evaluation")
	}
}
