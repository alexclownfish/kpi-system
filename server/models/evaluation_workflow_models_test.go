package models

import (
	"fmt"
	"gorm.io/driver/postgres"
	"os"
	"testing"
	"time"

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

func TestScoringTaskDutyIndexUpgradePreservesScores(t *testing.T) {
	db := workflowModelTestDB(t)
	assertScoringTaskDutyIndexUpgrade(t, db)
}

func assertScoringTaskDutyIndexUpgrade(t *testing.T, db *gorm.DB) {
	t.Helper()
	// Simulate an installation created by the previous release.
	if err := db.Migrator().DropIndex(&EvaluationScoringTask{}, "idx_evaluation_item_scorer_duty"); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX idx_evaluation_item_scorer ON evaluation_scoring_tasks(evaluation_id,item_id,scorer_user_id)").Error; err != nil {
		t.Fatal(err)
	}
	department := Department{Name: "索引回归部门"}
	if err := db.Create(&department).Error; err != nil {
		t.Fatal(err)
	}
	employee := Employee{Name: "索引回归员工", Email: "index-fixture@example.invalid", DepartmentID: department.ID, IsActive: true}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatal(err)
	}
	template := KPITemplate{Name: "索引回归模板", Period: "monthly"}
	if err := db.Create(&template).Error; err != nil {
		t.Fatal(err)
	}
	item := KPIItem{TemplateID: template.ID, Name: "索引回归指标"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	evaluation := KPIEvaluation{EmployeeID: employee.ID, TemplateID: template.ID, Period: "monthly"}
	if err := db.Create(&evaluation).Error; err != nil {
		t.Fatal(err)
	}
	score := 87.5
	old := EvaluationScoringTask{EvaluationID: evaluation.ID, ItemID: item.ID, ScorerUserID: employee.ID, ScorerType: "CTO", Status: "submitted", Score: &score, Comment: "历史评分", Version: 3}
	if err := db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := migrateSchema(db); err != nil {
			t.Fatal(err)
		}
	}
	var loaded EvaluationScoringTask
	if err := db.First(&loaded, old.ID).Error; err != nil {
		t.Fatal(err)
	}
	if loaded.Score == nil || *loaded.Score != score || loaded.Comment != old.Comment || loaded.Version != old.Version || loaded.Status != old.Status {
		t.Fatalf("historical task changed: %+v", loaded)
	}
	next := EvaluationScoringTask{EvaluationID: evaluation.ID, ItemID: item.ID, ScorerUserID: employee.ID, ScorerType: "DEPARTMENT_MANAGER", Status: "pending", Version: 1}
	if err := db.Create(&next).Error; err != nil {
		t.Fatalf("second duty rejected: %v", err)
	}
	next.ID = 0
	if err := db.Create(&next).Error; err == nil {
		t.Fatal("same duty duplicate accepted")
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

func TestPostgresScoringTaskDutyIndexUpgrade(t *testing.T) {
	if os.Getenv("KPI_TEST_DISPOSABLE_POSTGRES") != "1" {
		t.Skip("requires isolated PostgreSQL test database")
	}
	dsn := os.Getenv("KPI_TEST_POSTGRES_DSN")
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal("open test PostgreSQL")
	}
	schema := fmt.Sprintf("task_index_test_%d", time.Now().UnixNano())
	if err := admin.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
		_ = admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`).Error
		adminSQL, _ := admin.DB()
		_ = adminSQL.Close()
	})
	// Exercise the production schema path, including PostgreSQL constraints.
	if err := migrateSchema(db); err != nil {
		t.Fatal(err)
	}
	assertScoringTaskDutyIndexUpgrade(t, db)
}
