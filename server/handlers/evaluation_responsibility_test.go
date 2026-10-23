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

func TestEvaluationResponsibilityCRUDAndTaskImmutability(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	department := models.Department{Name: "研发部"}
	db.Create(&department)
	actor := createWorkflowFixtureEmployee(t, db, "HR", "responsibility-hr@test", "HR", department.ID)
	db.Model(&actor).Update("role", "hr")
	scorer := createWorkflowFixtureEmployee(t, db, "项目经理", "responsibility-pm@test", "项目经理", department.ID)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/evaluation-responsibilities", func(c *gin.Context) { c.Set("user_id", actor.ID); CreateEvaluationResponsibility(c) })
	router.DELETE("/evaluation-responsibilities/:id", func(c *gin.Context) { c.Set("user_id", actor.ID); DeactivateEvaluationResponsibility(c) })
	body := fmt.Sprintf(`{"code":"PM","name":"项目经理","scope_type":"department","department_id":%d,"user_id":%d,"active":true}`, department.ID, scorer.ID)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/evaluation-responsibilities", strings.NewReader(body)))
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	var responsibility models.EvaluationResponsibility
	if err := db.Where("code = ?", "PM").First(&responsibility).Error; err != nil {
		t.Fatal(err)
	}

	evaluation := models.KPIEvaluation{EmployeeID: scorer.ID, TemplateID: 1, Period: "monthly", Year: 2026, Status: workflowStateScoring}
	db.Create(&evaluation)
	task := models.EvaluationScoringTask{EvaluationID: evaluation.ID, ItemID: 10, ScorerUserID: scorer.ID, ScorerType: "PM", Required: true, Status: "pending", Version: 1}
	db.Create(&task)

	duplicate := httptest.NewRecorder()
	router.ServeHTTP(duplicate, httptest.NewRequest(http.MethodPost, "/evaluation-responsibilities", strings.NewReader(body)))
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}

	deactivated := httptest.NewRecorder()
	router.ServeHTTP(deactivated, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/evaluation-responsibilities/%d", responsibility.ID), nil))
	if deactivated.Code != http.StatusOK {
		t.Fatalf("deactivate status=%d body=%s", deactivated.Code, deactivated.Body.String())
	}
	var storedTask models.EvaluationScoringTask
	if err := db.First(&storedTask, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedTask.ScorerUserID != scorer.ID || storedTask.Status != "pending" {
		t.Fatalf("existing task changed after responsibility deactivation: %+v", storedTask)
	}
	var auditCount int64
	db.Model(&models.AuditLog{}).Where("resource = ?", "evaluation_responsibility").Count(&auditCount)
	if auditCount < 2 {
		t.Fatalf("expected create/deactivate audit logs, got %d", auditCount)
	}
}

func TestEvaluationResponsibilityRejectsInactiveUser(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	department := models.Department{Name: "研发部"}
	db.Create(&department)
	actor := createWorkflowFixtureEmployee(t, db, "HR", "responsibility-inactive-hr@test", "HR", department.ID)
	db.Model(&actor).Update("role", "hr")
	scorer := createWorkflowFixtureEmployee(t, db, "离职员工", "responsibility-inactive@test", "项目经理", department.ID)
	db.Model(&scorer).Update("is_active", false)

	router := gin.New()
	router.POST("/evaluation-responsibilities", func(c *gin.Context) { c.Set("user_id", actor.ID); CreateEvaluationResponsibility(c) })
	body := fmt.Sprintf(`{"code":"PM","name":"项目经理","scope_type":"department","department_id":%d,"user_id":%d}`, department.ID, scorer.ID)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/evaluation-responsibilities", strings.NewReader(body)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
