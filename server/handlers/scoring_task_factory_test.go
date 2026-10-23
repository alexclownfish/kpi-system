package handlers

import (
	"testing"

	"dootask-kpi-server/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestResolveTemplateScoringAssignmentsUsesScopedResponsibilities(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:scoring-resolver-scope?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Department{}, &models.Employee{}, &models.EvaluationResponsibility{}, &models.KPITemplate{}, &models.KPIItem{}); err != nil {
		t.Fatal(err)
	}
	department := models.Department{Name: "研发部"}
	db.Create(&department)
	employee := models.Employee{Name: "被考核人", Email: "scope-target@test", Position: "工程师", DepartmentID: department.ID, IsActive: true}
	pm := models.Employee{Name: "项目经理", Email: "scope-pm@test", Position: "PM", DepartmentID: department.ID, IsActive: true}
	product := models.Employee{Name: "产品经理", Email: "scope-product@test", Position: "产品经理", DepartmentID: department.ID, IsActive: true}
	for _, value := range []*models.Employee{&employee, &pm, &product} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	db.Create(&models.EvaluationResponsibility{Code: "PM", Name: "项目经理", ScopeType: "department", DepartmentID: &department.ID, UserID: pm.ID, Active: true, CreatedBy: employee.ID})
	db.Create(&models.EvaluationResponsibility{Code: "PM", Name: "项目经理（公司范围）", ScopeType: "company", UserID: pm.ID, Active: true, CreatedBy: employee.ID})
	db.Create(&models.EvaluationResponsibility{Code: "PRODUCT_MANAGER", Name: "产品经理", ScopeType: "position", DepartmentID: &department.ID, PositionType: employee.Position, UserID: product.ID, Active: true, CreatedBy: employee.ID})
	template := models.KPITemplate{ID: 1, Name: "研发模板", Period: "monthly", Version: 1, Items: []models.KPIItem{{ID: 11, Name: "交付", MaxScore: 100, Required: true, ScorerTypes: []string{"PM", "PRODUCT_MANAGER"}}}}
	assignments, err := resolveTemplateScoringAssignments(db, employee, template)
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 2 {
		t.Fatalf("assignments=%+v", assignments)
	}
	if assignments[0].UserID == assignments[1].UserID {
		t.Fatalf("different scorer responsibilities should resolve to distinct users in this fixture: %+v", assignments)
	}
	if _, err := resolveTemplateScoringAssignments(db, employee, models.KPITemplate{ID: 2, Name: "缺少职责", Period: "monthly", Items: []models.KPIItem{{ID: 12, Name: "技术", MaxScore: 100, Required: true, ScorerTypes: []string{"CTO"}}}}); err == nil {
		t.Fatal("expected missing CTO responsibility to fail explicitly")
	}

	if err := db.Model(&models.EvaluationResponsibility{}).Where("code = ?", "PM").Update("active", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := resolveTemplateScoringAssignments(db, employee, template); err == nil {
		t.Fatal("expected inactive PM responsibility to block new assessment resolution")
	}
}

func TestGenerateEvaluationScoringTasksIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:scoring-task-factory?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Employee{}, &models.KPITemplate{}, &models.KPIEvaluation{}, &models.KPIItem{}, &models.EvaluationScoringTask{}); err != nil {
		t.Fatal(err)
	}
	evaluation := models.KPIEvaluation{EmployeeID: 1, TemplateID: 1, Status: "self_evaluating"}
	if err := db.Create(&evaluation).Error; err != nil {
		t.Fatal(err)
	}
	template := models.KPITemplate{ID: 1, Name: "研发模板", Period: "monthly", Version: 1, Items: []models.KPIItem{{ID: 11, Name: "交付", MaxScore: 100, Required: true, ScorerTypes: []string{"PM"}}}}
	assignment := []workflowScoringAssignment{{ItemID: 11, ScorerType: "PM", UserID: 2, UserName: "项目经理", SourceRule: "evaluation_responsibility"}}
	raw, err := buildWorkflowTemplateSnapshot(template, assignment)
	if err != nil {
		t.Fatal(err)
	}
	evaluation.TemplateSnapshotJSON = raw
	snapshot, err := parseEvaluationSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	first, err := generateEvaluationScoringTasks(db, evaluation, snapshot)
	if err != nil || len(first) != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := generateEvaluationScoringTasks(db, evaluation, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("retry created duplicate tasks: %+v", second)
	}
	var count int64
	db.Model(&models.EvaluationScoringTask{}).Count(&count)
	if count != 1 {
		t.Fatalf("task count=%d", count)
	}
	// Changing the legacy RBAC/role column after task creation must not alter
	// the snapshotted scorer assignment.
	if err := db.Model(&models.Employee{}).Where("id = ?", 2).Update("role", "employee").Error; err != nil {
		t.Fatal(err)
	}
	var persisted models.EvaluationScoringTask
	if err := db.First(&persisted).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.ScorerUserID != 2 || persisted.ScorerType != "PM" {
		t.Fatalf("task assignment changed after RBAC update: %+v", persisted)
	}
}
