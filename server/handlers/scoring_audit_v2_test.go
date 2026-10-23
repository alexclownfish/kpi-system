package handlers

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
)

func TestScoringV2AuditDetailsExcludeScoreAndCommentContent(t *testing.T) {
	db, owner, _, _, task := setupEvaluationAccessDB(t)
	if err := db.AutoMigrate(&models.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	models.DB = db
	item := workflowSnapshotItem{ID: task.ItemID, Name: "交付", MaxScore: 100, Required: true, ScorerTypes: []string{"PM"}, Aggregation: "average"}
	raw, err := buildWorkflowTemplateSnapshot(models.KPITemplate{ID: 1, Name: "权限模板", Period: "monthly", Version: 1, Items: []models.KPIItem{{ID: item.ID, Name: item.Name, MaxScore: 100, Required: true, ScorerTypes: item.ScorerTypes}}}, []workflowScoringAssignment{{ItemID: item.ID, ScorerType: "PM", UserID: owner.ID, UserName: owner.Name, SourceRule: "evaluation_responsibility"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.KPIEvaluation{}).Where("id = ?", task.EvaluationID).Updates(map[string]any{"template_snapshot_json": raw, "calculation_version": canonicalCalculationVersion}).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/scoring-tasks/:id/submit", func(c *gin.Context) { c.Set("user_id", owner.ID); SubmitScoringTask(c) })
	secretComment := "员工隐私评论 PASSWORD=should-not-be-audit-value"
	request := httptest.NewRequest("POST", fmt.Sprintf("/scoring-tasks/%d/submit", task.ID), strings.NewReader(fmt.Sprintf(`{"score":80,"comment":%q,"version":%d}`, secretComment, task.Version)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var audit models.AuditLog
	if err := db.Where("action = ? AND resource_id = ?", "score_submitted", fmt.Sprint(task.ID)).First(&audit).Error; err != nil {
		t.Fatalf("missing score audit: %v", err)
	}
	if strings.Contains(audit.Details, secretComment) || strings.Contains(audit.Details, "PASSWORD") || strings.Contains(audit.Details, "80") {
		t.Fatalf("score audit contains sensitive input: %q", audit.Details)
	}
	for _, expected := range []string{"evaluation_id=", "item_id=", "scorer_type="} {
		if !strings.Contains(audit.Details, expected) {
			t.Fatalf("score audit omitted operational field %q: %q", expected, audit.Details)
		}
	}
}

func TestWorkflowV2GateAuditContainsOnlyBlockingCount(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	owner := createWorkflowFixtureEmployee(t, db, "被考核人", "audit-gate@test", "engineer", 1)
	evaluation := createWorkflowFixtureEvaluation(t, db, owner.ID, 1, workflowStateScoring, requiredWeightedWorkflowSnapshot(t))
	router := gin.New()
	router.POST("/evaluations/:id/workflow/advance", func(c *gin.Context) { c.Set("user_id", owner.ID); AdvanceEvaluationWorkflow(c) })
	request := httptest.NewRequest("POST", fmt.Sprintf("/evaluations/%d/workflow/advance", evaluation.ID), strings.NewReader(`{"expected_state":"scoring"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 409 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var audit models.AuditLog
	if err := db.Where("action = ? AND resource_id = ?", "confirmation_gate_rejected", fmt.Sprint(evaluation.ID)).First(&audit).Error; err != nil {
		t.Fatalf("missing gate audit: %v", err)
	}
	if audit.Details == "" || strings.Contains(audit.Details, "score") || strings.Contains(audit.Details, "comment") || strings.Contains(audit.Details, "password") {
		t.Fatalf("gate audit contains forbidden detail: %q", audit.Details)
	}
	if !strings.HasPrefix(audit.Details, "blocking_tasks=") {
		t.Fatalf("unexpected gate audit details: %q", audit.Details)
	}
}
