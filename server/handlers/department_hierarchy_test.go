package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestDepartmentPositionPrefixMatchesPublishedTemplate(t *testing.T) {
	db := catalogTestDB(t)
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	var dept models.Department
	db.Where("name = ?", "软件应用开发部").First(&dept)
	for _, name := range []string{"软件应用开发工程师", "软件应用开发部工程师", "工程师"} {
		employee := models.Employee{Name: "工程师", DepartmentID: dept.ID, Position: name}
		template, err := findPublishedTemplate(db, employee, "monthly")
		if err != nil || template.PositionType != "工程师" {
			t.Fatalf("%s: template=%s err=%v", name, template.PositionType, err)
		}
	}
	for _, name := range []string{"软件应用开发部负责人", "软件应用开发部门负责人", "部门负责人"} {
		template, err := findPublishedTemplate(db, models.Employee{Name: "负责人", DepartmentID: dept.ID, Position: name}, "monthly")
		if err != nil || template.PositionType != "部门负责人" {
			t.Fatalf("valid source weights should match published manager template: %s template=%s err=%v", name, template.PositionType, err)
		}
	}
	if _, err := findPublishedTemplate(db, models.Employee{Name: "其它岗位", DepartmentID: dept.ID, Position: "高级开发工程师"}, "monthly"); err == nil {
		t.Fatal("ambiguous position matched")
	}
}

func TestProductManagerMapsToProductDepartmentHeadTemplate(t *testing.T) {
	db := catalogTestDB(t)
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	var productDepartment, otherDepartment models.Department
	if err := db.Where("name = ?", "产品部门").First(&productDepartment).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("name = ?", "软件应用开发部").First(&otherDepartment).Error; err != nil {
		t.Fatal(err)
	}
	productManager := models.Employee{Name: "产品经理", DepartmentID: productDepartment.ID, Position: "产品经理"}
	template, err := findPublishedTemplate(db, productManager, "monthly")
	if err != nil || template.PositionType != "部门负责人" {
		t.Fatalf("product manager mapping failed: template=%+v err=%v", template, err)
	}
	otherProductManager := models.Employee{Name: "其他部门产品经理", DepartmentID: otherDepartment.ID, Position: "产品经理"}
	if _, err := findPublishedTemplate(db, otherProductManager, "monthly"); err == nil {
		t.Fatal("product manager alias leaked outside product department")
	}
}

func TestHongxingHierarchyUpgradeAndHRPreservation(t *testing.T) {
	db := catalogTestDB(t)
	var catalog hongxingCatalog
	json.Unmarshal(hongxingCatalogJSON, &catalog)
	if err := seedHongxingCatalog(db, catalog); err != nil {
		t.Fatal(err)
	}
	if err := seedHongxingHierarchy(db); err != nil {
		t.Fatal(err)
	}
	var parent models.Department
	db.Where("name = ?", "机器人事业研发部").First(&parent)
	var children []models.Department
	db.Where("parent_id = ?", parent.ID).Find(&children)
	if parent.ID == 0 || len(children) != 4 {
		t.Fatalf("parent=%d children=%d", parent.ID, len(children))
	}
	child := children[0]
	db.Model(&child).Update("parent_id", nil)
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	db.First(&child, child.ID)
	if child.ParentID != nil {
		t.Fatal("HR root change overwritten")
	}
	catalogCount(t, db, &models.Department{}, 10)
	catalogCount(t, db, &models.KPITemplate{}, 18)
	catalogCount(t, db, &models.Employee{}, 0)
	catalogCount(t, db, &models.BusinessInitialization{}, 2)
}

func TestHongxingHierarchyConflictRollsBack(t *testing.T) {
	db := catalogTestDB(t)
	other := models.Department{Name: "其它上级"}
	db.Create(&other)
	child := models.Department{Name: "软件应用开发部", ParentID: &other.ID}
	db.Create(&child)
	if err := seedHongxingHierarchy(db); err == nil {
		t.Fatal("conflict accepted")
	}
	catalogCount(t, db, &models.Department{}, 2)
	catalogCount(t, db, &models.BusinessInitialization{}, 0)
}

func TestRepairDepartmentLeaderManagersBackfillsOnlyMissingValues(t *testing.T) {
	db := catalogTestDB(t)
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	var parent, child models.Department
	if err := db.Where("name = ?", "机器人事业研发部").First(&parent).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("name = ?", "软件应用开发部").First(&child).Error; err != nil {
		t.Fatal(err)
	}
	parentLeader := models.Employee{Name: "研发部负责人", Email: "repair-parent@test", Password: "hash", Position: "部门负责人", DepartmentID: parent.ID, IsActive: true}
	childLeader := models.Employee{Name: "软件负责人", Email: "repair-child@test", Password: "hash", Position: "部门负责人", DepartmentID: child.ID, IsActive: true}
	for _, employee := range []*models.Employee{&parentLeader, &childLeader} {
		if err := db.Create(employee).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&parent).Update("leader_id", parentLeader.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&child).Update("leader_id", childLeader.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := repairDepartmentLeaderManagers(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&childLeader, childLeader.ID).Error; err != nil {
		t.Fatal(err)
	}
	if childLeader.ManagerID == nil || *childLeader.ManagerID != parentLeader.ID {
		t.Fatalf("missing manager was not backfilled: %+v", childLeader)
	}

	cto := models.Employee{Name: "集团CTO", Email: "repair-cto@test", Password: "hash", Position: "CTO", DepartmentID: parent.ID, IsActive: true}
	if err := db.Create(&cto).Error; err != nil {
		t.Fatal(err)
	}
	createWorkflowFixtureResponsibility(t, db, "CTO", "集团CTO", "company", cto.ID, childLeader.ID, nil, "")
	if err := db.Model(&childLeader).Update("manager_id", cto.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := repairDepartmentLeaderManagers(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&childLeader, childLeader.ID).Error; err != nil {
		t.Fatal(err)
	}
	if childLeader.ManagerID == nil || *childLeader.ManagerID != cto.ID {
		t.Fatalf("HR manager assignment was overwritten: %+v", childLeader)
	}
}

func TestDepartmentHierarchyMaintenanceValidation(t *testing.T) {
	db := catalogTestDB(t)
	old := models.DB
	models.DB = db
	t.Cleanup(func() { models.DB = old })
	parent := models.Department{Name: "父部门"}
	db.Create(&parent)
	child := models.Department{Name: "子部门", ParentID: &parent.ID}
	db.Create(&child)
	actor := createWorkflowFixtureEmployee(t, db, "管理员", "hierarchy-validation@test", "管理", child.ID)
	hierarchyScopeRole(t, db, actor.ID, "ALL")
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", actor.ID) })
	r.PUT("/departments/:id", UpdateDepartment)
	r.DELETE("/departments/:id", DeleteDepartment)
	request := func(method string, id uint, body string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, fmt.Sprintf("/departments/%d", id), strings.NewReader(body)))
		if w.Code != want {
			t.Fatalf("%s: got %d want %d %s", body, w.Code, want, w.Body.String())
		}
	}
	request("PUT", parent.ID, fmt.Sprintf(`{"parent_id":%d}`, child.ID), 400)
	request("PUT", child.ID, fmt.Sprintf(`{"parent_id":%d}`, child.ID), 400)
	request("PUT", child.ID, `{"parent_id":999}`, 400)
	request("DELETE", parent.ID, "", 400)
	request("PUT", child.ID, `{"description":"changed"}`, 200)
	var unchanged models.Department
	db.First(&unchanged, child.ID)
	if unchanged.ParentID == nil || *unchanged.ParentID != parent.ID {
		t.Fatal("omitted parent reset")
	}
	request("PUT", child.ID, `{"parent_id":null}`, 200)
	var root models.Department
	db.First(&root, child.ID)
	if root.ParentID != nil {
		t.Fatal("null parent not cleared")
	}
	request("DELETE", parent.ID, "", 200)
}

func TestHeadSupervisorPriorityAndSafety(t *testing.T) {
	db := catalogTestDB(t)
	parent := models.Department{Name: "父部门"}
	db.Create(&parent)
	child := models.Department{Name: "子部门", ParentID: &parent.ID}
	db.Create(&child)
	other := models.Department{Name: "无关部门"}
	db.Create(&other)
	head := createWorkflowFixtureEmployee(t, db, "子部门负责人", "child@test", "部门负责人", child.ID)
	parentHead := createWorkflowFixtureEmployee(t, db, "上级负责人", "parent@test", "部门负责人", parent.ID)
	cto := createWorkflowFixtureEmployee(t, db, "CTO", "cto@test", "CTO", other.ID)
	outsider := createWorkflowFixtureEmployee(t, db, "无关负责人", "other@test", "部门负责人", other.ID)
	createWorkflowFixtureResponsibility(t, db, "DEPARTMENT_MANAGER", "本部门", "department", head.ID, head.ID, &child.ID, "")
	parentDuty := createWorkflowFixtureResponsibility(t, db, "DEPARTMENT_MANAGER", "上级", "department", parentHead.ID, head.ID, &parent.ID, "")
	createWorkflowFixtureResponsibility(t, db, "CTO", "公司CTO", "company", cto.ID, head.ID, nil, "")
	createWorkflowFixtureResponsibility(t, db, "DEPARTMENT_MANAGER", "无关", "department", outsider.ID, head.ID, &other.ID, "")
	assertScorer := func(want uint, source string) {
		t.Helper()
		users, rule, err := resolveHeadSupervisor(db, head)
		if err != nil || len(users) != 1 || users[0].ID != want || rule != source {
			t.Fatalf("users=%v source=%s err=%v", users, rule, err)
		}
	}
	assertScorer(parentHead.ID, "PARENT_DEPARTMENT_MANAGER")
	head.ManagerID = &cto.ID
	assertScorer(cto.ID, "DIRECT_MANAGER")
	head.ManagerID = &outsider.ID
	if _, _, err := resolveHeadSupervisor(db, head); err == nil {
		t.Fatal("unrelated direct manager accepted")
	}
	head.ManagerID = &head.ID
	if _, _, err := resolveHeadSupervisor(db, head); err == nil {
		t.Fatal("self direct manager accepted")
	}
	head.ManagerID = nil
	expired := time.Now().Add(-time.Hour)
	db.Model(&parentDuty).Update("effective_to", expired)
	assertScorer(cto.ID, "COMPANY_CTO")
	db.Model(&cto).Update("is_active", false)
	if _, _, err := resolveHeadSupervisor(db, head); err == nil {
		t.Fatal("inactive CTO accepted")
	}
	db.Model(&parentDuty).Update("effective_to", nil)
	second := createWorkflowFixtureEmployee(t, db, "第二负责人", "second@test", "部门负责人", parent.ID)
	createWorkflowFixtureResponsibility(t, db, "DEPARTMENT_MANAGER", "另一负责人", "department", second.ID, head.ID, &parent.ID, "")
	if _, _, err := resolveHeadSupervisor(db, head); err == nil {
		t.Fatal("ambiguous parent accepted")
	}
	head.ManagerID = &parentHead.ID
	assertScorer(parentHead.ID, "DIRECT_MANAGER")
	db.Model(&parentHead).Update("manager_id", head.ID)
	if err := managerAssignmentError(db, head, parentHead.ID); err == nil {
		t.Fatal("manager cycle accepted")
	}
}

func hierarchyScopeRole(t *testing.T, db *gorm.DB, userID uint, code string) {
	t.Helper()
	role := models.Role{Code: "scope-" + code, Name: code}
	db.Create(&role)
	scope := models.DataScope{Code: code, Name: code}
	db.Where("code = ?", code).FirstOrCreate(&scope)
	db.Create(&models.RoleDataScope{RoleID: role.ID, DataScopeID: scope.ID})
	db.Create(&models.UserRole{UserID: userID, RoleID: role.ID})
}

func TestPartialSupervisorUpdatePreservesEmployeeFields(t *testing.T) {
	db := catalogTestDB(t)
	old := models.DB
	models.DB = db
	t.Cleanup(func() { models.DB = old })
	dept := models.Department{Name: "子部门"}
	db.Create(&dept)
	company := models.Department{Name: "总部"}
	db.Create(&company)
	head := createWorkflowFixtureEmployee(t, db, "负责人", "partial-head@test", "部门负责人", dept.ID)
	db.Model(&head).Update("role", "manager")
	role := models.Role{Code: "department_manager", Name: "负责人"}
	db.Create(&role)
	cto := createWorkflowFixtureEmployee(t, db, "CTO", "partial-cto@test", "CTO", company.ID)
	createWorkflowFixtureResponsibility(t, db, "CTO", "CTO", "company", cto.ID, head.ID, nil, "")
	r := gin.New()
	r.PUT("/employees/:id", UpdateEmployee)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("PUT", fmt.Sprintf("/employees/%d", head.ID), strings.NewReader(fmt.Sprintf(`{"manager_id":%d}`, cto.ID))))
	if w.Code != 200 {
		t.Fatalf("update %d %s", w.Code, w.Body.String())
	}
	var updated models.Employee
	db.First(&updated, head.ID)
	if updated.Name != head.Name || updated.Email != head.Email || updated.Position != head.Position || !updated.IsActive || updated.ManagerID == nil || *updated.ManagerID != cto.ID || updated.Password != head.Password {
		t.Fatalf("fields corrupted: %+v", updated)
	}
}

func TestDepartmentTreeScopeAndPreviewBoundary(t *testing.T) {
	db := catalogTestDB(t)
	old := models.DB
	models.DB = db
	t.Cleanup(func() { models.DB = old })
	parent := models.Department{Name: "父部门"}
	db.Create(&parent)
	child := models.Department{Name: "子部门", ParentID: &parent.ID}
	db.Create(&child)
	grandchild := models.Department{Name: "孙部门", ParentID: &child.ID}
	db.Create(&grandchild)
	other := models.Department{Name: "其他"}
	db.Create(&other)
	actor := createWorkflowFixtureEmployee(t, db, "树范围", "tree@test", "负责人", parent.ID)
	hierarchyScopeRole(t, db, actor.ID, "DEPARTMENT_TREE")
	scorer := createWorkflowFixtureEmployee(t, db, "子员工", "child@test", "工程师", grandchild.ID)
	outsider := createWorkflowFixtureEmployee(t, db, "外员工", "other@test", "工程师", other.ID)
	if !CanAccessEmployee(actor.ID, scorer.ID) || CanAccessEmployee(actor.ID, outsider.ID) {
		t.Fatal("tree employee scope")
	}
	eval := createWorkflowFixtureEvaluation(t, db, scorer.ID, 1, "pending", "")
	outsideEval := createWorkflowFixtureEvaluation(t, db, outsider.ID, 1, "pending", "")
	if !CanAccessEvaluation(actor.ID, eval.ID) || CanAccessEvaluation(actor.ID, outsideEval.ID) {
		t.Fatal("tree evaluation scope")
	}
	local := createWorkflowFixtureEmployee(t, db, "本部门范围", "local@test", "负责人", parent.ID)
	hierarchyScopeRole(t, db, local.ID, "DEPARTMENT")
	if CanAccessEmployee(local.ID, scorer.ID) {
		t.Fatal("department scope widened")
	}
	r := gin.New()
	r.GET("/preview", func(c *gin.Context) { c.Set("user_id", actor.ID); PreviewEvaluationTemplate(c) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", fmt.Sprintf("/preview?employee_id=%d", outsider.ID), nil))
	if w.Code != 403 {
		t.Fatalf("preview scope %d", w.Code)
	}
}

func TestManagerCandidatesAuthorizationAndHeadRules(t *testing.T) {
	db := catalogTestDB(t)
	old := models.DB
	models.DB = db
	t.Cleanup(func() { models.DB = old })
	parent := models.Department{Name: "研发部"}
	db.Create(&parent)
	child := models.Department{Name: "子部门", ParentID: &parent.ID}
	db.Create(&child)
	head := createWorkflowFixtureEmployee(t, db, "子负责人", "candidate-head@test", "部门负责人", child.ID)
	actor := createWorkflowFixtureEmployee(t, db, "管理者", "candidate-actor@test", "管理", parent.ID)
	hierarchyScopeRole(t, db, actor.ID, "ALL")
	cto := createWorkflowFixtureEmployee(t, db, "CTO", "candidate-cto@test", "CTO", parent.ID)
	createWorkflowFixtureResponsibility(t, db, "CTO", "CTO", "company", cto.ID, actor.ID, nil, "")
	createWorkflowFixtureResponsibility(t, db, "DEPARTMENT_MANAGER", "子负责人", "department", head.ID, actor.ID, &child.ID, "")
	r := gin.New()
	r.GET("/departments/:id/manager-candidates", func(c *gin.Context) { c.Set("user_id", actor.ID) }, PermissionAnyMiddleware("department:view", "employee:create", "employee:edit"), GetDepartmentManagerCandidates)
	url := fmt.Sprintf("/departments/%d/manager-candidates?position=部门负责人&employee_id=%d", child.ID, head.ID)
	request := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
		return w
	}
	if w := request(); w.Code != 403 {
		t.Fatalf("missing permission accepted: %d", w.Code)
	}
	var role models.Role
	db.Where("code = ?", "scope-ALL").First(&role)
	permission := models.Permission{Code: "employee:edit", Name: "编辑员工"}
	db.Create(&permission)
	db.Create(&models.RolePermission{RoleID: role.ID, PermissionID: permission.ID})
	w := request()
	if w.Code != 200 {
		t.Fatalf("allowed candidates %d %s", w.Code, w.Body.String())
	}
	var response struct {
		Data []models.Employee `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data) != 1 || response.Data[0].ID != cto.ID {
		t.Fatalf("expected only CTO, not self or unrelated employees: %s", w.Body.String())
	}
}

func TestMatchingPreviewAndCreateUseSameScorers(t *testing.T) {
	t.Setenv("EVALUATION_WORKFLOW_MODE", "enabled")
	db := catalogTestDB(t)
	old := models.DB
	models.DB = db
	t.Cleanup(func() { models.DB = old })
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	var dept models.Department
	db.Where("name = ?", "软件应用开发部").First(&dept)
	actor := createWorkflowFixtureEmployee(t, db, "HR", "hr@test", "HR", dept.ID)
	db.Model(&actor).Update("role", "hr")
	employee := createWorkflowFixtureEmployee(t, db, "实际工程师", "engineer@test", "软件应用开发工程师", dept.ID)
	r := gin.New()
	r.GET("/preview", func(c *gin.Context) { c.Set("user_id", actor.ID); PreviewEvaluationTemplate(c) })
	r.POST("/evaluations", func(c *gin.Context) { c.Set("user_id", actor.ID); CreateEvaluation(c) })
	preview := func() templateMatchPreview {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", fmt.Sprintf("/preview?employee_id=%d&period=monthly", employee.ID), nil))
		if w.Code != 200 {
			t.Fatalf("preview %d %s", w.Code, w.Body.String())
		}
		var payload struct {
			Data templateMatchPreview `json:"data"`
		}
		json.Unmarshal(w.Body.Bytes(), &payload)
		return payload.Data
	}
	blocked := preview()
	if blocked.WorkflowMode != "enabled" {
		t.Fatal("preview lost runtime mode")
	}
	if blocked.Template.ID == 0 || blocked.Ready || len(blocked.BlockingErrors) == 0 {
		t.Fatalf("lost template on missing responsibilities: %+v", blocked)
	}
	for _, code := range []string{"PM", "PRODUCT_MANAGER", "CTO", "DEPARTMENT_MANAGER"} {
		user := createWorkflowFixtureEmployee(t, db, code, code+"@test", "评分人", dept.ID)
		scope := "company"
		var deptID *uint
		if code == "DEPARTMENT_MANAGER" {
			scope = "department"
			deptID = &dept.ID
		}
		createWorkflowFixtureResponsibility(t, db, code, code, scope, user.ID, actor.ID, deptID, "")
	}
	ready := preview()
	if !ready.Ready {
		t.Fatalf("not ready %+v", ready)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/evaluations", strings.NewReader(fmt.Sprintf(`{"employee_id":%d,"template_id":%d,"period":"monthly","year":2026,"month":10}`, employee.ID, ready.Template.ID))))
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var evaluation models.KPIEvaluation
	db.Where("employee_id = ?", employee.ID).First(&evaluation)
	var tasks []models.EvaluationScoringTask
	db.Where("evaluation_id = ?", evaluation.ID).Find(&tasks)
	if len(tasks) == 0 {
		t.Fatal("no generated tasks")
	}
	for _, task := range tasks {
		if task.ScorerUserID == employee.ID {
			t.Fatal("self score task")
		}
		if ready.Scorers[task.ScorerType].ID != task.ScorerUserID {
			t.Fatal("preview/create differ")
		}
	}
	var unchanged models.Employee
	db.First(&unchanged, employee.ID)
	if unchanged.Position != "软件应用开发工程师" {
		t.Fatal("employee position rewritten")
	}
	t.Setenv("EVALUATION_WORKFLOW_MODE", "audit")
	auditPreview := preview()
	if auditPreview.WorkflowMode != "audit" {
		t.Fatal("preview mode not updated")
	}
	if auditPreview.Ready || !strings.Contains(strings.Join(auditPreview.BlockingErrors, ";"), "EVALUATION_WORKFLOW_MODE=enabled") {
		t.Fatal("audit preview should block new template")
	}
	blockedCreate := httptest.NewRecorder()
	r.ServeHTTP(blockedCreate, httptest.NewRequest("POST", "/evaluations", strings.NewReader(fmt.Sprintf(`{"employee_id":%d,"template_id":%d,"period":"monthly","year":2026,"month":11}`, employee.ID, ready.Template.ID))))
	if blockedCreate.Code != 409 || !strings.Contains(blockedCreate.Body.String(), "WORKFLOW_WRITES_DISABLED") {
		t.Fatalf("audit mixed workflow creation %d %s", blockedCreate.Code, blockedCreate.Body.String())
	}
	var count int64
	db.Model(&models.KPIEvaluation{}).Where("employee_id = ?", employee.ID).Count(&count)
	if count != 1 {
		t.Fatalf("blocked creation left partial records: %d", count)
	}
}

func TestDepartmentHeadNeverResolvesSelf(t *testing.T) {
	db := catalogTestDB(t)
	dept := models.Department{Name: "子部门"}
	db.Create(&dept)
	head := createWorkflowFixtureEmployee(t, db, "负责人", "head-self@test", "部门负责人", dept.ID)
	createWorkflowFixtureResponsibility(t, db, "DEPARTMENT_MANAGER", "负责人", "department", head.ID, head.ID, &dept.ID, "")
	template := models.KPITemplate{Items: []models.KPIItem{{ID: 1, Name: "质量", ScorerTypes: []string{"DEPARTMENT_MANAGER"}}}}
	if assignments, err := resolveTemplateScoringAssignments(db, head, template); err == nil {
		t.Fatalf("self assignments accepted: %+v", assignments)
	}
}
