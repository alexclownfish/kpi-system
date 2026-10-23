package handlers

import (
	"fmt"
	"testing"

	"dootask-kpi-server/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPersistCanonicalResultIgnoresLegacyClientTotalAndVersionsChanges(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:canonical-result-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Employee{}, &models.KPITemplate{}, &models.KPIEvaluation{}, &models.EvaluationScoringTask{}, &models.EvaluationCalculationResult{}); err != nil {
		t.Fatal(err)
	}
	template := models.KPITemplate{ID: 1, Name: "结果模板", Period: "monthly", Version: 1}
	db.Create(&template)
	score := float64(80)
	dimensionID := uint(20)
	raw, err := buildWorkflowTemplateSnapshot(models.KPITemplate{ID: 1, Name: "结果模板", Period: "monthly", Version: 1, Dimensions: []models.KPIDimension{{ID: dimensionID, TemplateID: 1, Name: "交付", Weight: 100}}, Items: []models.KPIItem{{ID: 10, Name: "交付", MaxScore: 100, DimensionID: &dimensionID, ItemWeight: 100, CompositeWeight: 100, Required: true, ScorerTypes: []string{"PM"}}}}, []workflowScoringAssignment{{ItemID: 10, ScorerType: "PM", UserID: 2, UserName: "PM", SourceRule: "evaluation_responsibility"}})
	if err != nil {
		t.Fatal(err)
	}
	evaluation := models.KPIEvaluation{ID: 1, EmployeeID: 1, TemplateID: template.ID, Status: workflowStateAwaitingEmployeeConfirmation, CalculationVersion: canonicalCalculationVersion, TemplateSnapshotJSON: raw, TotalScore: 999}
	db.Create(&evaluation)
	db.Create(&models.EvaluationScoringTask{EvaluationID: evaluation.ID, ItemID: 10, ScorerUserID: 2, ScorerType: "PM", Required: true, Status: "submitted", Score: &score, Version: 1})
	stored, result, err := persistCanonicalEvaluationResult(db, evaluation.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Version != 1 || result.TotalScore != 80 {
		t.Fatalf("stored=%+v result=%+v", stored, result)
	}
	var updated models.KPIEvaluation
	db.First(&updated, evaluation.ID)
	if updated.TotalScore != 80 {
		t.Fatalf("legacy total was not replaced by canonical result: %v", updated.TotalScore)
	}
	newScore := float64(90)
	db.Model(&models.EvaluationScoringTask{}).Where("evaluation_id = ?", evaluation.ID).Updates(map[string]any{"score": newScore, "version": 2})
	stored, _, err = persistCanonicalEvaluationResult(db, evaluation.ID, 2)
	if err != nil || stored.Version != 2 {
		t.Fatalf("second result=%+v err=%v", stored, err)
	}
}
