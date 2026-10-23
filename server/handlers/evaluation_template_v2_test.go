package handlers

import (
	"strings"
	"testing"

	"dootask-kpi-server/models"
)

func TestFindPublishedTemplateRequiresOneExactDepartmentPositionPeriodMatch(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	parent := models.Department{Name: "研发中心"}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	child := models.Department{Name: "软件应用开发部", ParentID: &parent.ID}
	if err := db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	manager := models.Employee{Name: "负责人", Email: "match-manager@test", Position: "部门负责人", DepartmentID: child.ID, IsActive: true}
	if err := db.Create(&manager).Error; err != nil {
		t.Fatal(err)
	}
	item := models.KPIItem{Name: "交付", MaxScore: 100, Required: true}
	makeTemplate := func(departmentID uint, position, period string) models.KPITemplate {
		template := models.KPITemplate{Name: position + "模板", DepartmentID: &departmentID, PositionType: position, Period: period, Version: 1, Status: "published", IsActive: true, Items: []models.KPIItem{item}}
		if err := db.Create(&template).Error; err != nil {
			t.Fatal(err)
		}
		return template
	}
	makeTemplate(child.ID, "部门负责人", "monthly")
	engineer := models.Employee{Name: "工程师", Email: "match-engineer@test", Position: "软件应用开发部工程师", DepartmentID: child.ID, IsActive: true}
	if err := db.Create(&engineer).Error; err != nil {
		t.Fatal(err)
	}
	// The catalog position fallback is allowed only in the employee's own
	// department; a parent template must never leak into a child department.
	if _, err := findPublishedTemplate(db, engineer, "monthly"); err == nil {
		t.Fatal("expected no parent/child hierarchy fallback when child template is absent")
	}
	makeTemplate(child.ID, "工程师", "monthly")
	matched, err := findPublishedTemplate(db, engineer, "monthly")
	if err != nil || matched.PositionType != "工程师" {
		t.Fatalf("canonical match failed: %+v %v", matched, err)
	}
	if _, err := findPublishedTemplate(db, engineer, "quarterly"); err == nil {
		t.Fatal("expected period boundary to reject a monthly template")
	}
	if _, err := findPublishedTemplate(db, models.Employee{Name: "空岗位", DepartmentID: child.ID}, "monthly"); err == nil || !strings.Contains(err.Error(), "未配置岗位") {
		t.Fatalf("expected explicit empty-position error, got %v", err)
	}
}

func TestFindPublishedTemplateRejectsAmbiguousPublishedVersions(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	department := models.Department{Name: "测试部"}
	if err := db.Create(&department).Error; err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{1, 2} {
		template := models.KPITemplate{Name: "重复模板", DepartmentID: &department.ID, PositionType: "工程师", Period: "monthly", Version: version, Status: "published", IsActive: true}
		if err := db.Create(&template).Error; err != nil {
			t.Fatal(err)
		}
	}
	employee := models.Employee{Name: "员工", Email: "ambiguous@test", Position: "工程师", DepartmentID: department.ID, IsActive: true}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := findPublishedTemplate(db, employee, "monthly"); err == nil || !strings.Contains(err.Error(), "TEMPLATE_MATCH_AMBIGUOUS") {
		t.Fatalf("expected stable ambiguous-match error, got %v", err)
	}
}
