package handlers

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
)

func TestLegacyObjectionSubmissionAndHandlingStayInPendingConfirmation(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	employee, _, evaluation := seedConfirmationEvaluation(t, "pending_confirm")

	r := gin.New()
	r.POST("/evaluations/:id/objection", func(c *gin.Context) { c.Set("user_id", employee.ID) }, SubmitObjection)
	request := httptest.NewRequest(http.MethodPost, "/evaluations/"+strconv.FormatUint(uint64(evaluation.ID), 10)+"/objection", strings.NewReader(`{"reason":"需要补充交付说明"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("legacy objection submit status=%d body=%s", response.Code, response.Body.String())
	}
	models.DB.First(&evaluation, evaluation.ID)
	if !evaluation.HasObjection || evaluation.Status != "pending_confirm" {
		t.Fatalf("legacy objection must preserve pending_confirm: %+v", evaluation)
	}

	r = gin.New()
	r.PUT("/evaluations/:id/objection/handle", func(c *gin.Context) { c.Set("user_id", employee.ID) }, HandleObjection)
	request = httptest.NewRequest(http.MethodPut, "/evaluations/"+strconv.FormatUint(uint64(evaluation.ID), 10)+"/objection/handle", strings.NewReader(`{"total_score":0,"final_comment":"已复核"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("legacy objection handle status=%d body=%s", response.Code, response.Body.String())
	}
	models.DB.First(&evaluation, evaluation.ID)
	if evaluation.HasObjection || evaluation.Status != "pending_confirm" || evaluation.TotalScore != 0 {
		t.Fatalf("legacy objection handling should accept zero and remain pending_confirm: %+v", evaluation)
	}
}

func TestWeightedObjectionTransitionsAndRejectsTerminalRecords(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	employee, _, evaluation := seedConfirmationEvaluation(t, workflowStateAwaitingEmployeeConfirmation)
	models.DB.Model(&evaluation).Updates(map[string]any{
		"calculation_version":    canonicalCalculationVersion,
		"template_snapshot_json": requiredWeightedWorkflowSnapshot(t),
	})

	r := gin.New()
	r.POST("/evaluations/:id/objection", func(c *gin.Context) { c.Set("user_id", employee.ID) }, SubmitObjection)
	request := httptest.NewRequest(http.MethodPost, "/evaluations/"+strconv.FormatUint(uint64(evaluation.ID), 10)+"/objection", strings.NewReader(`{"reason":"请复核评分"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("weighted objection submit status=%d body=%s", response.Code, response.Body.String())
	}
	models.DB.First(&evaluation, evaluation.ID)
	if !evaluation.HasObjection || evaluation.Status != workflowStateRejected {
		t.Fatalf("weighted objection must enter rejected: %+v", evaluation)
	}
	zero := float64(0)
	if err := models.DB.Create(&models.KPIScore{EvaluationID: evaluation.ID, ItemID: 10, SelfScore: &zero}).Error; err != nil {
		t.Fatal(err)
	}
	if err := models.DB.Create(&models.EvaluationScoringTask{EvaluationID: evaluation.ID, ItemID: 10, ScorerUserID: employee.ID, ScorerType: "PM", Required: true, Status: "submitted", Score: &zero, Version: 1}).Error; err != nil {
		t.Fatal(err)
	}

	r = gin.New()
	r.PUT("/evaluations/:id/objection/handle", func(c *gin.Context) { c.Set("user_id", employee.ID) }, HandleObjection)
	request = httptest.NewRequest(http.MethodPut, "/evaluations/"+strconv.FormatUint(uint64(evaluation.ID), 10)+"/objection/handle", strings.NewReader(`{"total_score":99,"final_comment":"已复核"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("weighted objection handle status=%d body=%s", response.Code, response.Body.String())
	}
	models.DB.First(&evaluation, evaluation.ID)
	if evaluation.HasObjection || evaluation.Status != workflowStateAwaitingEmployeeConfirmation || evaluation.TotalScore != 0 {
		t.Fatalf("weighted handling must ignore forged total and return to confirmation: %+v", evaluation)
	}

	// A completed record with a stale objection marker must remain terminal.
	models.DB.Model(&evaluation).Updates(map[string]any{"status": workflowStateCompleted, "has_objection": true})
	request = httptest.NewRequest(http.MethodPut, "/evaluations/"+strconv.FormatUint(uint64(evaluation.ID), 10)+"/objection/handle", strings.NewReader(`{"final_comment":"不应处理"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "不能处理异议") {
		t.Fatalf("completed weighted objection must be rejected: status=%d body=%s", response.Code, response.Body.String())
	}
}
