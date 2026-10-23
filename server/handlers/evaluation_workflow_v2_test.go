package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
)

func TestEvaluationWorkflowV2RequiredGateIsAtomicAndAudited(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	owner := createWorkflowFixtureEmployee(t, db, "被考核人", "gate-owner@test", "engineer", 1)
	evaluation := createWorkflowFixtureEvaluation(t, db, owner.ID, 1, workflowStateScoring, requiredWeightedWorkflowSnapshot(t))

	router := gin.New()
	router.POST("/evaluations/:id/workflow/advance", func(c *gin.Context) {
		c.Set("user_id", owner.ID)
		AdvanceEvaluationWorkflow(c)
	})
	advance := func() *httptest.ResponseRecorder {
		record := httptest.NewRecorder()
		request := httptest.NewRequest("POST", fmt.Sprintf("/evaluations/%d/workflow/advance", evaluation.ID), strings.NewReader(`{"expected_state":"scoring"}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(record, request)
		return record
	}

	record := advance()
	if record.Code != 409 || !strings.Contains(record.Body.String(), "REQUIRED_SCORES_INCOMPLETE") {
		t.Fatalf("incomplete gate status=%d body=%s", record.Code, record.Body.String())
	}
	var unchanged models.KPIEvaluation
	db.First(&unchanged, evaluation.ID)
	if unchanged.Status != workflowStateScoring {
		t.Fatalf("failed confirmation gate changed state=%s", unchanged.Status)
	}
	var rejectedAudit models.AuditLog
	if err := db.Where("action = ? AND resource_id = ?", "confirmation_gate_rejected", fmt.Sprint(evaluation.ID)).First(&rejectedAudit).Error; err != nil {
		t.Fatalf("missing rejected audit: %v", err)
	}

	zero := float64(0)
	if err := db.Create(&models.KPIScore{EvaluationID: evaluation.ID, ItemID: 10, SelfScore: &zero}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.EvaluationScoringTask{EvaluationID: evaluation.ID, ItemID: 10, ScorerUserID: 2, ScorerType: "PM", Required: true, Status: "submitted", Score: &zero, Version: 1}).Error; err != nil {
		t.Fatal(err)
	}
	record = advance()
	if record.Code != 200 {
		t.Fatalf("complete gate status=%d body=%s", record.Code, record.Body.String())
	}
	db.First(&unchanged, evaluation.ID)
	if unchanged.Status != workflowStateAwaitingEmployeeConfirmation || unchanged.TotalScore != 0 {
		t.Fatalf("complete gate result=%+v", unchanged)
	}
	var result models.EvaluationCalculationResult
	if err := db.Where("evaluation_id = ?", evaluation.ID).First(&result).Error; err != nil {
		t.Fatalf("missing calculation result: %v", err)
	}
	var calculatedAudit models.AuditLog
	if err := db.Where("action = ? AND resource_id = ?", "evaluation_calculated", fmt.Sprint(evaluation.ID)).First(&calculatedAudit).Error; err != nil {
		t.Fatalf("missing calculated audit: %v", err)
	}
	var response struct {
		Data struct {
			CurrentState string `json:"current_state"`
		} `json:"data"`
	}
	if err := json.Unmarshal(record.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.CurrentState != workflowStateAwaitingEmployeeConfirmation {
		t.Fatalf("response=%s", record.Body.String())
	}
}

func TestEvaluationWorkflowV2RequiresSelfEvaluationBeforeScoring(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	owner := createWorkflowFixtureEmployee(t, db, "自评员工", "workflow-order@test", "engineer", 1)
	evaluation := createWorkflowFixtureEvaluation(t, db, owner.ID, 1, workflowStateSelfEvaluating, requiredWeightedWorkflowSnapshot(t))
	router := gin.New()
	router.POST("/evaluations/:id/workflow/advance", func(c *gin.Context) {
		c.Set("user_id", owner.ID)
		AdvanceEvaluationWorkflow(c)
	})
	advance := func() *httptest.ResponseRecorder {
		record := httptest.NewRecorder()
		request := httptest.NewRequest("POST", fmt.Sprintf("/evaluations/%d/workflow/advance", evaluation.ID), strings.NewReader(`{"expected_state":"self_evaluating"}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(record, request)
		return record
	}
	if record := advance(); record.Code != 409 || !strings.Contains(record.Body.String(), "REQUIRED_SCORES_INCOMPLETE") {
		t.Fatalf("missing self score should block scoring: status=%d body=%s", record.Code, record.Body.String())
	}
	var stored models.KPIEvaluation
	db.First(&stored, evaluation.ID)
	if stored.Status != workflowStateSelfEvaluating {
		t.Fatalf("failed self gate changed status=%s", stored.Status)
	}
	zero := float64(0)
	if err := db.Create(&models.KPIScore{EvaluationID: evaluation.ID, ItemID: 10, SelfScore: &zero}).Error; err != nil {
		t.Fatal(err)
	}
	if record := advance(); record.Code != 200 || !strings.Contains(record.Body.String(), workflowStateScoring) {
		t.Fatalf("complete self evaluation should enter scoring: status=%d body=%s", record.Code, record.Body.String())
	}
	db.First(&stored, evaluation.ID)
	if stored.Status != workflowStateScoring {
		t.Fatalf("self evaluation transition status=%s", stored.Status)
	}
}
