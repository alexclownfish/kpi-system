package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func requiredWeightedWorkflowSnapshot(t *testing.T) string {
	t.Helper()
	item := workflowSnapshotItem{ID: 10, Name: "交付质量", MaxScore: 100, ItemWeight: 100, CompositeWeight: 100, Required: true, Aggregation: "average"}
	snapshot := workflowEvaluationSnapshot{
		Template:           workflowTemplateIdentity{ID: 1, Name: "研发考核", Version: 1, Period: "monthly"},
		CalculationVersion: canonicalCalculationVersion,
		Dimensions:         []workflowSnapshotDimension{{ID: 1, Name: "结果", Weight: 100, Items: []workflowSnapshotItem{item}}},
		ResolvedScorers:    []evaluationResolvedScorer{{ItemID: 10, ScorerType: "PM", UserID: 2}},
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestRequiredTaskCompletionDistinguishesZeroMissingAndRejected(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:workflow-completion-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.EvaluationScoringTask{}); err != nil {
		t.Fatal(err)
	}
	snapshot := parsedEvaluationSnapshot{Items: []workflowSnapshotItem{{ID: 10, Name: "交付", MaxScore: 100, Required: true}}}
	complete, blocking := requiredScoringTasksCompletion(db, 1, snapshot)
	if complete || len(blocking) == 0 {
		t.Fatal("missing required task must block workflow")
	}
	zero := float64(0)
	task := models.EvaluationScoringTask{EvaluationID: 1, ItemID: 10, ScorerUserID: 2, ScorerType: "PM", Required: true, Status: "submitted", Score: &zero, Version: 1}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	complete, blocking = requiredScoringTasksCompletion(db, 1, snapshot)
	if !complete || len(blocking) != 0 {
		t.Fatalf("submitted zero score should complete task: %v", blocking)
	}
	if err := db.Model(&task).Update("status", "rejected").Error; err != nil {
		t.Fatal(err)
	}
	complete, blocking = requiredScoringTasksCompletion(db, 1, snapshot)
	if complete || len(blocking) == 0 {
		t.Fatal("rejected required task must block workflow")
	}
	if err := db.Model(&task).Update("status", "not_applicable").Error; err != nil {
		t.Fatal(err)
	}
	complete, blocking = requiredScoringTasksCompletion(db, 1, snapshot)
	if complete || len(blocking) == 0 {
		t.Fatal("required task marked not-applicable must still block workflow")
	}
}

func TestNotApplicablePolicyBlocksRequiredAndAllowsOptional(t *testing.T) {
	request := scoringTaskWriteRequest{NotApplicable: true}
	if err := validateScoringTaskInput(request, workflowSnapshotItem{MaxScore: 100, Required: true}, true); err != errNotApplicableNotAllowed {
		t.Fatalf("required item err=%v", err)
	}
	if err := validateScoringTaskInput(request, workflowSnapshotItem{MaxScore: 100, Required: false}, true); err != errNotApplicableReasonRequired {
		t.Fatalf("optional item without reason err=%v", err)
	}
	request.Comment = "该岗位不涉及此职责"
	if err := validateScoringTaskInput(request, workflowSnapshotItem{MaxScore: 100, Required: false}, true); err != nil {
		t.Fatalf("optional item with reason should allow not-applicable: %v", err)
	}
}

func TestConfirmationGuardRequiresAllWeightedV2Work(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	evaluation := models.KPIEvaluation{ID: 1, Status: workflowStateAwaitingEmployeeConfirmation, CalculationVersion: canonicalCalculationVersion, TemplateSnapshotJSON: requiredWeightedWorkflowSnapshot(t)}
	if err := db.Create(&evaluation).Error; err != nil {
		t.Fatal(err)
	}
	if failure := guardEvaluationConfirmation(db, evaluation); failure == nil || failure.Code != "REQUIRED_SCORES_INCOMPLETE" {
		t.Fatalf("missing self/task work must block confirmation: %+v", failure)
	}

	zero := float64(0)
	if err := db.Create(&models.KPIScore{EvaluationID: evaluation.ID, ItemID: 10, SelfScore: &zero}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.EvaluationScoringTask{EvaluationID: evaluation.ID, ItemID: 10, ScorerUserID: 2, ScorerType: "PM", Required: true, Status: "submitted", Score: &zero, Version: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if failure := guardEvaluationConfirmation(db, evaluation); failure != nil {
		t.Fatalf("valid zero self/task scores should allow confirmation: %+v", failure)
	}

	evaluation.Status = workflowStateCompleted
	if failure := guardEvaluationConfirmation(db, evaluation); failure == nil || failure.Code != "WORKFLOW_CONFIRMATION_STATE" {
		t.Fatalf("completed evaluation must be terminal: %+v", failure)
	}
}

func TestWeightedConfirmationEndpointsUseTheCompletionGuard(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	employee, _, evaluation := seedConfirmationEvaluation(t, workflowStateAwaitingEmployeeConfirmation)
	models.DB.Model(&evaluation).Updates(map[string]any{
		"calculation_version":    canonicalCalculationVersion,
		"template_snapshot_json": requiredWeightedWorkflowSnapshot(t),
	})
	assignScope(t, employee.ID, "SELF")

	r := gin.New()
	r.POST("/evaluations/:id/confirm-online", func(c *gin.Context) { c.Set("user_id", employee.ID) }, ConfirmEvaluationOnline)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("POST", fmt.Sprintf("/evaluations/%d/confirm-online", evaluation.ID), nil))
	if response.Code != 409 || !strings.Contains(response.Body.String(), "REQUIRED_SCORES_INCOMPLETE") {
		t.Fatalf("weighted online confirmation must expose completion blocking details: status=%d body=%s", response.Code, response.Body.String())
	}

	r = gin.New()
	r.POST("/evaluations/:id/confirm-paper", func(c *gin.Context) { c.Set("user_id", employee.ID) }, ConfirmEvaluationPaper)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, newPaperConfirmationRequest(t, evaluation.ID))
	if response.Code != 409 || !strings.Contains(response.Body.String(), "REQUIRED_SCORES_INCOMPLETE") {
		t.Fatalf("weighted paper confirmation must use the same guard: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestConfirmationAndObjectionGuardsKeepLegacyAndNewStatesSeparate(t *testing.T) {
	legacy := models.KPIEvaluation{Status: "pending_confirm"}
	if failure := guardEvaluationConfirmation(nil, legacy); failure != nil {
		t.Fatalf("legacy pending_confirm should remain confirmable: %+v", failure)
	}
	legacy.Status = workflowStateAwaitingEmployeeConfirmation
	if failure := guardEvaluationConfirmation(nil, legacy); failure == nil {
		t.Fatal("legacy assessment must not use weighted-v2 confirmation state")
	}
	if !objectionSubmissionAllowed(models.KPIEvaluation{Status: "pending_confirm"}) || objectionSubmissionAllowed(models.KPIEvaluation{Status: workflowStateCompleted}) {
		t.Fatal("legacy objection state guard is incorrect")
	}
	weighted := models.KPIEvaluation{CalculationVersion: canonicalCalculationVersion, TemplateSnapshotJSON: requiredWeightedWorkflowSnapshot(t), Status: workflowStateAwaitingEmployeeConfirmation}
	if !objectionSubmissionAllowed(weighted) || objectionHandlingAllowed(weighted) {
		t.Fatal("weighted-v2 objection submission/handling states are incorrect")
	}
	weighted.Status = workflowStateRejected
	if !objectionHandlingAllowed(weighted) || objectionSubmissionAllowed(weighted) {
		t.Fatal("weighted-v2 rejection must be the only handling state")
	}
}

func TestWeightedConfirmationSuccessPersistsCanonicalEvidence(t *testing.T) {
	for _, method := range []string{"online", "paper"} {
		t.Run(method, func(t *testing.T) {
			setupConfirmationDB(t)
			employee, _, evaluation := seedConfirmationEvaluation(t, workflowStateAwaitingEmployeeConfirmation)
			if err := models.DB.Model(&evaluation).Updates(map[string]any{"calculation_version": canonicalCalculationVersion, "template_snapshot_json": requiredWeightedWorkflowSnapshot(t)}).Error; err != nil {
				t.Fatal(err)
			}
			zero := float64(0)
			if err := models.DB.Create(&models.KPIScore{EvaluationID: evaluation.ID, ItemID: 10, SelfScore: &zero}).Error; err != nil {
				t.Fatal(err)
			}
			if err := models.DB.Create(&models.EvaluationScoringTask{EvaluationID: evaluation.ID, ItemID: 10, ScorerUserID: 2, ScorerType: "PM", Required: true, Status: "submitted", Score: &zero, Version: 1}).Error; err != nil {
				t.Fatal(err)
			}
			assignScope(t, employee.ID, "SELF")
			t.Setenv("SIGNOFF_UPLOAD_DIR", t.TempDir())
			if method == "paper" {
				if _, _, err := persistCanonicalEvaluationResult(models.DB, evaluation.ID, employee.ID); err != nil {
					t.Fatal(err)
				}
				if _, _, err := GetOrCreateResultSnapshot(models.DB, evaluation.ID, employee.ID); err != nil {
					t.Fatal(err)
				}
			}
			r := gin.New()
			path := fmt.Sprintf("/evaluations/%d/confirm-%s", evaluation.ID, method)
			request := httptest.NewRequest("POST", path, nil)
			handler := gin.HandlerFunc(ConfirmEvaluationOnline)
			if method == "paper" {
				request = newPaperConfirmationRequest(t, evaluation.ID)
				handler = ConfirmEvaluationPaper
			}
			r.POST("/evaluations/:id/confirm-"+method, func(c *gin.Context) { c.Set("user_id", employee.ID) }, handler)
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			if response.Code != 200 {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			models.DB.Preload("Employee.Department").Preload("Template").First(&evaluation, evaluation.ID)
			if evaluation.Status != workflowStateCompleted || evaluation.TotalScore != 0 {
				t.Fatalf("evaluation=%+v", evaluation)
			}
			var evidence models.EvaluationConfirmation
			if err := models.DB.Where("evaluation_id = ?", evaluation.ID).First(&evidence).Error; err != nil {
				t.Fatal(err)
			}
			if evidence.SnapshotID == 0 || evidence.Method != method {
				t.Fatalf("evidence=%+v", evidence)
			}
			finalized, _, _, err := buildCanonicalResultSnapshot(evaluation)
			if err != nil {
				t.Fatal(err)
			}
			if finalized.TotalScore != evaluation.TotalScore || finalized.TotalScore != 0 {
				t.Fatalf("canonical export changed after confirmation: payload=%+v evaluation=%+v", finalized, evaluation)
			}
			var lockedTask models.EvaluationScoringTask
			if err := models.DB.Where("evaluation_id = ?", evaluation.ID).First(&lockedTask).Error; err != nil {
				t.Fatal(err)
			}
			if lockedTask.Status != "submitted" || lockedTask.LockedAt == nil || lockedTask.Version != 2 {
				t.Fatalf("confirmation must lock scoring task: %+v", lockedTask)
			}
			response = httptest.NewRecorder()
			if method == "paper" {
				request = newPaperConfirmationRequest(t, evaluation.ID)
			} else {
				request = httptest.NewRequest("POST", path, nil)
			}
			r.ServeHTTP(response, request)
			if response.Code != 409 {
				t.Fatalf("duplicate status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
