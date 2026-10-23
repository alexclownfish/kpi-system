package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func leaderTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := catalogTestDB(t)
	if !db.Migrator().HasColumn(&models.Department{}, "leader_id") {
		if err := db.Exec("ALTER TABLE departments ADD COLUMN leader_id INTEGER").Error; err != nil {
			t.Fatal(err)
		}
	}
	old := models.DB
	models.DB = db
	t.Cleanup(func() { models.DB = old })
	return db
}

func TestDepartmentLeaderMaintenanceAndScope(t *testing.T) {
	db := leaderTestDB(t)
	dept := models.Department{Name: "自定义父部门"}
	db.Create(&dept)
	other := models.Department{Name: "其它部门"}
	db.Create(&other)
	actor := createWorkflowFixtureEmployee(t, db, "管理员", "leader-admin@test", "管理", dept.ID)
	hierarchyScopeRole(t, db, actor.ID, "ALL")
	leader := createWorkflowFixtureEmployee(t, db, "实际负责人", "leader@test", "自定义岗位", dept.ID)
	outside := createWorkflowFixtureEmployee(t, db, "其它人", "outside-leader@test", "负责人", other.ID)
	r := gin.New()
	r.PUT("/departments/:id", func(c *gin.Context) { c.Set("user_id", actor.ID) }, UpdateDepartment)
	update := func(body string, status int) {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("PUT", fmt.Sprintf("/departments/%d", dept.ID), strings.NewReader(body)))
		if w.Code != status {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
	}
	getLeader := func() *uint {
		var row struct{ LeaderID *uint }
		db.Table("departments").Select("leader_id").Where("id = ?", dept.ID).Scan(&row)
		return row.LeaderID
	}
	update(fmt.Sprintf(`{"leader_id":%d}`, leader.ID), 200)
	if id := getLeader(); id == nil || *id != leader.ID {
		t.Fatal("department leader not saved")
	}
	update(`{"description":"保留负责人"}`, 200)
	if id := getLeader(); id == nil || *id != leader.ID {
		t.Fatal("omitted leader cleared")
	}
	update(fmt.Sprintf(`{"leader_id":%d}`, outside.ID), 400)
	db.Model(&leader).Update("is_active", false)
	update(fmt.Sprintf(`{"leader_id":%d}`, leader.ID), 400)
	db.Model(&leader).Update("is_active", true)
	limited := createWorkflowFixtureEmployee(t, db, "受限管理员", "limited-leader@test", "管理", other.ID)
	hierarchyScopeRole(t, db, limited.ID, "DEPARTMENT")
	actor.ID = limited.ID
	update(fmt.Sprintf(`{"leader_id":%d}`, leader.ID), 403)
	actor.ID = leader.ID
	hierarchyScopeRole(t, db, actor.ID, "ALL")
	update(`{"leader_id":null}`, 200)
	if getLeader() != nil {
		t.Fatal("explicit null did not clear leader")
	}
}

func TestExplicitParentLeaderWithoutDuplicateResponsibility(t *testing.T) {
	db := leaderTestDB(t)
	parent := models.Department{Name: "自定义父部门"}
	db.Create(&parent)
	child := models.Department{Name: "自定义子部门", ParentID: &parent.ID}
	db.Create(&child)
	leader := createWorkflowFixtureEmployee(t, db, "父负责人", "parent-leader@test", "任意岗位", parent.ID)
	head := createWorkflowFixtureEmployee(t, db, "子负责人", "child-leader@test", "业务负责人", child.ID)
	engineer := createWorkflowFixtureEmployee(t, db, "工程师", "child-engineer@test", "自定义工程师", child.ID)
	db.Model(&parent).Update("leader_id", leader.ID)
	db.Model(&child).Update("leader_id", head.ID)
	if err := managerAssignmentError(db, head, leader.ID); err != nil {
		t.Fatalf("configured parent leader not allowed: %v", err)
	}
	for _, pair := range []struct {
		employee models.Employee
		expected uint
	}{{head, leader.ID}, {engineer, head.ID}} {
		users, _, err := resolveScorerUsersByResponsibility(db, pair.employee, "DEPARTMENT_MANAGER")
		if err != nil || len(users) != 1 || users[0].ID != pair.expected {
			t.Fatalf("explicit leader scoring mismatch %v %+v", err, users)
		}
	}
	head.ManagerID = &leader.ID
	users, source, err := resolveHeadSupervisor(db, head)
	if err != nil || len(users) != 1 || users[0].ID != leader.ID || source != "DIRECT_MANAGER" {
		t.Fatalf("direct manager mismatch %v", err)
	}
	legacy := createWorkflowFixtureEmployee(t, db, "旧职责负责人", "legacy-leader@test", "负责人", parent.ID)
	createWorkflowFixtureResponsibility(t, db, "DEPARTMENT_MANAGER", "旧职责", "department", legacy.ID, leader.ID, &parent.ID, "")
	head.ManagerID = nil
	users, _, err = resolveHeadSupervisor(db, head)
	if err != nil || len(users) != 1 || users[0].ID != leader.ID {
		t.Fatal("explicit leader must override duplicate legacy duty")
	}
	db.Model(&leader).Update("is_active", false)
	if _, _, err = resolveHeadSupervisor(db, head); err == nil {
		t.Fatal("inactive configured leader fell back silently")
	}
	db.Model(&leader).Updates(map[string]any{"is_active": true, "department_id": child.ID})
	if _, _, err = resolveHeadSupervisor(db, head); err == nil {
		t.Fatal("moved configured leader accepted")
	}
	db.Model(&leader).Update("department_id", parent.ID)
	db.Model(&leader).Update("manager_id", head.ID)
	if _, _, err = resolveHeadSupervisor(db, head); err == nil {
		t.Fatal("automatic parent scorer accepted manager cycle")
	}
}

func TestExplicitLeaderCandidatesAndHistoricalSnapshot(t *testing.T) {
	db := leaderTestDB(t)
	parent := models.Department{Name: "父部"}
	db.Create(&parent)
	child := models.Department{Name: "子部", ParentID: &parent.ID}
	db.Create(&child)
	actor := createWorkflowFixtureEmployee(t, db, "管理员", "candidate-explicit@test", "管理", parent.ID)
	hierarchyScopeRole(t, db, actor.ID, "ALL")
	leader := createWorkflowFixtureEmployee(t, db, "父负责人", "explicit-parent@test", "岗位", parent.ID)
	head := createWorkflowFixtureEmployee(t, db, "子负责人", "explicit-child@test", "岗位", child.ID)
	db.Model(&parent).Update("leader_id", leader.ID)
	db.Model(&child).Update("leader_id", head.ID)
	evaluation := createWorkflowFixtureEvaluation(t, db, head.ID, 1, "pending", "")
	snapshot := `{"scorers":{"DEPARTMENT_MANAGER":999},"items":[]}`
	db.Model(&evaluation).Update("template_snapshot_json", snapshot)
	r := gin.New()
	r.GET("/departments/:id/candidates", func(c *gin.Context) { c.Set("user_id", actor.ID) }, GetDepartmentManagerCandidates)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", fmt.Sprintf("/departments/%d/candidates?position=岗位&employee_id=%d", child.ID, head.ID), nil))
	var response struct {
		Data []models.Employee `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(response.Data) != 1 || response.Data[0].ID != leader.ID {
		t.Fatalf("parent candidate missing: %s", w.Body.String())
	}
	// Reload stored snapshot, not the transient pre-update struct.
	db.First(&evaluation, evaluation.ID)
	scorers, _, err := snapshotScorers(db, evaluation)
	if err != nil || scorers["DEPARTMENT_MANAGER"] != 999 {
		t.Fatal("history re-resolved")
	}
}

func TestExplicitLeaderPreviewCreateAndTaskSnapshot(t *testing.T) {
	t.Setenv("EVALUATION_WORKFLOW_MODE", "enabled")
	db := leaderTestDB(t)
	parent := models.Department{Name: "业务父部门"}
	db.Create(&parent)
	child := models.Department{Name: "业务子部门", ParentID: &parent.ID}
	db.Create(&child)
	leader := createWorkflowFixtureEmployee(t, db, "实际父负责人", "snapshot-leader@test", "任意岗位", parent.ID)
	head := createWorkflowFixtureEmployee(t, db, "实际子负责人", "snapshot-head@test", "任意负责人岗位", child.ID)
	db.Model(&parent).Update("leader_id", leader.ID)
	db.Model(&child).Update("leader_id", head.ID)
	hierarchyScopeRole(t, db, leader.ID, "ALL")
	template := models.KPITemplate{Name: "自定义岗位负责人模板", DepartmentID: &child.ID, PositionType: head.Position, Period: "monthly", Status: "published", IsActive: true, Version: 1, CalculationVersion: canonicalCalculationVersion}
	db.Create(&template)
	dimension := models.KPIDimension{TemplateID: template.ID, Name: "业绩", Weight: 100}
	db.Create(&dimension)
	item := models.KPIItem{TemplateID: template.ID, DimensionID: &dimension.ID, Name: "交付", MaxScore: 100, ItemWeight: 100, CompositeWeight: 100, Required: true, ScorerTypesJSON: `["DEPARTMENT_MANAGER"]`}
	db.Create(&item)
	r := gin.New()
	r.GET("/preview", func(c *gin.Context) { c.Set("user_id", leader.ID) }, PreviewEvaluationTemplate)
	r.POST("/evaluations", func(c *gin.Context) { c.Set("user_id", leader.ID) }, CreateEvaluation)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", fmt.Sprintf("/preview?employee_id=%d", head.ID), nil))
	var response struct {
		Data templateMatchPreview `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &response)
	if w.Code != 200 || !response.Data.Ready || response.Data.Scorers["DEPARTMENT_MANAGER"].ID != leader.ID {
		t.Fatalf("preview %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/evaluations", strings.NewReader(fmt.Sprintf(`{"employee_id":%d,"template_id":%d,"period":"monthly","year":2026,"month":10}`, head.ID, template.ID))))
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var evaluation models.KPIEvaluation
	db.Where("employee_id = ?", head.ID).First(&evaluation)
	var tasks []models.EvaluationScoringTask
	db.Where("evaluation_id = ?", evaluation.ID).Find(&tasks)
	if len(tasks) != 1 || tasks[0].ScorerUserID != leader.ID {
		t.Fatalf("task mismatch %+v", tasks)
	}
	snapshot, err := parseEvaluationSnapshot(evaluation.TemplateSnapshotJSON)
	if err != nil || len(snapshot.ResolvedScorers) != 1 || snapshot.ResolvedScorers[0].UserID != leader.ID {
		t.Fatalf("snapshot mismatch %v %+v", err, snapshot)
	}
	db.Model(&parent).Update("leader_id", nil)
	var stored models.KPIEvaluation
	db.First(&stored, evaluation.ID)
	if stored.TemplateSnapshotJSON != evaluation.TemplateSnapshotJSON {
		t.Fatal("organization change rewrote snapshot")
	}
	db.First(&tasks[0], tasks[0].ID)
	if tasks[0].ScorerUserID != leader.ID {
		t.Fatal("organization change reassigned historical task")
	}
}

func TestDepartmentLeaderWritePermissionAndInitializationPreservation(t *testing.T) {
	db := leaderTestDB(t)
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	var department models.Department
	db.Where("name = ?", "机器人事业研发部").First(&department)
	leader := createWorkflowFixtureEmployee(t, db, "负责人", "permission-leader@test", "业务岗位", department.ID)
	hierarchyScopeRole(t, db, leader.ID, "ALL")
	r := gin.New()
	r.PUT("/departments/:id", func(c *gin.Context) { c.Set("user_id", leader.ID) }, PermissionMiddleware("department:edit"), UpdateDepartment)
	request := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("PUT", fmt.Sprintf("/departments/%d", department.ID), strings.NewReader(fmt.Sprintf(`{"leader_id":%d}`, leader.ID))))
		return w
	}
	if w := request(); w.Code != 403 {
		t.Fatal("missing permission allowed")
	}
	permission := models.Permission{Code: "department:edit", Name: "部门编辑"}
	db.Create(&permission)
	var role models.Role
	db.Where("code = ?", "scope-ALL").First(&role)
	db.Create(&models.RolePermission{RoleID: role.ID, PermissionID: permission.ID})
	if w := request(); w.Code != 200 {
		t.Fatalf("allowed write %d %s", w.Code, w.Body.String())
	}
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	db.First(&department, department.ID)
	if department.LeaderID == nil || *department.LeaderID != leader.ID {
		t.Fatal("initialization overwrote HR leader")
	}
	var userRoleCount int64
	db.Model(&models.UserRole{}).Where("user_id = ?", leader.ID).Count(&userRoleCount)
	if userRoleCount != 1 {
		t.Fatal("leader assignment changed RBAC")
	}
}
