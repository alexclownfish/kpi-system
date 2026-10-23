package models

import (
	"fmt"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func workflowModelTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:workflow-model-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&Department{}, &Employee{}, &KPITemplate{}, &KPIItem{}, &KPIEvaluation{},
		&EvaluationResponsibility{}, &EvaluationScoringTask{}, &EvaluationCalculationResult{},
		&AssessmentAuditFinding{}, &MigrationBatch{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestEvaluationScoringTaskUniquenessAndZeroScore(t *testing.T) {
	db := workflowModelTestDB(t)
	zero := float64(0)
	task := EvaluationScoringTask{
		EvaluationID: 1, ItemID: 10, ScorerUserID: 20, ScorerType: "PM",
		Status: "submitted", Score: &zero, Required: true, Version: 1,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	var loaded EvaluationScoringTask
	if err := db.First(&loaded, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if loaded.Score == nil || *loaded.Score != 0 {
		t.Fatalf("zero score must remain a submitted value: %+v", loaded.Score)
	}

	duplicate := task
	duplicate.ID = 0
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("expected duplicate evaluation/item/scorer task to be rejected")
	}
}

func TestEvaluationScoringTaskMissingScoreRemainsNil(t *testing.T) {
	db := workflowModelTestDB(t)
	task := EvaluationScoringTask{EvaluationID: 2, ItemID: 11, ScorerUserID: 21, ScorerType: "PM", Status: "pending", Required: true, Version: 1}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	var loaded EvaluationScoringTask
	if err := db.First(&loaded, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if loaded.Score != nil {
		t.Fatalf("missing score must remain nil, got %v", *loaded.Score)
	}
}

func TestWorkflowAutoMigratePreservesLegacyRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:workflow-legacy-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Department{}, &Employee{}, &KPITemplate{}, &KPIItem{}, &KPIEvaluation{}, &KPIScore{}, &EvaluationInvitation{}, &InvitedScore{}); err != nil {
		t.Fatal(err)
	}
	department := Department{Name: "研发部"}
	db.Create(&department)
	employee := Employee{Name: "历史员工", Email: "legacy-model@test", Password: "hash", DepartmentID: department.ID, IsActive: true}
	db.Create(&employee)
	template := KPITemplate{Name: "历史模板", Period: "monthly"}
	db.Create(&template)
	item := KPIItem{TemplateID: template.ID, Name: "历史指标", MaxScore: 100}
	db.Create(&item)
	evaluation := KPIEvaluation{EmployeeID: employee.ID, TemplateID: template.ID, Period: "2026-09", Status: "pending"}
	db.Create(&evaluation)
	legacyScore := KPIScore{EvaluationID: evaluation.ID, ItemID: item.ID}
	db.Create(&legacyScore)
	invitation := EvaluationInvitation{EvaluationID: evaluation.ID, InviterID: employee.ID, InviteeID: employee.ID, Status: "pending"}
	db.Create(&invitation)
	db.Create(&InvitedScore{InvitationID: invitation.ID, ItemID: item.ID})

	if err := db.AutoMigrate(&EvaluationResponsibility{}, &EvaluationScoringTask{}, &EvaluationCalculationResult{}, &AssessmentAuditFinding{}, &MigrationBatch{}); err != nil {
		t.Fatal(err)
	}
	for name, model := range map[string]any{
		"evaluation": &KPIEvaluation{}, "score": &KPIScore{}, "invitation": &EvaluationInvitation{}, "invited_score": &InvitedScore{},
	} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("legacy %s rows changed after migration: count=%d err=%v", name, count, err)
		}
	}
}

func TestEvaluationWorkflowModeDefaultsToAudit(t *testing.T) {
	t.Setenv("EVALUATION_WORKFLOW_MODE", "")
	if got := EvaluationWorkflowMode(); got != EvaluationWorkflowModeAudit {
		t.Fatalf("mode=%q, want audit", got)
	}
	if !EvaluationWorkflowAuditEnabled() || EvaluationWorkflowWritesEnabled() {
		t.Fatal("audit mode must allow audit and deny new writes")
	}

	t.Setenv("EVALUATION_WORKFLOW_MODE", "enabled")
	if !EvaluationWorkflowWritesEnabled() {
		t.Fatal("enabled mode must allow new workflow writes")
	}

	t.Setenv("EVALUATION_WORKFLOW_MODE", "disabled")
	if EvaluationWorkflowAuditEnabled() {
		t.Fatal("disabled mode must deny audit endpoints")
	}
}
