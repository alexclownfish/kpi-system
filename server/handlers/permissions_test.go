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
	"golang.org/x/crypto/bcrypt"
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

func TestCreateEmployeeAcceptsAndHashesInitialPassword(t *testing.T) {
	setupPermissionDB(t)
	gin.SetMode(gin.TestMode)
	department := models.Department{Name: "运维部"}
	role := models.Role{Code: "department_manager", Name: "部门负责人"}
	models.DB.Create(&department)
	models.DB.Create(&role)

	r := gin.New()
	r.POST("/employees", func(c *gin.Context) {
		c.Set("user_id", uint(1))
		c.Next()
	}, CreateEmployee)
	body := fmt.Sprintf(`{"name":"新员工","email":"new-employee@test","password":"12345678","position":"运维","department_id":%d,"role":"department_manager","is_active":true}`, department.ID)
	req := httptest.NewRequest(http.MethodPost, "/employees", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s, want 201", response.Code, response.Body.String())
	}

	var employee models.Employee
	if err := models.DB.Where("email = ?", "new-employee@test").First(&employee).Error; err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(employee.Password), []byte("12345678")); err != nil {
		t.Fatalf("stored password is not the submitted bcrypt hash: %v", err)
	}
	if employee.Role != "manager" {
		t.Fatalf("legacy employee role=%q, want manager", employee.Role)
	}
	var assignment models.UserRole
	if err := models.DB.Where("user_id = ? AND role_id = ?", employee.ID, role.ID).First(&assignment).Error; err != nil {
		t.Fatalf("role assignment was not created: %v", err)
	}
}

func TestCreateEmployeeRejectsInvalidInputsWithoutCreatingRecords(t *testing.T) {
	setupPermissionDB(t)
	gin.SetMode(gin.TestMode)
	deptA := models.Department{Name: "研发部"}
	deptB := models.Department{Name: "销售部"}
	models.DB.Create(&deptA)
	models.DB.Create(&deptB)
	employeeRole := models.Role{Code: "employee", Name: "普通员工"}
	models.DB.Create(&employeeRole)
	activeManager := models.Employee{Name: "在职主管", Email: "active-manager@test", Password: "hash", Role: "manager", DepartmentID: deptA.ID, IsActive: true}
	inactiveManager := models.Employee{Name: "离职主管", Email: "inactive-manager@test", Password: "hash", Role: "manager", DepartmentID: deptA.ID, IsActive: false}
	otherManager := models.Employee{Name: "外部主管", Email: "other-manager@test", Password: "hash", Role: "manager", DepartmentID: deptB.ID, IsActive: true}
	existing := models.Employee{Name: "已有员工", Email: "duplicate@test", Password: "hash", Role: "employee", DepartmentID: deptA.ID, ManagerID: &activeManager.ID, IsActive: true}
	models.DB.Create(&activeManager)
	models.DB.Create(&inactiveManager)
	models.DB.Model(&inactiveManager).Update("is_active", false)
	inactiveManager.IsActive = false
	models.DB.Create(&otherManager)
	existing.ManagerID = &activeManager.ID
	models.DB.Create(&existing)

	r := gin.New()
	r.POST("/employees", func(c *gin.Context) {
		c.Set("user_id", uint(1))
		c.Next()
	}, CreateEmployee)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{"missing password", fmt.Sprintf(`{"name":"无密码","email":"missing-password@test","position":"开发","department_id":%d,"manager_id":%d,"role":"employee","is_active":true}`, deptA.ID, activeManager.ID), http.StatusBadRequest, "初始密码"},
		{"missing manager", fmt.Sprintf(`{"name":"无上级","email":"missing-manager@test","password":"12345678","position":"开发","department_id":%d,"role":"employee","is_active":true}`, deptA.ID), http.StatusBadRequest, "直属上级"},
		{"inactive manager", fmt.Sprintf(`{"name":"离职上级","email":"inactive@test","password":"12345678","position":"开发","department_id":%d,"manager_id":%d,"role":"employee","is_active":true}`, deptA.ID, inactiveManager.ID), http.StatusBadRequest, "已停用"},
		{"manager in another department", fmt.Sprintf(`{"name":"跨部门","email":"wrong-dept@test","password":"12345678","position":"开发","department_id":%d,"manager_id":%d,"role":"employee","is_active":true}`, deptA.ID, otherManager.ID), http.StatusBadRequest, "不属于所选部门"},
		{"duplicate email", fmt.Sprintf(`{"name":"重复邮箱","email":"DUPLICATE@test","password":"12345678","position":"开发","department_id":%d,"manager_id":%d,"role":"employee","is_active":true}`, deptA.ID, activeManager.ID), http.StatusConflict, "邮箱已被使用"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var before int64
			models.DB.Model(&models.Employee{}).Count(&before)
			req := httptest.NewRequest(http.MethodPost, "/employees", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			r.ServeHTTP(response, req)
			if response.Code != tt.wantStatus || !strings.Contains(response.Body.String(), tt.wantError) {
				t.Fatalf("status=%d body=%s, want status=%d error containing %q", response.Code, response.Body.String(), tt.wantStatus, tt.wantError)
			}
			var after int64
			models.DB.Model(&models.Employee{}).Count(&after)
			if after != before {
				t.Fatalf("invalid request created a record: before=%d after=%d", before, after)
			}
		})
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

func TestEmployeeCannotCreateEmployeesOrViewCompanyStatistics(t *testing.T) {
	setupPermissionDB(t)
	gin.SetMode(gin.TestMode)
	user := models.Employee{Name: "员工", Email: "employee-denied@test", Password: "hash", Role: "employee", IsActive: true}
	role := models.Role{Code: "employee", Name: "普通员工"}
	models.DB.Create(&user)
	models.DB.Create(&role)
	models.DB.Create(&models.UserRole{UserID: user.ID, RoleID: role.ID})

	r := gin.New()
	r.POST("/employees", func(c *gin.Context) { c.Set("user_id", user.ID) }, PermissionMiddleware("employee:create"), CreateEmployee)
	r.GET("/statistics/dashboard", func(c *gin.Context) { c.Set("user_id", user.ID) }, PermissionMiddleware("report:company"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"total_employees": 999})
	})
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/employees", strings.NewReader(`{}`)),
		httptest.NewRequest(http.MethodGet, "/statistics/dashboard", nil),
	} {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s %s status=%d body=%s, want 403", request.Method, request.URL.Path, response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "total_employees") {
			t.Fatalf("denial leaked protected statistics: %s", response.Body.String())
		}
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
