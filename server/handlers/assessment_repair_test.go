package handlers

import (
	"testing"

	"dootask-kpi-server/models"
)

func TestBuildMigrationPlanSkipsLegacySnapshotForManualReview(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	department := models.Department{Name: "迁移部门"}
	db.Create(&department)
	actor := createWorkflowFixtureEmployee(t, db, "迁移管理员", "migration-admin@test", "HR", department.ID)
	evaluation := models.KPIEvaluation{EmployeeID: actor.ID, TemplateID: 1, Period: "monthly", Year: 2026, Status: "pending", TemplateSnapshotJSON: `{"template_id":1,"template_name":"旧模板","template_version":1,"items":[{"id":10,"name":"指标","max_score":100,"required":true}],"scorers":{"PM":1}}`}
	db.Create(&evaluation)
	plans, manual, err := buildMigrationPlan(db, []uint{evaluation.ID}, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 0 || len(manual) != 1 {
		t.Fatalf("plans=%v manual=%v", plans, manual)
	}
}

func TestBuildMigrationPlanIsDeterministicForValidWeightedSnapshot(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	department := models.Department{Name: "迁移部门"}
	db.Create(&department)
	actor := createWorkflowFixtureEmployee(t, db, "迁移管理员", "migration-deterministic@test", "HR", department.ID)
	raw, err := buildWorkflowTemplateSnapshot(models.KPITemplate{ID: 1, Name: "新版模板", Period: "monthly", Version: 2, Items: []models.KPIItem{{ID: 10, Name: "指标", MaxScore: 100, ItemWeight: 100, CompositeWeight: 100, Required: true, ScorerTypesJSON: `["PM"]`}}}, []workflowScoringAssignment{{ItemID: 10, ScorerType: "PM", UserID: actor.ID, UserName: actor.Name, SourceRule: "fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	month := 9
	evaluation := models.KPIEvaluation{EmployeeID: actor.ID, TemplateID: 1, Period: "monthly", Year: 2026, Month: &month, Status: workflowStateScoring, TemplateSnapshotJSON: raw}
	db.Create(&evaluation)
	db.Create(&models.KPIScore{EvaluationID: evaluation.ID, ItemID: 10})
	first, manual, err := buildMigrationPlan(db, []uint{evaluation.ID}, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, manualAgain, err := buildMigrationPlan(db, []uint{evaluation.ID}, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 1 || len(manual) != 0 || len(manualAgain) != 0 || first[0].SourceChecksum != second[0].SourceChecksum {
		t.Fatalf("first=%v second=%v manual=%v/%v", first, second, manual, manualAgain)
	}
}
