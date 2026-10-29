package handlers

import (
	"fmt"
	"testing"

	"dootask-kpi-server/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// These builders keep workflow regression fixtures deterministic and additive.
// They are intentionally small so tests can compose only the rows they need.
func newEvaluationWorkflowTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:evaluation-workflow-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	migrateWorkflowFixtureDB(t, db)
	return db
}

func migrateWorkflowFixtureDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.AutoMigrate(
		&models.Role{}, &models.Permission{}, &models.UserRole{}, &models.RolePermission{},
		&models.DataScope{}, &models.RoleDataScope{}, &models.AuditLog{},
		&models.Department{}, &models.Employee{}, &models.KPITemplate{}, &models.KPIDimension{},
		&models.KPIItem{}, &models.KPIEvaluation{}, &models.KPIScore{},
		&models.EvaluationResponsibility{}, &models.EvaluationScoringTask{},
		&models.EvaluationCalculationResult{}, &models.EvaluationInvitation{}, &models.InvitedScore{},
		&models.AssessmentAuditFinding{}, &models.MigrationBatch{}, &models.EvaluationResultSnapshot{}, &models.EvaluationConfirmation{},
	); err != nil {
		t.Fatalf("migrate fixture db: %v", err)
	}
}

func createWorkflowFixtureEmployee(t *testing.T, db *gorm.DB, name, email, position string, departmentID uint) models.Employee {
	t.Helper()
	employee := models.Employee{Name: name, Email: email, Password: "fixture-hash", Position: position, DepartmentID: departmentID, IsActive: true}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatalf("create employee fixture: %v", err)
	}
	return employee
}

func createWorkflowFixtureResponsibility(t *testing.T, db *gorm.DB, code, name, scope string, userID, createdBy uint, departmentID *uint, position string) models.EvaluationResponsibility {
	t.Helper()
	item := models.EvaluationResponsibility{Code: code, Name: name, ScopeType: scope, DepartmentID: departmentID, PositionType: position, UserID: userID, Active: true, CreatedBy: createdBy}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create responsibility fixture: %v", err)
	}
	return item
}

func createWorkflowFixtureEvaluation(t *testing.T, db *gorm.DB, employeeID, templateID uint, status, snapshot string) models.KPIEvaluation {
	t.Helper()
	evaluation := models.KPIEvaluation{EmployeeID: employeeID, TemplateID: templateID, Period: "monthly", Year: 2026, Status: status, TemplateSnapshotJSON: snapshot, CalculationVersion: canonicalCalculationVersion}
	if err := db.Create(&evaluation).Error; err != nil {
		t.Fatalf("create evaluation fixture: %v", err)
	}
	return evaluation
}

func createWorkflowFixtureTask(t *testing.T, db *gorm.DB, evaluationID, itemID, scorerID uint, required bool, score *float64, status string) models.EvaluationScoringTask {
	t.Helper()
	task := models.EvaluationScoringTask{EvaluationID: evaluationID, ItemID: itemID, ScorerUserID: scorerID, ScorerType: "PM", Required: required, Status: status, Score: score, Version: 1}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create scoring task fixture: %v", err)
	}
	return task
}
