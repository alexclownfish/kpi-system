package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

func newFinalScoreImportRequest(t *testing.T, rows [][]any) *http.Request {
	t.Helper()
	book := excelize.NewFile()
	sheet := "最终评分导入"
	book.SetSheetName("Sheet1", sheet)
	headers := []string{"考核ID", "评分项ID", "员工", "部门", "周期", "指标", "满分", "当前最终分", "导入最终分", "最终评价"}
	for col, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		book.SetCellValue(sheet, cell, header)
	}
	for rowIndex, values := range rows {
		for col, value := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, rowIndex+2)
			book.SetCellValue(sheet, cell, value)
		}
	}
	var workbook bytes.Buffer
	if err := book.Write(&workbook); err != nil {
		t.Fatal(err)
	}
	book.Close()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "scores.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(workbook.Bytes()); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/final-scores/import/preview", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func TestFinalScoreImportPreviewAndCommit(t *testing.T) {
	setupConfirmationDB(t)
	if err := models.DB.AutoMigrate(&models.FinalScoreImportBatch{}); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	employee, handler, evaluation := seedConfirmationEvaluation(t, "pending_confirm")
	assignScope(t, handler.ID, "ALL")
	var score models.KPIScore
	models.DB.Preload("Item").Where("evaluation_id = ?", evaluation.ID).First(&score)
	rows := [][]any{{evaluation.ID, score.ItemID, employee.Name, "研发部", "2026年9月", score.Item.Name, score.Item.MaxScore, 88, 76.5, "批量调整"}}
	router := gin.New()
	router.POST("/final-scores/import/preview", func(c *gin.Context) { c.Set("user_id", handler.ID) }, PreviewFinalScoreImport)
	previewResponse := httptest.NewRecorder()
	router.ServeHTTP(previewResponse, newFinalScoreImportRequest(t, rows))
	if previewResponse.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewResponse.Code, previewResponse.Body.String())
	}
	var preview struct {
		BatchID string        `json:"batch_id"`
		Summary importSummary `json:"summary"`
	}
	if err := json.Unmarshal(previewResponse.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.BatchID == "" || preview.Summary.ValidEvaluations != 1 {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	commitRouter := gin.New()
	commitRouter.POST("/final-scores/import/:batchId/commit", func(c *gin.Context) { c.Set("user_id", handler.ID) }, CommitFinalScoreImport)
	commitResponse := httptest.NewRecorder()
	commitRouter.ServeHTTP(commitResponse, httptest.NewRequest(http.MethodPost, "/final-scores/import/"+preview.BatchID+"/commit", nil))
	if commitResponse.Code != http.StatusOK {
		t.Fatalf("commit status=%d body=%s", commitResponse.Code, commitResponse.Body.String())
	}
	models.DB.First(&score, score.ID)
	if score.FinalScore == nil || *score.FinalScore != 76.5 || score.FinalComment != "批量调整" {
		t.Fatalf("score was not committed: %+v", score)
	}
	models.DB.First(&evaluation, evaluation.ID)
	if evaluation.TotalScore != 76.5 {
		t.Fatalf("total score=%v, want 76.5", evaluation.TotalScore)
	}
	duplicateResponse := httptest.NewRecorder()
	commitRouter.ServeHTTP(duplicateResponse, httptest.NewRequest(http.MethodPost, "/final-scores/import/"+preview.BatchID+"/commit", nil))
	if duplicateResponse.Code != http.StatusConflict {
		t.Fatalf("duplicate commit status=%d body=%s", duplicateResponse.Code, duplicateResponse.Body.String())
	}
}

func TestFinalScoreImportRejectsDuplicateOutOfRangeAndCompleted(t *testing.T) {
	setupConfirmationDB(t)
	if err := models.DB.AutoMigrate(&models.FinalScoreImportBatch{}); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	employee, handler, evaluation := seedConfirmationEvaluation(t, "pending_confirm")
	_, _, completed := seedConfirmationEvaluation(t, "completed")
	assignScope(t, handler.ID, "ALL")
	var score, completedScore models.KPIScore
	models.DB.Preload("Item").Where("evaluation_id = ?", evaluation.ID).First(&score)
	models.DB.Preload("Item").Where("evaluation_id = ?", completed.ID).First(&completedScore)
	rows := [][]any{
		{evaluation.ID, score.ItemID, employee.Name, "研发部", "月度", score.Item.Name, 100, 88, 101, "越界"},
		{evaluation.ID, score.ItemID, employee.Name, "研发部", "月度", score.Item.Name, 100, 88, 80, "重复"},
		{completed.ID, completedScore.ItemID, "员工", "研发部", "月度", completedScore.Item.Name, 100, 88, 80, "已完成"},
	}
	router := gin.New()
	router.POST("/final-scores/import/preview", func(c *gin.Context) { c.Set("user_id", handler.ID) }, PreviewFinalScoreImport)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, newFinalScoreImportRequest(t, rows))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var preview struct {
		Summary importSummary `json:"summary"`
	}
	json.Unmarshal(response.Body.Bytes(), &preview)
	if preview.Summary.InvalidRows != 3 || preview.Summary.ValidEvaluations != 0 {
		t.Fatalf("unexpected validation summary: %+v body=%s", preview.Summary, response.Body.String())
	}
}

func TestSignoffStatisticsCountsMethodsAndHonorsScope(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	employee, handler, pending := seedConfirmationEvaluation(t, "pending_confirm")
	_, _, completed := seedConfirmationEvaluation(t, "completed")
	assignScope(t, handler.ID, "ALL")
	snapshot, _, err := GetOrCreateResultSnapshot(models.DB, completed.ID, handler.ID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	models.DB.Create(&models.EvaluationConfirmation{EvaluationID: completed.ID, SnapshotID: snapshot.ID, Method: "paper", ConfirmedBy: employee.ID, ConfirmedAt: now, HandledBy: &handler.ID})
	router := gin.New()
	router.GET("/statistics/signoffs", func(c *gin.Context) { c.Set("user_id", handler.ID) }, GetSignoffStatistics)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/statistics/signoffs", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			Summary struct {
				Total, Paper, Pending int
				RecoveryRate          float64 `json:"recovery_rate"`
			} `json:"summary"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Summary.Total != 2 || payload.Data.Summary.Paper != 1 || payload.Data.Summary.Pending != 1 || payload.Data.Summary.RecoveryRate != 50 {
		t.Fatalf("unexpected summary: %+v pending=%d", payload.Data.Summary, pending.ID)
	}

	outsider := models.Employee{Name: "外部", Email: fmt.Sprintf("outside-%s@test", t.Name()), Password: "hash", Role: "employee", DepartmentID: employee.DepartmentID, IsActive: true}
	models.DB.Create(&outsider)
	assignScope(t, outsider.ID, "SELF")
	selfRouter := gin.New()
	selfRouter.GET("/statistics/signoffs", func(c *gin.Context) { c.Set("user_id", outsider.ID) }, GetSignoffStatistics)
	selfResponse := httptest.NewRecorder()
	selfRouter.ServeHTTP(selfResponse, httptest.NewRequest(http.MethodGet, "/statistics/signoffs", nil))
	if selfResponse.Code != http.StatusOK || !bytes.Contains(selfResponse.Body.Bytes(), []byte(`"total":0`)) {
		t.Fatalf("self scope leaked data: status=%d body=%s", selfResponse.Code, selfResponse.Body.String())
	}
}
