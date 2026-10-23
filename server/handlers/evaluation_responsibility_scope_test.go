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

func TestEvaluationResponsibilityDepartmentScopeDeniedOutsideDepartment(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	first := models.Department{Name: "研发一部"}
	second := models.Department{Name: "研发二部"}
	db.Create(&first)
	db.Create(&second)
	actor := createWorkflowFixtureEmployee(t, db, "部门管理员", "responsibility-scope-actor@test", "经理", first.ID)
	other := createWorkflowFixtureEmployee(t, db, "其他部门评分人", "responsibility-scope-other@test", "项目经理", second.ID)

	// The fallback scope for a legacy manager is DEPARTMENT.
	db.Model(&actor).Update("role", "manager")
	router := gin.New()
	router.POST("/evaluation-responsibilities", func(c *gin.Context) { c.Set("user_id", actor.ID); CreateEvaluationResponsibility(c) })
	body := fmt.Sprintf(`{"code":"PM","name":"项目经理","scope_type":"department","department_id":%d,"user_id":%d}`, second.ID, other.ID)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/evaluation-responsibilities", strings.NewReader(body)))
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
