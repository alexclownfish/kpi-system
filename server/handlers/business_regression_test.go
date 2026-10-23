package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
)

func TestSelfScoreOwnerStateAndSnapshotRange(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	owner := createWorkflowFixtureEmployee(t, db, "owner", "owner@test", "head", 1)
	outsider := createWorkflowFixtureEmployee(t, db, "outsider", "outsider@test", "engineer", 2)
	item := models.KPIItem{Name: "live changed", MaxScore: 999}
	db.Create(&item)
	raw := fmt.Sprintf(`{"template":{"id":1,"name":"frozen","version":1,"period":"monthly"},"calculation_version":"weighted-v2","dimensions":[{"id":1,"name":"quality","weight":100,"items":[{"id":%d,"name":"frozen item","max_score":100,"item_weight":100,"composite_weight":100,"required":true}]}]}`, item.ID)
	evaluation := createWorkflowFixtureEvaluation(t, db, owner.ID, 1, "self_evaluating", raw)
	score := models.KPIScore{EvaluationID: evaluation.ID, ItemID: item.ID}
	db.Create(&score)
	for _, tc := range []struct {
		name        string
		user        uint
		state, body string
		status      int
	}{
		{"outsider", outsider.ID, "self_evaluating", `{"self_score":80}`, 403},
		{"too high", owner.ID, "self_evaluating", `{"self_score":101}`, 400},
		{"negative", owner.ID, "self_evaluating", `{"self_score":-1}`, 400},
		{"zero without manager", owner.ID, "self_evaluating", `{"self_score":0}`, 200},
		{"locked scoring", owner.ID, "scoring", `{"self_score":90}`, 409},
		{"locked cancelled", owner.ID, "cancelled", `{"self_score":90}`, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db.Model(&evaluation).Update("status", tc.state)
			r := gin.New()
			r.PUT("/scores/:id", func(c *gin.Context) { c.Set("user_id", tc.user) }, UpdateSelfScore)
			request := httptest.NewRequest("PUT", fmt.Sprintf("/scores/%d", score.ID), strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAssignedPMEvaluationScopeDoesNotLeakUnassigned(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	pm := createWorkflowFixtureEmployee(t, db, "pm", "pm@test", "engineer", 1)
	owner := createWorkflowFixtureEmployee(t, db, "owner", "owner@test", "engineer", 2)
	assigned := createWorkflowFixtureEvaluation(t, db, owner.ID, 1, "scoring", "")
	unassigned := createWorkflowFixtureEvaluation(t, db, owner.ID, 1, "scoring", "")
	createWorkflowFixtureTask(t, db, assigned.ID, 1, pm.ID, true, nil, "pending")
	var evaluations []models.KPIEvaluation
	if err := ApplyEvaluationScope(db.Model(&models.KPIEvaluation{}), pm.ID).Find(&evaluations).Error; err != nil {
		t.Fatal(err)
	}
	if len(evaluations) != 1 || evaluations[0].ID != assigned.ID {
		t.Fatalf("scope=%+v", evaluations)
	}
	if CanAccessEvaluation(pm.ID, unassigned.ID) {
		t.Fatal("scope leaked unassigned assessment")
	}
}

func TestCommentsEnforceAssessmentAndAuthorScope(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	db.AutoMigrate(&models.EvaluationComment{})
	owner := createWorkflowFixtureEmployee(t, db, "owner", "comment-owner@test", "engineer", 1)
	outsider := createWorkflowFixtureEmployee(t, db, "outsider", "comment-outsider@test", "engineer", 2)
	evaluation := createWorkflowFixtureEvaluation(t, db, owner.ID, 1, "scoring", "")
	comment := models.EvaluationComment{EvaluationID: evaluation.ID, UserID: owner.ID, Content: "private business data"}
	db.Create(&comment)
	for _, user := range []uint{outsider.ID, owner.ID} {
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("user_id", user) })
		r.GET("/evaluations/:id/comments", GetEvaluationComments)
		r.POST("/evaluations/:id/comments", CreateEvaluationComment)
		r.PUT("/evaluations/:id/comments/:comment_id", UpdateEvaluationComment)
		r.DELETE("/evaluations/:id/comments/:comment_id", DeleteEvaluationComment)
		for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
			url := fmt.Sprintf("/evaluations/%d/comments", evaluation.ID)
			if method == "PUT" || method == "DELETE" {
				url += fmt.Sprintf("/%d", comment.ID)
			}
			request := httptest.NewRequest(method, url, strings.NewReader(`{"content":"updated"}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			if user == outsider.ID && response.Code != 403 {
				t.Fatalf("outsider %s status=%d %s", method, response.Code, response.Body.String())
			}
			if user == owner.ID && response.Code >= 400 {
				t.Fatalf("owner %s status=%d %s", method, response.Code, response.Body.String())
			}
		}
	}
}

func TestTrendPeriodsNeverOverflowMonthEnd(t *testing.T) {
	periods := trendPeriods(time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC), "monthly")
	if len(periods) != 12 || periods[10] != "2026-02" || periods[11] != "2026-03" {
		t.Fatalf("periods=%v", periods)
	}
	seen := map[string]bool{}
	for _, period := range periods {
		if seen[period] {
			t.Fatalf("duplicate period %s", period)
		}
		seen[period] = true
	}
}

func TestEmployeeStatisticsUsesCanonicalItems(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	owner := createWorkflowFixtureEmployee(t, db, "owner", "stats-owner@test", "engineer", 1)
	evaluation := createWorkflowFixtureEvaluation(t, db, owner.ID, 1, "completed", "")
	value := 80.0
	raw, _ := json.Marshal(canonicalEvaluationResult{TotalScore: 54, Items: []canonicalItemResult{{ItemID: 1, Name: "frozen item", MaxScore: 100, AggregatedScore: &value}}})
	db.Create(&models.EvaluationCalculationResult{EvaluationID: evaluation.ID, Version: 1, TotalScore: 54, ResultJSON: string(raw)})
	r := gin.New()
	r.GET("/statistics/:id", func(c *gin.Context) { c.Set("user_id", owner.ID) }, GetEmployeeStats)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("GET", fmt.Sprintf("/statistics/%d", owner.ID), nil))
	var body struct {
		Data struct {
			LatestScore  float64 `json:"latest_score"`
			KPIBreakdown []struct {
				AverageScore float64 `json:"average_score"`
			} `json:"kpi_breakdown"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || body.Data.LatestScore != 54 || len(body.Data.KPIBreakdown) != 1 || body.Data.KPIBreakdown[0].AverageScore != 80 {
		t.Fatalf("body=%s", response.Body.String())
	}
}

func TestAssessmentCreationRejectsForgedAndMismatchedInputs(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	t.Setenv("EVALUATION_WORKFLOW_MODE", "enabled")
	owner := createWorkflowFixtureEmployee(t, db, "owner", "create-owner@test", "engineer", 1)
	dept := uint(1)
	template := models.KPITemplate{Name: "head only", DepartmentID: &dept, PositionType: "head", Period: "monthly", Status: "published", IsActive: true}
	db.Create(&template)
	for _, addition := range []string{`"status":"completed"`, `"total_score":999`, `"month":13`, `"year":0`, `"period":"unknown"`, `"template_id":1`} {
		// Later duplicate keys override the valid defaults intentionally.
		body := fmt.Sprintf(`{"employee_id":%d,"period":"monthly","year":2026,"month":9,%s}`, owner.ID, addition)
		r := gin.New()
		r.POST("/evaluations", func(c *gin.Context) { c.Set("user_id", owner.ID) }, CreateEvaluation)
		request := httptest.NewRequest("POST", "/evaluations", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		if response.Code != 400 {
			t.Fatalf("addition=%s status=%d body=%s", addition, response.Code, response.Body.String())
		}
	}
	var count int64
	db.Model(&models.KPIEvaluation{}).Count(&count)
	if count != 0 {
		t.Fatal("invalid creation persisted records")
	}
}

func TestAutomaticTemplateMatchingFreezesConfiguredExportLayout(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	departmentID := uint(1)
	template := models.KPITemplate{Name: "source", DepartmentID: &departmentID, PositionType: "head", Period: "monthly", Status: "published", IsActive: true, Version: 1, ExportLayoutJSON: `{"version":1,"title":"custom source title","orientation":"portrait","columns":["item_name","final_score"],"signature_labels":["PM签字","CTO签字"]}`}
	db.Create(&template)
	matched, err := findPublishedTemplate(db, models.Employee{DepartmentID: 1, Position: "head"}, "monthly")
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := loadCalculationTemplate(db, template.ID)
	if err != nil {
		t.Fatal(err)
	}
	if matched.ExportLayout.Title != explicit.ExportLayout.Title || matched.ExportLayout.Title != "custom source title" {
		t.Fatalf("automatic=%+v explicit=%+v", matched.ExportLayout, explicit.ExportLayout)
	}
	raw, err := buildWorkflowTemplateSnapshot(matched, nil)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot workflowEvaluationSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.ExportLayout.Title != "custom source title" || len(snapshot.ExportLayout.SignatureLabels) != 2 {
		t.Fatalf("snapshot=%+v", snapshot.ExportLayout)
	}
}

func TestCanonicalContributionUsesDecimalHalfUp(t *testing.T) {
	for _, tc := range []struct{ score, weight, expected float64 }{
		{63, 7.5, 4.73}, {1, .5, .01}, {0, 7.5, 0}, {99, 7.5, 7.43},
	} {
		result, err := calculateCanonicalResult(parsedEvaluationSnapshot{Items: []workflowSnapshotItem{{ID: 1, Name: "source", MaxScore: 100, CompositeWeight: tc.weight, Required: true}}}, []models.EvaluationScoringTask{{ItemID: 1, Status: "submitted", Score: &tc.score}})
		if err != nil {
			t.Fatal(err)
		}
		if result.TotalScore != tc.expected {
			t.Fatalf("score=%v weight=%v result=%v want=%v", tc.score, tc.weight, result.TotalScore, tc.expected)
		}
	}
}

func TestCanonicalMultiScorerContributionDoesNotRoundAverageEarly(t *testing.T) {
	a, b := 60.0, 66.125
	result, err := calculateCanonicalResult(parsedEvaluationSnapshot{Items: []workflowSnapshotItem{{ID: 1, Name: "average", MaxScore: 100, CompositeWeight: 8, Required: true}}}, []models.EvaluationScoringTask{
		{ItemID: 1, Status: "submitted", Score: &a}, {ItemID: 1, Status: "submitted", Score: &b},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalScore != 5.05 || *result.Items[0].AggregatedScore != 63.06 {
		t.Fatalf("result=%+v", result)
	}
	for _, invalid := range []float64{math.NaN(), math.Inf(1)} {
		_, err := calculateCanonicalResult(parsedEvaluationSnapshot{Items: []workflowSnapshotItem{{ID: 1, Name: "invalid", MaxScore: 100, CompositeWeight: 100, Required: true}}}, []models.EvaluationScoringTask{{ItemID: 1, Status: "submitted", Score: &invalid}})
		if err == nil {
			t.Fatal("accepted non-finite score")
		}
	}
}

func TestAssignedListPendingCountAndZeroAverageAgree(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	pm := createWorkflowFixtureEmployee(t, db, "pm", "pending-pm@test", "engineer", 1)
	owner := createWorkflowFixtureEmployee(t, db, "owner", "pending-owner@test", "engineer", 2)
	assigned := createWorkflowFixtureEvaluation(t, db, owner.ID, 1, "scoring", "")
	createWorkflowFixtureTask(t, db, assigned.ID, 1, pm.ID, true, nil, "pending")
	done := createWorkflowFixtureEvaluation(t, db, owner.ID, 1, "completed", "")
	zero := 0.0
	createWorkflowFixtureTask(t, db, done.ID, 1, pm.ID, true, &zero, "submitted")
	hundred := createWorkflowFixtureEvaluation(t, db, owner.ID, 1, "completed", "")
	db.Model(&hundred).Update("total_score", 100)
	createWorkflowFixtureTask(t, db, hundred.ID, 1, pm.ID, true, &zero, "submitted")
	createWorkflowFixtureEvaluation(t, db, owner.ID, 1, "scoring", "")
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", pm.ID) })
	r.GET("/evaluations", GetEvaluations)
	r.GET("/pending/:employeeId", GetPendingEvaluations)
	r.GET("/pending-count", GetPendingCountEvaluations)
	for _, path := range []string{"/evaluations?assigned_to_me=true", fmt.Sprintf("/pending/%d", pm.ID)} {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		var body struct {
			Data  []models.KPIEvaluation `json:"data"`
			Total int                    `json:"total"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if response.Code != 200 || body.Total != 1 || len(body.Data) != 1 || body.Data[0].ID != assigned.ID {
			t.Fatalf("%s: %s", path, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("GET", "/pending-count", nil))
	if response.Body.String() != `{"count":1}` {
		t.Fatalf("pending count=%s", response.Body.String())
	}
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("GET", "/evaluations", nil))
	var body struct {
		Total int `json:"total"`
		Stats struct {
			Avg float64 `json:"avgScore"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 3 || body.Stats.Avg != 50 {
		t.Fatalf("scoped average=%s", response.Body.String())
	}
}

func TestHRCanOnboardBuiltInRolesButCannotGrantElevatedRole(t *testing.T) {
	db := newEvaluationWorkflowTestDB(t)
	models.DB = db
	hr := createWorkflowFixtureEmployee(t, db, "hr", "hr-grant@test", "HR", 1)
	hr.Role = "hr"
	db.Model(&hr).Update("role", "hr")
	actorRole := models.Role{Code: "hr_admin", IsSystem: true}
	db.Create(&actorRole)
	actorScope := models.DataScope{Code: "ALL"}
	db.Create(&actorScope)
	db.Create(&models.RoleDataScope{RoleID: actorRole.ID, DataScopeID: actorScope.ID})
	db.Create(&models.UserRole{UserID: hr.ID, RoleID: actorRole.ID})
	assign := models.Permission{Code: "employee:assign_role", Name: "分配角色"}
	submit := models.Permission{Code: "assessment:submit", Name: "提交考核"}
	elevated := models.Permission{Code: "system:edit", Name: "系统管理"}
	for _, permission := range []*models.Permission{&assign, &submit, &elevated} {
		db.Create(permission)
	}
	db.Create(&models.RolePermission{RoleID: actorRole.ID, PermissionID: assign.ID})
	for _, code := range []string{"employee", "department_manager", "custom-elevated", "super_admin"} {
		role := models.Role{Code: code, Name: code, IsSystem: code != "custom-elevated"}
		db.Create(&role)
		db.Create(&models.RolePermission{RoleID: role.ID, PermissionID: submit.ID})
		if code == "super_admin" || code == "custom-elevated" {
			db.Create(&models.RolePermission{RoleID: role.ID, PermissionID: elevated.ID})
		}
		scope := models.DataScope{Code: "scope-" + code}
		if code == "employee" {
			scope.Code = "SELF"
		} else if code == "department_manager" {
			scope.Code = "DEPARTMENT"
		} else {
			scope.Code = "ALL"
		}
		db.Where("code = ?", scope.Code).FirstOrCreate(&scope)
		db.Create(&models.RoleDataScope{RoleID: role.ID, DataScopeID: scope.ID})
		err := CanGrantExistingRole(db, hr.ID, role)
		if (code == "employee" || code == "department_manager") && err != nil {
			t.Fatalf("%s grant failed: %v", code, err)
		}
		if (code == "super_admin" || code == "custom-elevated") && err == nil {
			t.Fatalf("allowed elevated %s", code)
		}
	}
}
