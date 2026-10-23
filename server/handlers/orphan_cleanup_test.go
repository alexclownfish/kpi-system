package handlers

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
)

func TestCleanupOrphanScoringTasksDeletesOnlyExplicitOrphans(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	employee := createWorkflowFixtureEmployee(t, db, "孤儿测试员工", "orphan-cleanup@test", "engineer", 1)
	evaluation := createWorkflowFixtureEvaluation(t, db, employee.ID, 1, workflowStateScoring, `{}`)
	validTask := models.EvaluationScoringTask{EvaluationID: evaluation.ID, ItemID: 1, ScorerUserID: employee.ID, ScorerType: "PM", Required: true, Status: "pending", Version: 1}
	orphanTask := models.EvaluationScoringTask{EvaluationID: 999, ItemID: 1, ScorerUserID: employee.ID, ScorerType: "PM", Required: true, Status: "pending", Version: 1}
	if err := db.Create(&validTask).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&orphanTask).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/assessment-audits/cleanup-orphan-tasks", func(c *gin.Context) {
		c.Set("user_id", employee.ID)
		CleanupOrphanScoringTasks(c)
	})
	body := fmt.Sprintf(`{"task_ids":[%d,%d],"confirm":"DELETE_ORPHAN_SCORING_TASKS"}`, validTask.ID, orphanTask.ID)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/assessment-audits/cleanup-orphan-tasks", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), `"deleted":1`) {
		t.Fatalf("cleanup status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var validCount, orphanCount int64
	db.Model(&models.EvaluationScoringTask{}).Where("id = ?", validTask.ID).Count(&validCount)
	db.Model(&models.EvaluationScoringTask{}).Where("id = ?", orphanTask.ID).Count(&orphanCount)
	if validCount != 1 || orphanCount != 0 {
		t.Fatalf("valid=%d orphan=%d", validCount, orphanCount)
	}
	var audit models.AuditLog
	if err := db.Where("action = ?", "cleanup_orphan_scoring_tasks").First(&audit).Error; err != nil {
		t.Fatalf("missing cleanup audit: %v", err)
	}
}
