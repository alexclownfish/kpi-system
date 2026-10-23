package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
)

func TestWorkSummaryIsRequiredBeforeSelfScore(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	owner := createWorkflowFixtureEmployee(t, db, "总结员工", "summary-owner@test", "engineer", 1)
	evaluation := createWorkflowFixtureEvaluation(t, db, owner.ID, 1, workflowStateSelfEvaluating, requiredWeightedWorkflowSnapshot(t))
	db.Model(&evaluation).Update("work_summary_required", true)

	router := gin.New()
	router.POST("/evaluations/:id/work-summary", func(c *gin.Context) {
		c.Set("user_id", owner.ID)
		SubmitEvaluationWorkSummary(c)
	})
	record := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/evaluations/%d/work-summary", evaluation.ID), strings.NewReader(`{"work_summary":"交付完成","work_risks":"无","support_needed":"无","next_plan":"下月完成版本发布"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(record, request)
	if record.Code != http.StatusOK {
		t.Fatalf("summary submit=%d %s", record.Code, record.Body.String())
	}
	var stored models.KPIEvaluation
	if err := db.First(&stored, evaluation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.WorkSummarySubmittedAt == nil || stored.WorkSummary == "" || stored.NextPlan == "" {
		t.Fatalf("summary was not persisted: %+v", stored)
	}
	var audit models.AuditLog
	if err := db.Where("action = ? AND resource_id = ?", "submit_work_summary", fmt.Sprint(evaluation.ID)).First(&audit).Error; err != nil {
		t.Fatalf("missing summary audit: %v", err)
	}

	// A second submission is locked after the self-evaluation phase starts.
	db.Model(&evaluation).Update("status", workflowStateScoring)
	second := httptest.NewRecorder()
	secondRequest := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/evaluations/%d/work-summary", evaluation.ID), strings.NewReader(`{"work_summary":"x","work_risks":"x","support_needed":"x","next_plan":"x"}`))
	secondRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(second, secondRequest)
	if second.Code != http.StatusConflict {
		t.Fatalf("late summary submission=%d %s", second.Code, second.Body.String())
	}
}

func TestScoringSequenceBlocksLaterScorersUntilPMSubmits(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	owner := createWorkflowFixtureEmployee(t, db, "顺序员工", "sequence-owner@test", "engineer", 1)
	pm := createWorkflowFixtureEmployee(t, db, "项目经理", "sequence-pm@test", "PM", 1)
	product := createWorkflowFixtureEmployee(t, db, "产品经理", "sequence-product@test", "产品经理", 1)
	evaluation := createWorkflowFixtureEvaluation(t, db, owner.ID, 1, workflowStateScoring, requiredWeightedWorkflowSnapshot(t))
	pmTask := models.EvaluationScoringTask{EvaluationID: evaluation.ID, ItemID: 10, ScorerUserID: pm.ID, ScorerType: "PM", Sequence: 1, Required: true, Status: "pending", Version: 1}
	productTask := models.EvaluationScoringTask{EvaluationID: evaluation.ID, ItemID: 10, ScorerUserID: product.ID, ScorerType: "PRODUCT_MANAGER", Sequence: 3, Required: true, Status: "pending", Version: 1}
	if err := db.Create(&pmTask).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&productTask).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/scoring-tasks/:id/submit", func(c *gin.Context) {
		c.Set("user_id", product.ID)
		SubmitScoringTask(c)
	})
	submitAsProduct := func(taskID uint) *httptest.ResponseRecorder {
		record := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/scoring-tasks/%d/submit", taskID), strings.NewReader(`{"score":80,"comment":"ok","version":1}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(record, request)
		return record
	}
	if record := submitAsProduct(productTask.ID); record.Code != http.StatusConflict || !strings.Contains(record.Body.String(), "SCORING_SEQUENCE_BLOCKED") {
		t.Fatalf("out-of-order submit=%d %s", record.Code, record.Body.String())
	}
	db.Model(&pmTask).Updates(map[string]any{"status": "submitted", "score": 80})
	if record := submitAsProduct(productTask.ID); record.Code != http.StatusOK {
		t.Fatalf("ordered submit=%d %s", record.Code, record.Body.String())
	}
}
