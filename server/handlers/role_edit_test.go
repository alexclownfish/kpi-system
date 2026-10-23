package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
)

func TestSystemRolePermissionEditing(t *testing.T) {
	setupPermissionDB(t)
	db := models.DB
	permissions := []models.Permission{
		{Code: "role:view", Resource: "role", Action: "view"},
		{Code: "role:edit", Resource: "role", Action: "edit"},
		{Code: "assessment:view", Resource: "assessment", Action: "view"},
		{Code: "assessment:review", Resource: "assessment", Action: "review"},
	}
	db.Create(&permissions)
	scope := models.DataScope{Code: "ALL", Name: "全公司"}
	self := models.DataScope{Code: "SELF", Name: "本人"}
	db.Create(&scope)
	db.Create(&self)
	adminRole := models.Role{Code: "super_admin", Name: "超级管理员", IsSystem: true}
	target := models.Role{Code: "pm", Name: "项目经理", IsSystem: true}
	other := models.Role{Code: "product_manager", Name: "产品经理", IsSystem: true}
	db.Create(&adminRole)
	db.Create(&target)
	db.Create(&other)
	for _, role := range []models.Role{adminRole, target, other} {
		db.Create(&models.RoleDataScope{RoleID: role.ID, DataScopeID: scope.ID})
		for _, p := range permissions {
			db.Create(&models.RolePermission{RoleID: role.ID, PermissionID: p.ID})
		}
	}
	admin := models.Employee{Name: "管理员", Email: "role-edit-admin@test", Role: "super_admin", IsActive: true}
	member := models.Employee{Name: "项目经理", Email: "role-edit-member@test", Role: "pm", IsActive: true}
	db.Create(&admin)
	db.Create(&member)
	db.Create(&models.UserRole{UserID: admin.ID, RoleID: adminRole.ID})
	db.Create(&models.UserRole{UserID: member.ID, RoleID: target.ID})
	actor := admin.ID
	r := gin.New()
	r.PUT("/roles/:id", func(c *gin.Context) { c.Set("user_id", actor) }, PermissionMiddleware("role:edit"), UpdateRole)
	request := func(id uint, payload RoleMutationRequest, want int) {
		t.Helper()
		body, _ := json.Marshal(payload)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("PUT", fmt.Sprintf("/roles/%d", id), strings.NewReader(string(body))))
		if w.Code != want {
			t.Fatalf("role=%d status=%d want=%d body=%s", id, w.Code, want, w.Body.String())
		}
	}
	payload := RoleMutationRequest{Name: target.Name, Description: "定制项目权限", DataScopeCode: "SELF", PermissionIDs: []uint{permissions[3].ID}}
	request(target.ID, payload, 200)
	if !UserHasPermission(member.ID, "assessment:review") || UserHasPermission(member.ID, "role:edit") {
		t.Fatal("saved target permission not enforced")
	}
	if DataScopeForUser(member.ID) != "SELF" {
		t.Fatal("scope not saved")
	}
	var otherCount, bindings, success int64
	db.Model(&models.RolePermission{}).Where("role_id = ?", other.ID).Count(&otherCount)
	db.Model(&models.UserRole{}).Where("user_id = ? AND role_id = ?", member.ID, target.ID).Count(&bindings)
	db.Model(&models.AuditLog{}).Where("action = ? AND result = ?", "update_role", "SUCCESS").Count(&success)
	if otherCount != 4 || bindings != 1 || success != 1 {
		t.Fatalf("isolation/audit: other=%d bindings=%d success=%d", otherCount, bindings, success)
	}
	for _, mutation := range []RoleMutationRequest{
		{Name: "改名项目经理", DataScopeCode: "SELF"},
		{Name: target.Name, DataScopeCode: "SELF", RequiresManager: true},
	} {
		request(target.ID, mutation, 400)
	}
	request(adminRole.ID, RoleMutationRequest{Name: adminRole.Name, DataScopeCode: "ALL"}, 403)
	payload.PermissionIDs = []uint{99999}
	request(target.ID, payload, 400)
	if !UserHasPermission(member.ID, "assessment:review") {
		t.Fatal("denied write changed role")
	}
	actor = member.ID
	payload.PermissionIDs = nil
	request(target.ID, payload, 403)
	actor = admin.ID
	request(target.ID, payload, 200)
	if len(UserPermissions(member.ID)) != 0 || UserHasPermission(member.ID, "assessment:review") {
		t.Fatal("empty role regained legacy permission")
	}
}

func TestRoleEditCannotAlterHigherPrivilegeTarget(t *testing.T) {
	setupPermissionDB(t)
	db := models.DB
	edit := models.Permission{Code: "role:edit", Resource: "role", Action: "edit"}
	view := models.Permission{Code: "role:view", Resource: "role", Action: "view"}
	high := models.Permission{Code: "system:edit", Resource: "system", Action: "edit"}
	db.Create(&edit)
	db.Create(&view)
	db.Create(&high)
	scope := models.DataScope{Code: "SELF"}
	db.Create(&scope)
	actorRole := models.Role{Code: "custom_editor", Name: "角色编辑员"}
	target := models.Role{Code: "cto", Name: "CTO", IsSystem: true}
	db.Create(&actorRole)
	db.Create(&target)
	for _, p := range []models.Permission{edit, view} {
		db.Create(&models.RolePermission{RoleID: actorRole.ID, PermissionID: p.ID})
	}
	db.Create(&models.RolePermission{RoleID: target.ID, PermissionID: high.ID})
	for _, role := range []models.Role{actorRole, target} {
		db.Create(&models.RoleDataScope{RoleID: role.ID, DataScopeID: scope.ID})
	}
	actor := models.Employee{Name: "编辑员", Email: "role-editor-low@test", Role: actorRole.Code, IsActive: true}
	db.Create(&actor)
	db.Create(&models.UserRole{UserID: actor.ID, RoleID: actorRole.ID})
	r := gin.New()
	r.PUT("/roles/:id", func(c *gin.Context) { c.Set("user_id", actor.ID) }, PermissionMiddleware("role:edit"), UpdateRole)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("PUT", fmt.Sprintf("/roles/%d", target.ID), strings.NewReader(`{"name":"CTO","data_scope_code":"SELF","permission_ids":[]}`)))
	if w.Code != 403 {
		t.Fatalf("privileged target edit accepted: %d %s", w.Code, w.Body.String())
	}
	var count int64
	db.Model(&models.RolePermission{}).Where("role_id = ? AND permission_id = ?", target.ID, high.ID).Count(&count)
	if count != 1 {
		t.Fatal("rejected edit changed privileged role")
	}
}

func TestBoundRoleNeverRegainsLegacyAccess(t *testing.T) {
	setupPermissionDB(t)
	db := models.DB
	legacy := models.Role{Code: "hr_admin", Name: "HR"}
	bound := models.Role{Code: "custom_empty", Name: "无权限角色"}
	db.Create(&legacy)
	db.Create(&bound)
	p := models.Permission{Code: "role:edit", Resource: "role", Action: "edit"}
	db.Create(&p)
	db.Create(&models.RolePermission{RoleID: legacy.ID, PermissionID: p.ID})
	user := models.Employee{Name: "旧HR", Email: "legacy-bound@test", Role: "hr", IsActive: true}
	db.Create(&user)
	db.Create(&models.UserRole{UserID: user.ID, RoleID: bound.ID})
	if UserHasPermission(user.ID, p.Code) || len(UserPermissions(user.ID)) != 0 || DataScopeForUser(user.ID) != "SELF" {
		t.Fatal("explicit empty role fell back to legacy HR")
	}
	db.Where("user_id = ?", user.ID).Delete(&models.UserRole{})
	if !UserHasPermission(user.ID, p.Code) || len(UserPermissions(user.ID)) != 1 {
		t.Fatal("unmigrated user lost compatibility")
	}
}

func TestDepartmentTreeGrantScopeBoundary(t *testing.T) {
	if !IsAssignableDataScope("DEPARTMENT_TREE") || !CanGrantDataScope("ALL", "DEPARTMENT_TREE") || !CanGrantDataScope("DEPARTMENT_TREE", "DEPARTMENT") || CanGrantDataScope("DEPARTMENT", "DEPARTMENT_TREE") {
		t.Fatal("department subtree grant ceiling incorrect")
	}
}
