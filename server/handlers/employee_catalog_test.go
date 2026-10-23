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
)

func TestEmployeeCatalogPaginationScopeAndActiveFilter(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	previous := models.DB
	models.DB = db
	t.Cleanup(func() { models.DB = previous })
	for i := 206; i >= 1; i-- {
		department := uint(2)
		if i <= 7 {
			department = 1
		}
		role := "employee"
		if i == 11 {
			role = "cto"
		}
		if i == 12 {
			role = "manager"
		}
		person := models.Employee{ID: uint(i), Name: fmt.Sprintf("员工%03d", i), Email: fmt.Sprintf("catalog-%d@test.invalid", i), DepartmentID: department, Role: role, IsActive: i != 206, Password: "private-fixture-hash"}
		if err := db.Create(&person).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&models.Employee{}).Where("id = ?", 206).Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}
	permission := models.Permission{Code: "employee:view", Resource: "employee", Action: "view"}
	role := models.Role{Code: "catalog_admin", Name: "目录维护"}
	scope := models.DataScope{Code: "ALL"}
	db.Create(&permission)
	db.Create(&role)
	db.Create(&scope)
	db.Create(&models.RolePermission{RoleID: role.ID, PermissionID: permission.ID})
	db.Create(&models.RoleDataScope{RoleID: role.ID, DataScopeID: scope.ID})
	db.Create(&models.UserRole{UserID: 1, RoleID: role.ID})
	router := gin.New()
	router.GET("/employees", func(c *gin.Context) { c.Set("user_id", uint(1)) }, PermissionMiddleware("employee:view"), GetEmployees)
	type response struct {
		Data       []models.Employee `json:"data"`
		Total      int               `json:"total"`
		TotalPages int               `json:"totalPages"`
		PageSize   int               `json:"pageSize"`
	}
	request := func(query string) response {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/employees?"+query, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d %s", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "private-fixture-hash") {
			t.Fatal("password leaked")
		}
		var result response
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	var ids []uint
	for page := 1; page <= 3; page++ {
		result := request(fmt.Sprintf("page=%d&pageSize=100&is_active=true", page))
		if result.Total != 205 || result.TotalPages != 3 || result.PageSize != 100 {
			t.Fatalf("total=%d pages=%d pageSize=%d", result.Total, result.TotalPages, result.PageSize)
		}
		for _, person := range result.Data {
			ids = append(ids, person.ID)
		}
	}
	if len(ids) != 205 {
		t.Fatalf("employees=%d", len(ids))
	}
	for index, id := range ids {
		if id != uint(index+1) {
			t.Fatalf("unstable order/omission at %d: %d", index, id)
		}
	}
	if result := request("page=1&pageSize=500&is_active=true"); len(result.Data) != 10 || result.PageSize != 10 {
		t.Fatal("invalid page-size contract changed")
	}
	if result := request("page=1&pageSize=100&is_active=false"); result.Total != 1 || result.Data[0].ID != 206 {
		t.Fatal("active filter failed")
	}
	if result := request("page=1&pageSize=100&is_active=true&search=不存在"); result.Total != 0 {
		t.Fatal("empty search failed")
	}
	if result := request("page=1&pageSize=100&is_active=true&search=CATALOG-3@TEST.INVALID"); result.Total != 1 || len(result.Data) != 1 || result.Data[0].ID != 3 {
		t.Fatal("employee email search must be case-insensitive on SQLite and PostgreSQL")
	}
	db.Model(&scope).Update("code", "DEPARTMENT")
	result := request("page=1&pageSize=100&is_active=true")
	if result.Total != 7 || len(result.Data) != 7 {
		t.Fatal("department scope not enforced")
	}
	for _, person := range result.Data {
		if person.DepartmentID != 1 {
			t.Fatal("outside department employee leaked")
		}
	}
	if result := request("page=1&pageSize=100&is_active=true&department_id=2"); result.Total != 0 {
		t.Fatal("explicit filter bypassed scope")
	}
	db.Where("role_id = ?", role.ID).Delete(&models.RolePermission{})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/employees?pageSize=100&is_active=true", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("revoked read allowed: %d", w.Code)
	}
}
