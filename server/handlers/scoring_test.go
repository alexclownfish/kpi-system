package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
)

func TestSubmitScoringTaskAcceptsZeroScore(t *testing.T) {
	db, owner, _, _, task := setupEvaluationAccessDB(t)
	models.DB = db
	item := workflowSnapshotItem{ID: task.ItemID, Name: "交付", MaxScore: 100, Required: true, ScorerTypes: []string{"PM"}, Aggregation: "average"}
	raw, err := buildWorkflowTemplateSnapshot(models.KPITemplate{ID: 1, Name: "权限模板", Period: "monthly", Version: 1, Items: []models.KPIItem{{ID: item.ID, Name: item.Name, MaxScore: item.MaxScore, Required: true, ScorerTypes: item.ScorerTypes}}}, []workflowScoringAssignment{{ItemID: item.ID, ScorerType: "PM", UserID: owner.ID, UserName: owner.Name, SourceRule: "evaluation_responsibility"}})
	if err != nil {
		t.Fatal(err)
	}
	db.Model(&models.KPIEvaluation{}).Where("id = ?", task.EvaluationID).Update("template_snapshot_json", raw)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/scoring-tasks/:id/submit", func(c *gin.Context) {
		c.Set("user_id", owner.ID)
		SubmitScoringTask(c)
	})
	record := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/scoring-tasks/"+formatUint(task.ID)+"/submit", strings.NewReader(`{"score":0,"comment":"有效零分","not_applicable":false,"version":2}`))
	router.ServeHTTP(record, request)
	if record.Code != 200 {
		t.Fatalf("status=%d body=%s", record.Code, record.Body.String())
	}
	var envelope struct {
		Data struct {
			Task struct {
				Score  *float64 `json:"score"`
				Status string   `json:"status"`
			} `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(record.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Task.Score == nil || *envelope.Data.Task.Score != 0 || envelope.Data.Task.Status != "submitted" {
		t.Fatalf("unexpected response: %+v", envelope.Data.Task)
	}
}

func TestSubmitScoringTaskRejectsAfterConfirmationPhase(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		required bool
		submit   bool
	}{
		{name: "awaiting required submit", status: workflowStateAwaitingEmployeeConfirmation, required: true, submit: true},
		{name: "awaiting optional draft", status: workflowStateAwaitingEmployeeConfirmation, required: false, submit: false},
		{name: "completed submit", status: workflowStateCompleted, required: true, submit: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, owner, _, _, task := setupEvaluationAccessDB(t)
			models.DB = db
			item := workflowSnapshotItem{ID: task.ItemID, Name: "交付", MaxScore: 100, Required: test.required, ScorerTypes: []string{"PM"}, Aggregation: "average"}
			raw, err := buildWorkflowTemplateSnapshot(models.KPITemplate{ID: 1, Name: "权限模板", Period: "monthly", Version: 1, Items: []models.KPIItem{{ID: item.ID, Name: item.Name, MaxScore: item.MaxScore, Required: test.required, ScorerTypes: item.ScorerTypes}}}, []workflowScoringAssignment{{ItemID: item.ID, ScorerType: "PM", UserID: owner.ID, UserName: owner.Name, SourceRule: "evaluation_responsibility"}})
			if err != nil {
				t.Fatal(err)
			}
			db.Model(&models.KPIEvaluation{}).Where("id = ?", task.EvaluationID).Updates(map[string]any{
				"status": test.status, "calculation_version": canonicalCalculationVersion, "template_snapshot_json": raw,
			})
			db.Model(&models.EvaluationScoringTask{}).Where("id = ?", task.ID).Update("required", test.required)

			gin.SetMode(gin.TestMode)
			router := gin.New()
			handler := SubmitScoringTask
			method := "POST"
			route := "/scoring-tasks/:id/submit"
			path := "/scoring-tasks/" + formatUint(task.ID) + "/submit"
			if !test.submit {
				handler = UpdateScoringTask
				method = "PUT"
				route = "/scoring-tasks/:id"
				path = "/scoring-tasks/" + formatUint(task.ID)
			}
			router.Handle(method, route, func(c *gin.Context) {
				c.Set("user_id", owner.ID)
				handler(c)
			})
			record := httptest.NewRecorder()
			request := httptest.NewRequest(method, path, strings.NewReader(`{"score":80,"version":2}`))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(record, request)
			if record.Code != 409 || !strings.Contains(record.Body.String(), `"SCORING_WORKFLOW_STATE"`) {
				t.Fatalf("status=%d body=%s", record.Code, record.Body.String())
			}
			var stored models.EvaluationScoringTask
			if err := db.First(&stored, task.ID).Error; err != nil {
				t.Fatal(err)
			}
			if stored.Status != "pending" || stored.Score != nil {
				t.Fatalf("scoring task changed after terminal write: %+v", stored)
			}
		})
	}
}

func TestScoringTaskWorkflowGuardRejectsLegacyTerminalStates(t *testing.T) {
	for _, status := range []string{"pending_confirm", workflowStateCompleted, workflowStateRejected, workflowStateCancelled} {
		failure := guardScoringTaskWorkflowState(models.KPIEvaluation{Status: status})
		if failure == nil || failure.Code != "SCORING_WORKFLOW_STATE" {
			t.Fatalf("status=%s failure=%+v", status, failure)
		}
	}
	if failure := guardScoringTaskWorkflowState(models.KPIEvaluation{Status: "manager_evaluated"}); failure != nil {
		t.Fatalf("legacy pre-confirmation state should remain writable: %+v", failure)
	}
}

func TestValidateScoringTaskInputRejectsRequiredNotApplicable(t *testing.T) {
	err := validateScoringTaskInput(scoringTaskWriteRequest{NotApplicable: true}, workflowSnapshotItem{MaxScore: 100, Required: true}, true)
	if err != errNotApplicableNotAllowed {
		t.Fatalf("err=%v", err)
	}
}

func TestScoringTaskValidationRejectsInvalidSubmissions(t *testing.T) {
	tests := []struct {
		name string
		body scoringTaskWriteRequest
		item workflowSnapshotItem
		want error
	}{
		{name: "above max", body: scoringTaskWriteRequest{Score: floatPointer(101)}, item: workflowSnapshotItem{MaxScore: 100}, want: errScoreOutOfRange},
		{name: "below zero", body: scoringTaskWriteRequest{Score: floatPointer(-1)}, item: workflowSnapshotItem{MaxScore: 100}, want: errScoreOutOfRange},
		{name: "missing submit value", body: scoringTaskWriteRequest{}, item: workflowSnapshotItem{MaxScore: 100}, want: errScoreRequired},
		{name: "zero accepted", body: scoringTaskWriteRequest{Score: floatPointer(0)}, item: workflowSnapshotItem{MaxScore: 100}, want: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateScoringTaskInput(test.body, test.item, true)
			if err != test.want {
				t.Fatalf("err=%v want=%v", err, test.want)
			}
		})
	}
}

func TestGetEvaluationScoringTasksReturnsOnlyAssignedTasks(t *testing.T) {
	db, owner, _, _, task := setupEvaluationAccessDB(t)
	models.DB = db
	raw, err := buildWorkflowTemplateSnapshot(models.KPITemplate{ID: 1, Name: "权限模板", Period: "monthly", Version: 1, Items: []models.KPIItem{{ID: task.ItemID, Name: "交付", MaxScore: 100, Required: true, ScorerTypes: []string{"PM"}}}}, []workflowScoringAssignment{{ItemID: task.ItemID, ScorerType: "PM", UserID: owner.ID, UserName: owner.Name, SourceRule: "evaluation_responsibility"}})
	if err != nil {
		t.Fatal(err)
	}
	db.Model(&models.KPIEvaluation{}).Where("id = ?", task.EvaluationID).Update("template_snapshot_json", raw)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/evaluations/:id/scoring-tasks", func(c *gin.Context) {
		c.Set("user_id", owner.ID)
		GetEvaluationScoringTasks(c)
	})
	record := httptest.NewRecorder()
	router.ServeHTTP(record, httptest.NewRequest("GET", "/evaluations/"+formatUint(task.EvaluationID)+"/scoring-tasks", nil))
	if record.Code != 200 {
		t.Fatalf("status=%d body=%s", record.Code, record.Body.String())
	}
	var envelope struct {
		Data struct {
			Tasks []map[string]any `json:"tasks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(record.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data.Tasks) != 1 {
		t.Fatalf("tasks=%v", envelope.Data.Tasks)
	}
}

func floatPointer(value float64) *float64 { return &value }

func formatUint(value uint) string {
	if value == 0 {
		return "0"
	}
	const digits = "0123456789"
	var buffer [20]byte
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = digits[value%10]
		value /= 10
	}
	return string(buffer[index:])
}
