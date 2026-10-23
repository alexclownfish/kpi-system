package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
)

// TestScoringV2RejectsUnassignedAndStaleWrites exercises the two guards that
// are easiest to bypass from a browser: task ownership and optimistic
// concurrency. Both failures must leave the task untouched.
func TestScoringV2RejectsUnassignedAndStaleWrites(t *testing.T) {
	db, owner, _, outsider, task := setupEvaluationAccessDB(t)
	models.DB = db
	for _, tc := range []struct {
		name    string
		user    uint
		version int
		want    string
	}{
		{name: "unassigned scorer", user: outsider.ID, version: task.Version, want: "SCORING_TASK_FORBIDDEN"},
		{name: "stale version", user: owner.ID, version: task.Version - 1, want: "TASK_VERSION_CONFLICT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/scoring-tasks/:id/submit", func(c *gin.Context) {
				c.Set("user_id", tc.user)
				SubmitScoringTask(c)
			})
			request := httptest.NewRequest("POST", fmt.Sprintf("/scoring-tasks/%d/submit", task.ID), strings.NewReader(fmt.Sprintf(`{"score":80,"version":%d}`, tc.version)))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != 403 && tc.want == "SCORING_TASK_FORBIDDEN" {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if response.Code != 409 && tc.want == "TASK_VERSION_CONFLICT" {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), tc.want) {
				t.Fatalf("body=%s want code %s", response.Body.String(), tc.want)
			}
			var stored models.EvaluationScoringTask
			if err := db.First(&stored, task.ID).Error; err != nil {
				t.Fatal(err)
			}
			if stored.Status != "pending" || stored.Score != nil || stored.Version != task.Version {
				t.Fatalf("failed write mutated task: %+v", stored)
			}
		})
	}
}

func TestScoringV2DuplicateSubmitIsImmutable(t *testing.T) {
	db, owner, _, _, task := setupEvaluationAccessDB(t)
	models.DB = db
	item := workflowSnapshotItem{ID: task.ItemID, Name: "交付", MaxScore: 100, Required: true, ScorerTypes: []string{"PM"}, Aggregation: "average"}
	raw, err := buildWorkflowTemplateSnapshot(models.KPITemplate{ID: 1, Name: "权限模板", Period: "monthly", Version: 1, Items: []models.KPIItem{{ID: item.ID, Name: item.Name, MaxScore: 100, Required: true, ScorerTypes: item.ScorerTypes}}}, []workflowScoringAssignment{{ItemID: item.ID, ScorerType: "PM", UserID: owner.ID, UserName: owner.Name, SourceRule: "evaluation_responsibility"}})
	if err != nil {
		t.Fatal(err)
	}
	db.Model(&models.KPIEvaluation{}).Where("id = ?", task.EvaluationID).Updates(map[string]any{"template_snapshot_json": raw, "calculation_version": canonicalCalculationVersion})
	router := gin.New()
	router.POST("/scoring-tasks/:id/submit", func(c *gin.Context) { c.Set("user_id", owner.ID); SubmitScoringTask(c) })
	path := fmt.Sprintf("/scoring-tasks/%d/submit", task.ID)
	first := httptest.NewRequest("POST", path, strings.NewReader(fmt.Sprintf(`{"score":80,"version":%d}`, task.Version)))
	first.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, first)
	if response.Code != 200 {
		t.Fatalf("first submit status=%d body=%s", response.Code, response.Body.String())
	}
	var stored models.EvaluationScoringTask
	if err := db.First(&stored, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != "submitted" || stored.Score == nil || *stored.Score != 80 {
		t.Fatalf("first submit=%+v", stored)
	}
	second := httptest.NewRequest("POST", path, strings.NewReader(fmt.Sprintf(`{"score":90,"version":%d}`, stored.Version)))
	second.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, second)
	if response.Code != 409 || !strings.Contains(response.Body.String(), "TASK_LOCKED") {
		t.Fatalf("duplicate status=%d body=%s", response.Code, response.Body.String())
	}
	db.First(&stored, task.ID)
	if stored.Score == nil || *stored.Score != 80 || stored.Version != task.Version+1 {
		t.Fatalf("duplicate changed submitted task=%+v", stored)
	}
}

func TestScoringV2OptionalNotApplicableRetainsReason(t *testing.T) {
	db, owner, _, _, task := setupEvaluationAccessDB(t)
	models.DB = db
	item := workflowSnapshotItem{ID: task.ItemID, Name: "岗位不适用项", MaxScore: 100, Required: false, ScorerTypes: []string{"PM"}, Aggregation: "average"}
	raw, err := buildWorkflowTemplateSnapshot(models.KPITemplate{ID: 1, Name: "权限模板", Period: "monthly", Version: 1, Items: []models.KPIItem{{ID: item.ID, Name: item.Name, MaxScore: item.MaxScore, Required: false, ScorerTypes: item.ScorerTypes}}}, []workflowScoringAssignment{{ItemID: item.ID, ScorerType: "PM", UserID: owner.ID, UserName: owner.Name, SourceRule: "evaluation_responsibility"}})
	if err != nil {
		t.Fatal(err)
	}
	db.Model(&models.KPIEvaluation{}).Where("id = ?", task.EvaluationID).Updates(map[string]any{"template_snapshot_json": raw, "calculation_version": canonicalCalculationVersion})
	db.Model(&models.EvaluationScoringTask{}).Where("id = ?", task.ID).Update("required", false)
	router := gin.New()
	router.POST("/scoring-tasks/:id/submit", func(c *gin.Context) { c.Set("user_id", owner.ID); SubmitScoringTask(c) })
	body := `{"not_applicable":true,"comment":"该岗位不涉及此职责","version":2}`
	request := httptest.NewRequest("POST", fmt.Sprintf("/scoring-tasks/%d/submit", task.ID), strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var stored models.EvaluationScoringTask
	if err := db.First(&stored, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != "not_applicable" || !stored.NotApplicable || stored.Comment != "该岗位不涉及此职责" {
		t.Fatalf("not applicable reason not retained: %+v", stored)
	}
	var envelope struct {
		Data struct {
			Task map[string]any `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Task["comment"] != "该岗位不涉及此职责" {
		t.Fatalf("response omitted reason: %v", envelope.Data.Task)
	}
}

func TestScoringV2OptionalNotApplicableRequiresReason(t *testing.T) {
	db, owner, _, _, task := setupEvaluationAccessDB(t)
	models.DB = db
	item := workflowSnapshotItem{ID: task.ItemID, Name: "岗位不适用项", MaxScore: 100, Required: false, ScorerTypes: []string{"PM"}, Aggregation: "average"}
	raw, err := buildWorkflowTemplateSnapshot(models.KPITemplate{ID: 1, Name: "权限模板", Period: "monthly", Version: 1, Items: []models.KPIItem{{ID: item.ID, Name: item.Name, MaxScore: item.MaxScore, Required: false, ScorerTypes: item.ScorerTypes}}}, []workflowScoringAssignment{{ItemID: item.ID, ScorerType: "PM", UserID: owner.ID, UserName: owner.Name, SourceRule: "evaluation_responsibility"}})
	if err != nil {
		t.Fatal(err)
	}
	db.Model(&models.KPIEvaluation{}).Where("id = ?", task.EvaluationID).Updates(map[string]any{"template_snapshot_json": raw, "calculation_version": canonicalCalculationVersion})
	db.Model(&models.EvaluationScoringTask{}).Where("id = ?", task.ID).Update("required", false)
	router := gin.New()
	router.POST("/scoring-tasks/:id/submit", func(c *gin.Context) { c.Set("user_id", owner.ID); SubmitScoringTask(c) })
	request := httptest.NewRequest("POST", fmt.Sprintf("/scoring-tasks/%d/submit", task.ID), strings.NewReader(`{"not_applicable":true,"version":2}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 422 || !strings.Contains(response.Body.String(), "NOT_APPLICABLE_REASON_REQUIRED") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var stored models.EvaluationScoringTask
	if err := db.First(&stored, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != "pending" || stored.NotApplicable || stored.Comment != "" {
		t.Fatalf("invalid N/A submit mutated task: %+v", stored)
	}
}

func TestScoringV2RejectsNonFiniteScores(t *testing.T) {
	item := workflowSnapshotItem{MaxScore: 100, Required: true}
	for _, score := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := validateScoringTaskInput(scoringTaskWriteRequest{Score: &score}, item, true); err != errScoreOutOfRange {
			t.Fatalf("score=%v err=%v, want %v", score, err, errScoreOutOfRange)
		}
	}
}
