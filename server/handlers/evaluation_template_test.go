package handlers

import (
	"testing"

	"dootask-kpi-server/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestResolveTemplateScorersAndSnapshot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:evaluation-template-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Department{}, &models.Employee{}, &models.Role{}, &models.UserRole{}, &models.KPITemplate{}, &models.KPIDimension{}, &models.KPIItem{}); err != nil {
		t.Fatal(err)
	}
	department := models.Department{Name: "研发部"}
	if err := db.Create(&department).Error; err != nil {
		t.Fatal(err)
	}
	manager := models.Employee{Name: "部门负责人", Email: "manager@test", Position: "部门负责人", Role: "manager", DepartmentID: department.ID, IsActive: true}
	pm := models.Employee{Name: "项目经理", Email: "pm@test", Position: "普通岗位", DepartmentID: department.ID, IsActive: true}
	employee := models.Employee{Name: "员工", Email: "employee@test", Position: "工程师", DepartmentID: department.ID, ManagerID: &manager.ID, IsActive: true}
	for _, item := range []*models.Employee{&manager, &pm, &employee} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	pmRole := models.Role{Code: "pm", Name: "项目经理"}
	if err := db.Create(&pmRole).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.UserRole{UserID: pm.ID, RoleID: pmRole.ID}).Error; err != nil {
		t.Fatal(err)
	}
	template := models.KPITemplate{ID: 1, Name: "工程师模板", Period: "monthly", Version: 2, CalculationVersion: "v1", DepartmentID: &department.ID, PositionType: "工程师", Items: []models.KPIItem{{Name: "交付", Required: true, ScorerTypesJSON: `["PM","DEPARTMENT_MANAGER"]`}}}
	hydrateCalculationTemplate(&template)
	scorers, err := resolveTemplateScorers(db, employee, template)
	if err != nil {
		t.Fatal(err)
	}
	if scorers["PM"] != pm.ID || scorers["DEPARTMENT_MANAGER"] != manager.ID {
		t.Fatalf("unexpected scorers: %+v", scorers)
	}
	raw, err := buildEvaluationTemplateSnapshotWithScorers(template, scorers)
	if err != nil || raw == "" {
		t.Fatalf("snapshot error: %v", err)
	}
}
