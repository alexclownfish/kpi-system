package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// The PostgreSQL path is opt-in and restricted to a disposable test database.
// Each test owns a separate schema; daily and production databases are unused.
func crossDepartmentLeaderTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	if os.Getenv("KPI_TEST_DISPOSABLE_POSTGRES") != "1" {
		return leaderTestDB(t)
	}
	dsn := os.Getenv("KPI_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Fatal("disposable PostgreSQL DSN is missing")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal("open disposable PostgreSQL")
	}
	schema := fmt.Sprintf("leader_test_%d", time.Now().UnixNano())
	if err := admin.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	old := models.DB
	t.Cleanup(func() {
		models.DB = old
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
		_ = admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`).Error
		adminSQL, _ := admin.DB()
		_ = adminSQL.Close()
	})
	migrateWorkflowFixtureDB(t, db)
	if err := db.AutoMigrate(&models.JobPosition{}, &models.BusinessInitialization{}); err != nil {
		t.Fatal(err)
	}
	models.DB = db
	return db
}

func TestCrossDepartmentLeaderBaselineScenario(t *testing.T) {
	db := crossDepartmentLeaderTestDB(t)
	hq := models.Department{Name: "总部"}
	db.Create(&hq)
	child := models.Department{Name: "研发"}
	db.Create(&child)
	cto := createWorkflowFixtureEmployee(t, db, "CTO", "cross-cto@test", "CTO", hq.ID)
	hierarchyScopeRole(t, db, cto.ID, "ALL")
	r := gin.New()
	r.PUT("/departments/:id", func(c *gin.Context) { c.Set("user_id", cto.ID) }, UpdateDepartment)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("PUT", fmt.Sprintf("/departments/%d", child.ID), strings.NewReader(fmt.Sprintf(`{"leader_id":%d}`, cto.ID))))
	if w.Code != 200 {
		t.Fatalf("cross-department appointment rejected: %d %s", w.Code, w.Body.String())
	}
	employee := createWorkflowFixtureEmployee(t, db, "工程师", "cross-engineer@test", "工程师", child.ID)
	if err := managerAssignmentError(db, employee, cto.ID); err != nil {
		t.Fatalf("appointed external manager rejected: %v", err)
	}
	users, _, err := resolveScorerUsersByResponsibility(db, employee, "DEPARTMENT_MANAGER")
	if err != nil || len(users) != 1 || users[0].ID != cto.ID {
		t.Fatalf("scorer mismatch %v %+v", err, users)
	}
}

func TestCrossDepartmentLeaderCreateReplaceAndPermissions(t *testing.T) {
	db := crossDepartmentLeaderTestDB(t)
	hq := models.Department{Name: "总部"}
	db.Create(&hq)
	actor := createWorkflowFixtureEmployee(t, db, "管理员", "cross-admin@test", "管理员", hq.ID)
	hierarchyScopeRole(t, db, actor.ID, "ALL")
	cto := createWorkflowFixtureEmployee(t, db, "CTO", "cross-cto-create@test", "CTO", hq.ID)
	replacement := createWorkflowFixtureEmployee(t, db, "新主管", "cross-next@test", "CTO", hq.ID)
	r := gin.New()
	current := actor.ID
	r.Use(func(c *gin.Context) { c.Set("user_id", current) })
	r.POST("/departments", CreateDepartment)
	r.PUT("/departments/:id", UpdateDepartment)
	r.GET("/departments", GetDepartments)
	r.GET("/candidates", PermissionAnyMiddleware("department:create", "department:edit"), GetDepartmentLeaderCandidates)
	request := func(method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w
	}
	var departments []models.Department
	for _, name := range []string{"运控", "结构"} {
		w := request("POST", "/departments", fmt.Sprintf(`{"name":%q,"leader_id":%d}`, name, cto.ID), 201)
		var response struct {
			Data models.Department `json:"data"`
		}
		json.Unmarshal(w.Body.Bytes(), &response)
		if response.Data.LeaderID == nil || *response.Data.LeaderID != cto.ID || response.Data.Leader == nil || response.Data.Leader.Name != "CTO" {
			t.Fatalf("create response lacks leader: %s", w.Body.String())
		}
		departments = append(departments, response.Data)
	}
	employee := createWorkflowFixtureEmployee(t, db, "员工", "cross-handoff@test", "工程师", departments[0].ID)
	db.Model(&employee).Update("manager_id", cto.ID)
	w := request("PUT", fmt.Sprintf("/departments/%d", departments[0].ID), fmt.Sprintf(`{"leader_id":%d}`, replacement.ID), 200)
	var result struct {
		Reports int `json:"previous_leader_direct_reports"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Reports != 1 {
		t.Fatalf("handoff count %s", w.Body.String())
	}
	db.First(&employee, employee.ID)
	if employee.ManagerID == nil || *employee.ManagerID != cto.ID {
		t.Fatal("handoff silently rewrote manager")
	}
	var stored models.Employee
	db.First(&stored, cto.ID)
	if stored.DepartmentID != hq.ID || stored.Position != "CTO" || stored.Role != cto.Role {
		t.Fatal("appointment changed owner identity")
	}
	var audit models.AuditLog
	db.Where("action = ?", "update_department").Last(&audit)
	if !strings.Contains(audit.Details, fmt.Sprintf("old_leader_id=%d", cto.ID)) {
		t.Fatalf("handoff audit missing %+v", audit)
	}
	request("PUT", fmt.Sprintf("/departments/%d", departments[1].ID), `{"leader_id":99999}`, 400)
	db.Model(&replacement).Update("is_active", false)
	request("PUT", fmt.Sprintf("/departments/%d", departments[1].ID), fmt.Sprintf(`{"leader_id":%d}`, replacement.ID), 400)
	request("GET", "/candidates", "", 403)
	permission := models.Permission{Code: "department:edit", Name: "编辑部门"}
	db.Create(&permission)
	var role models.Role
	db.Where("code = ?", "scope-ALL").First(&role)
	db.Create(&models.RolePermission{RoleID: role.ID, PermissionID: permission.ID})
	w = request("GET", "/candidates?pageSize=1&page=1", "", 200)
	var candidates struct {
		Data  []models.Employee `json:"data"`
		Total int               `json:"total"`
		Pages int               `json:"totalPages"`
	}
	json.Unmarshal(w.Body.Bytes(), &candidates)
	if len(candidates.Data) != 1 || candidates.Pages < 2 {
		t.Fatalf("pagination %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"password"`) {
		t.Fatal("credential leaked")
	}
	limited := createWorkflowFixtureEmployee(t, db, "受限", "cross-limited@test", "员工", hq.ID)
	hierarchyScopeRole(t, db, limited.ID, "DEPARTMENT")
	var limitedRole models.Role
	db.Where("code = ?", "scope-DEPARTMENT").First(&limitedRole)
	db.Create(&models.RolePermission{RoleID: limitedRole.ID, PermissionID: permission.ID})
	current = limited.ID
	// The chosen CTO is visible but the target department is not: no takeover.
	request("PUT", fmt.Sprintf("/departments/%d", departments[1].ID), fmt.Sprintf(`{"leader_id":%d}`, cto.ID), 403)
	// The department is visible, but an employee outside the candidate scope
	// must not be appointed by guessing an ID.
	request("PUT", fmt.Sprintf("/departments/%d", hq.ID), fmt.Sprintf(`{"leader_id":%d}`, employee.ID), 403)
	w = request("GET", "/candidates?pageSize=100&search=cross-handoff", "", 200)
	if err := json.Unmarshal(w.Body.Bytes(), &candidates); err != nil {
		t.Fatal(err)
	}
	if len(candidates.Data) != 0 {
		t.Fatal("out-of-scope leader candidate leaked")
	}
	w = request("GET", "/departments?pageSize=100", "", 200)
	var visibleDepartments struct {
		Data []models.Department `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &visibleDepartments); err != nil {
		t.Fatal(err)
	}
	for _, department := range visibleDepartments.Data {
		if department.ID != hq.ID && department.Leader != nil {
			t.Fatal("unrelated department leader summary leaked")
		}
	}
	current = actor.ID
	w = request("GET", "/departments?pageSize=100", "", 200)
	if err := json.Unmarshal(w.Body.Bytes(), &visibleDepartments); err != nil {
		t.Fatal(err)
	}
	for _, department := range visibleDepartments.Data {
		if department.ID == departments[1].ID && (department.Leader == nil || department.Leader.ID != cto.ID) {
			t.Fatal("cross-department leader missing from department list")
		}
	}
	// Clearing also reports the existing direct report and preserves the link.
	db.Model(&employee).Update("manager_id", cto.ID)
	w = request("PUT", fmt.Sprintf("/departments/%d", departments[0].ID), `{"leader_id":null}`, 200)
	if !strings.Contains(w.Body.String(), `"previous_leader_direct_reports":0`) {
		t.Fatal("handoff counted a report belonging to a different manager")
	}
}

func TestCrossDepartmentLeaderScopesAndRemoval(t *testing.T) {
	for _, scope := range []string{"DEPARTMENT", "DEPARTMENT_TREE", "SELF", "DIRECT_SUBORDINATES", "ASSIGNED"} {
		t.Run(scope, func(t *testing.T) {
			db := crossDepartmentLeaderTestDB(t)
			hq := models.Department{Name: "总部"}
			db.Create(&hq)
			a := models.Department{Name: "A"}
			db.Create(&a)
			b := models.Department{Name: "B"}
			db.Create(&b)
			child := models.Department{Name: "A子", ParentID: &a.ID}
			db.Create(&child)
			other := models.Department{Name: "无关"}
			db.Create(&other)
			manager := createWorkflowFixtureEmployee(t, db, "兼任负责人", "cross-scope@test", "CTO", hq.ID)
			hierarchyScopeRole(t, db, manager.ID, scope)
			template := models.KPITemplate{Name: "范围测试", Period: "monthly"}
			if err := db.Create(&template).Error; err != nil {
				t.Fatal(err)
			}
			db.Model(&a).Update("leader_id", manager.ID)
			db.Model(&b).Update("leader_id", manager.ID)
			for _, row := range []struct {
				dept       uint
				department bool
				tree       bool
			}{{a.ID, true, true}, {b.ID, true, true}, {child.ID, false, true}, {other.ID, false, false}} {
				employee := createWorkflowFixtureEmployee(t, db, fmt.Sprint(row.dept), fmt.Sprintf("scope-%d@test", row.dept), "工程师", row.dept)
				evaluation := createWorkflowFixtureEvaluation(t, db, employee.ID, template.ID, "pending", "")
				want := scope == "DEPARTMENT" && row.department || scope == "DEPARTMENT_TREE" && row.tree
				if got := CanAccessEmployee(manager.ID, employee.ID); got != want {
					t.Fatalf("employee dept %d got %v want %v", row.dept, got, want)
				}
				if got := CanAccessEvaluation(manager.ID, evaluation.ID); got != want {
					t.Fatalf("evaluation dept %d got %v want %v", row.dept, got, want)
				}
				if want && !departmentWithinActorScope(manager.ID, row.dept) {
					t.Fatal("responsibility scope inconsistent")
				}
				responsibility := createWorkflowFixtureResponsibility(t, db, "PM", "范围职责", "department", employee.ID, manager.ID, &row.dept, "")
				var count int64
				if err := applyResponsibilityAdministrationScope(db.Model(&models.EvaluationResponsibility{}), manager.ID).Where("id = ?", responsibility.ID).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				if (count > 0) != want {
					t.Fatal("responsibility management scope differs from employee scope")
				}
			}
			db.Model(&a).Update("leader_id", nil)
			var employee models.Employee
			db.Where("department_id = ?", a.ID).First(&employee)
			if CanAccessEmployee(manager.ID, employee.ID) {
				t.Fatal("removed appointment still grants department access")
			}
		})
	}
}

func TestCrossDepartmentLeaderCandidatesScorersAndSafety(t *testing.T) {
	db := crossDepartmentLeaderTestDB(t)
	hq := models.Department{Name: "总部"}
	db.Create(&hq)
	dept := models.Department{Name: "研发"}
	db.Create(&dept)
	cto := createWorkflowFixtureEmployee(t, db, "CTO", "cross-scorer@test", "CTO", hq.ID)
	engineer := createWorkflowFixtureEmployee(t, db, "员工", "cross-target@test", "工程师", dept.ID)
	outsider := createWorkflowFixtureEmployee(t, db, "外部", "cross-outsider@test", "工程师", hq.ID)
	db.Model(&dept).Update("leader_id", cto.ID)
	if err := managerAssignmentError(db, engineer, outsider.ID); err == nil {
		t.Fatal("unappointed external manager accepted")
	}
	if err := managerAssignmentError(db, engineer, engineer.ID); err == nil {
		t.Fatal("self manager accepted")
	}
	db.Model(&cto).Update("manager_id", engineer.ID)
	if err := managerAssignmentError(db, engineer, cto.ID); err == nil {
		t.Fatal("cycle accepted")
	}
	db.Model(&cto).Update("manager_id", nil)
	hierarchyScopeRole(t, db, engineer.ID, "DEPARTMENT")
	r := gin.New()
	r.GET("/departments/:id/candidates", func(c *gin.Context) { c.Set("user_id", engineer.ID) }, GetDepartmentManagerCandidates)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", fmt.Sprintf("/departments/%d/candidates?employee_id=%d&position=工程师", dept.ID, engineer.ID), nil))
	var response struct {
		Data []models.Employee `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &response)
	found := false
	for _, candidate := range response.Data {
		if candidate.ID == outsider.ID {
			t.Fatal("outsider leaked")
		}
		if candidate.ID == cto.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("appointed external leader missing %s", w.Body.String())
	}
	createWorkflowFixtureResponsibility(t, db, "CTO", "公司CTO", "company", cto.ID, engineer.ID, nil, "")
	template := models.KPITemplate{ID: 1, Items: []models.KPIItem{{ID: 1, Name: "综合", Required: true, ScorerTypes: []string{"DEPARTMENT_MANAGER", "CTO"}}}}
	assignments, err := resolveTemplateScoringAssignments(db, engineer, template)
	if err != nil || len(assignments) != 2 {
		t.Fatalf("dual duties %+v %v", assignments, err)
	}
	for _, assignment := range assignments {
		if assignment.UserID != cto.ID {
			t.Fatal("incorrect scorer")
		}
	}
	selfTemplate := models.KPITemplate{Items: []models.KPIItem{{ID: 1, Name: "CTO指标", Required: true, ScorerTypes: []string{"CTO"}}}}
	if _, err = resolveTemplateScoringAssignments(db, cto, selfTemplate); err == nil {
		t.Fatal("CTO self-scoring allowed")
	}
	db.Model(&cto).Update("is_active", false)
	if _, _, err = resolveScorerUsersByResponsibility(db, engineer, "DEPARTMENT_MANAGER"); err == nil {
		t.Fatal("inactive explicit leader accepted")
	}
}

func TestCrossDepartmentLeaderEmployeeAndWorkflowLifecycle(t *testing.T) {
	t.Setenv("EVALUATION_WORKFLOW_MODE", "enabled")
	db := crossDepartmentLeaderTestDB(t)
	hq := models.Department{Name: "总部"}
	if err := db.Create(&hq).Error; err != nil {
		t.Fatal(err)
	}
	actor := createWorkflowFixtureEmployee(t, db, "管理员", "lifecycle-admin@test", "管理员", hq.ID)
	hierarchyScopeRole(t, db, actor.ID, "ALL")
	cto := createWorkflowFixtureEmployee(t, db, "CTO", "lifecycle-cto@test", "CTO", hq.ID)
	hierarchyScopeRole(t, db, cto.ID, "DEPARTMENT")
	next := createWorkflowFixtureEmployee(t, db, "接任人", "lifecycle-next@test", "主管", hq.ID)
	employeeRole := models.Role{Code: "employee", Name: "普通员工", RequiresManager: true}
	if err := db.Create(&employeeRole).Error; err != nil {
		t.Fatal(err)
	}
	self := models.DataScope{Code: "SELF", Name: "本人"}
	db.Create(&self)
	db.Create(&models.RoleDataScope{RoleID: employeeRole.ID, DataScopeID: self.ID})
	createWorkflowFixtureResponsibility(t, db, "CTO", "公司CTO", "company", cto.ID, actor.ID, nil, "")
	current := actor.ID
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", current) })
	router.POST("/departments", CreateDepartment)
	router.PUT("/departments/:id", UpdateDepartment)
	router.POST("/employees", CreateEmployee)
	router.POST("/evaluations", CreateEvaluation)
	router.POST("/evaluations/:id/work-summary", SubmitEvaluationWorkSummary)
	router.POST("/scores/:id/self", UpdateSelfScore)
	router.POST("/evaluations/:id/advance", AdvanceEvaluationWorkflow)
	router.POST("/tasks/:id/submit", SubmitScoringTask)
	router.POST("/evaluations/:id/confirm", ConfirmEvaluationOnline)
	request := func(path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s got %d want %d: %s", path, w.Code, want, w.Body.String())
		}
		return w
	}
	var firstDept models.Department
	var firstEvaluation models.KPIEvaluation
	var firstTasks []models.EvaluationScoringTask
	var firstEmployee models.Employee
	for index, name := range []string{"运控测试", "结构测试"} {
		w := request("/departments", fmt.Sprintf(`{"name":%q,"leader_id":%d}`, name, cto.ID), 201)
		var departmentResponse struct {
			Data models.Department `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &departmentResponse); err != nil {
			t.Fatal(err)
		}
		department := departmentResponse.Data
		w = request("/employees", fmt.Sprintf(`{"name":%q,"email":%q,"position":"工程师","department_id":%d,"manager_id":%d,"role":"employee","is_active":true,"password":"fixture-password-only"}`, name+"工程师", fmt.Sprintf("lifecycle-%d@test", index), department.ID, cto.ID), 201)
		var employeeResponse struct {
			Data models.Employee `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &employeeResponse); err != nil {
			t.Fatal(err)
		}
		employee := employeeResponse.Data
		if employee.ManagerID == nil || *employee.ManagerID != cto.ID || employee.DepartmentID != department.ID {
			t.Fatalf("wrong employee identity: %+v", employee)
		}
		template := models.KPITemplate{Name: name + "模板", DepartmentID: &department.ID, PositionType: "工程师", Period: "monthly", Status: "published", IsActive: true, Version: 1, CalculationVersion: canonicalCalculationVersion}
		if err := db.Create(&template).Error; err != nil {
			t.Fatal(err)
		}
		dimension := models.KPIDimension{TemplateID: template.ID, Name: "绩效", Weight: 100}
		if err := db.Create(&dimension).Error; err != nil {
			t.Fatal(err)
		}
		item := models.KPIItem{TemplateID: template.ID, DimensionID: &dimension.ID, Name: "交付", MaxScore: 100, ItemWeight: 100, CompositeWeight: 100, Required: true, ScorerTypesJSON: `["DEPARTMENT_MANAGER","CTO"]`}
		if err := db.Create(&item).Error; err != nil {
			t.Fatal(err)
		}
		request("/evaluations", fmt.Sprintf(`{"employee_id":%d,"template_id":%d,"period":"monthly","year":2026,"month":10}`, employee.ID, template.ID), 201)
		var evaluation models.KPIEvaluation
		if err := db.Where("employee_id = ?", employee.ID).First(&evaluation).Error; err != nil {
			t.Fatal(err)
		}
		snapshot, err := parseEvaluationSnapshot(evaluation.TemplateSnapshotJSON)
		if err != nil {
			t.Fatal(err)
		}
		tasks, err := generateEvaluationScoringTasks(db, evaluation, snapshot)
		if err != nil || len(tasks) != 0 {
			t.Fatalf("factory retry duplicated tasks: %+v %v", tasks, err)
		}
		if err := db.Where("evaluation_id = ?", evaluation.ID).Order("id").Find(&tasks).Error; err != nil {
			t.Fatal(err)
		}
		if len(tasks) != 2 || tasks[0].ScorerType == tasks[1].ScorerType {
			t.Fatalf("duties merged: %+v", tasks)
		}
		for _, task := range tasks {
			if task.ScorerUserID != cto.ID {
				t.Fatal("wrong appointed scorer")
			}
		}
		current = employee.ID
		request(fmt.Sprintf("/evaluations/%d/work-summary", evaluation.ID), `{"work_summary":"完成交付","work_risks":"暂无","support_needed":"暂无","next_plan":"下一阶段计划"}`, 200)
		var selfScore models.KPIScore
		if err := db.Where("evaluation_id = ? AND item_id = ?", evaluation.ID, item.ID).First(&selfScore).Error; err != nil {
			t.Fatal(err)
		}
		request(fmt.Sprintf("/scores/%d/self", selfScore.ID), `{"self_score":90}`, 200)
		request(fmt.Sprintf("/evaluations/%d/advance", evaluation.ID), `{"expected_state":"self_evaluating"}`, 200)
		current = cto.ID
		request(fmt.Sprintf("/tasks/%d/submit", tasks[0].ID), `{"score":80,"version":1}`, 200)
		request(fmt.Sprintf("/evaluations/%d/advance", evaluation.ID), `{"expected_state":"scoring"}`, 409)
		request(fmt.Sprintf("/tasks/%d/submit", tasks[1].ID), `{"score":100,"version":1}`, 200)
		request(fmt.Sprintf("/evaluations/%d/advance", evaluation.ID), `{"expected_state":"scoring"}`, 200)
		current = employee.ID
		request(fmt.Sprintf("/evaluations/%d/confirm", evaluation.ID), `{}`, 200)
		if err := db.First(&evaluation, evaluation.ID).Error; err != nil {
			t.Fatal(err)
		}
		if evaluation.Status != "completed" || evaluation.TotalScore != 90 {
			t.Fatalf("wrong lifecycle result: %+v", evaluation)
		}
		if index == 0 {
			firstDept = department
			firstEvaluation = evaluation
			firstEmployee = employee
			firstTasks = tasks
		}
		current = actor.ID
	}
	// Replacing the leader changes only future resolution, preserving employee
	// relationships, old task ownership, grades and both immutable snapshots.
	var resultSnapshot models.EvaluationResultSnapshot
	if err := db.Where("evaluation_id = ?", firstEvaluation.ID).First(&resultSnapshot).Error; err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", fmt.Sprintf("/departments/%d", firstDept.ID), strings.NewReader(fmt.Sprintf(`{"leader_id":%d}`, next.ID)))
	router.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"previous_leader_direct_reports":1`) {
		t.Fatalf("handoff: %d %s", w.Code, w.Body.String())
	}
	if CanAccessEmployee(cto.ID, firstEmployee.ID) {
		t.Fatal("removed department appointment kept employee scope")
	}
	if !CanAccessEvaluation(cto.ID, firstEvaluation.ID) {
		t.Fatal("historical assigned task access was lost")
	}
	var stored models.KPIEvaluation
	db.First(&stored, firstEvaluation.ID)
	if stored.TemplateSnapshotJSON != firstEvaluation.TemplateSnapshotJSON || stored.TotalScore != firstEvaluation.TotalScore || stored.Status != firstEvaluation.Status {
		t.Fatal("historical evaluation changed")
	}
	var storedResult models.EvaluationResultSnapshot
	db.First(&storedResult, resultSnapshot.ID)
	if storedResult.Checksum != resultSnapshot.Checksum || storedResult.SnapshotJSON != resultSnapshot.SnapshotJSON {
		t.Fatal("signed result snapshot changed")
	}
	db.First(&firstEmployee, firstEmployee.ID)
	if firstEmployee.ManagerID == nil || *firstEmployee.ManagerID != cto.ID {
		t.Fatal("handoff rewrote employee manager")
	}
	for _, oldTask := range firstTasks {
		var storedTask models.EvaluationScoringTask
		db.First(&storedTask, oldTask.ID)
		if storedTask.ScorerUserID != cto.ID || storedTask.ScorerType != oldTask.ScorerType {
			t.Fatal("old task reassigned")
		}
	}
	var template models.KPITemplate
	if err := db.Preload("Items").First(&template, firstEvaluation.TemplateID).Error; err != nil {
		t.Fatal(err)
	}
	assignments, err := resolveTemplateScoringAssignments(db, firstEmployee, template)
	if err != nil {
		t.Fatal(err)
	}
	for _, assignment := range assignments {
		expected := cto.ID
		if assignment.ScorerType == "DEPARTMENT_MANAGER" {
			expected = next.ID
		}
		if assignment.UserID != expected {
			t.Fatalf("new assignment wrong: %+v", assignment)
		}
	}
	request("/evaluations", fmt.Sprintf(`{"employee_id":%d,"template_id":%d,"period":"monthly","year":2026,"month":11}`, firstEmployee.ID, template.ID), 201)
	var newEvaluation models.KPIEvaluation
	if err := db.Where("employee_id = ? AND month = ?", firstEmployee.ID, 11).First(&newEvaluation).Error; err != nil {
		t.Fatal(err)
	}
	var newTasks []models.EvaluationScoringTask
	if err := db.Where("evaluation_id = ?", newEvaluation.ID).Find(&newTasks).Error; err != nil {
		t.Fatal(err)
	}
	if len(newTasks) != 2 {
		t.Fatalf("new assessment did not generate both duties: %+v", newTasks)
	}
	for _, task := range newTasks {
		expected := cto.ID
		if task.ScorerType == "DEPARTMENT_MANAGER" {
			expected = next.ID
		}
		if task.ScorerUserID != expected {
			t.Fatalf("new assessment used stale leader: %+v", task)
		}
	}
}

func TestCrossDepartmentLeaderStartupRepairPreservesExternalManager(t *testing.T) {
	db := crossDepartmentLeaderTestDB(t)
	parent := models.Department{Name: "机器人事业研发部"}
	db.Create(&parent)
	hq := models.Department{Name: "总部", ParentID: &parent.ID}
	db.Create(&hq)
	child := models.Department{Name: "运控部", ParentID: &parent.ID}
	db.Create(&child)
	boss := createWorkflowFixtureEmployee(t, db, "上级负责人", "repair-boss@test", "管理", parent.ID)
	cto := createWorkflowFixtureEmployee(t, db, "兼任CTO", "repair-cto@test", "CTO", hq.ID)
	db.Model(&parent).Update("leader_id", boss.ID)
	db.Model(&hq).Update("leader_id", cto.ID)
	db.Model(&child).Update("leader_id", cto.ID)
	if err := repairDepartmentLeaderManagers(db); err != nil {
		t.Fatal(err)
	}
	var stored models.Employee
	if err := db.First(&stored, cto.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ManagerID != nil || stored.DepartmentID != hq.ID {
		t.Fatal("startup repair rewrote cross-department leader identity")
	}
}
