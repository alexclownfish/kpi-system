package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newPaperConfirmationRequest(t *testing.T, evaluationID uint) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("signed_at", "2026-09-17"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("remark", "线下签字已收回"); err != nil {
		t.Fatal(err)
	}
	fileWriter, err := writer.CreateFormFile("file", "signed.png")
	if err != nil {
		t.Fatal(err)
	}
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52}
	if _, err := fileWriter.Write(png); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/evaluations/%d/confirm-paper", evaluationID), &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func setupConfirmationDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:confirmation-test-%s?mode=memory&cache=shared&_busy_timeout=5000", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	models.DB = db
	if err := db.AutoMigrate(
		&models.Role{}, &models.Permission{}, &models.UserRole{}, &models.RolePermission{},
		&models.DataScope{}, &models.RoleDataScope{}, &models.AuditLog{}, &models.Department{},
		&models.Employee{}, &models.KPITemplate{}, &models.KPIItem{}, &models.KPIEvaluation{},
		&models.KPIScore{}, &models.EvaluationResultSnapshot{}, &models.EvaluationConfirmation{},
		&models.EvaluationSignoffSupplement{},
		&models.FinalScoreImportBatch{}, &models.EvaluationScoringTask{},
		&models.EvaluationCalculationResult{},
	); err != nil {
		t.Fatal(err)
	}
}

func newSupplementRequest(t *testing.T, evaluationID uint) *http.Request {
	return newSupplementRequestOptions(t, evaluationID, "signed-supplement.png", []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52}, "2026-09-18", "员工在线确认后的纸质归档")
}

func newSupplementRequestOptions(t *testing.T, evaluationID uint, filename string, payload []byte, signedAt, remark string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("signed_at", signedAt); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("remark", remark); err != nil {
		t.Fatal(err)
	}
	fileWriter, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fileWriter.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/evaluations/%d/confirmation/supplements", evaluationID), &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func seedConfirmationEvaluation(t *testing.T, status string) (models.Employee, models.Employee, models.KPIEvaluation) {
	t.Helper()
	department := models.Department{Name: "研发部"}
	models.DB.Create(&department)
	suffix := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
	employee := models.Employee{Name: "员工", Email: "employee-" + suffix + "@test", Password: "hash", Role: "employee", DepartmentID: department.ID, IsActive: true}
	other := models.Employee{Name: "其他员工", Email: "other-" + suffix + "@test", Password: "hash", Role: "employee", DepartmentID: department.ID, IsActive: true}
	models.DB.Create(&employee)
	models.DB.Create(&other)
	template := models.KPITemplate{Name: "月度考核", Period: "monthly", IsActive: true}
	models.DB.Create(&template)
	item := models.KPIItem{TemplateID: template.ID, Name: "质量", MaxScore: 100, Order: 1}
	models.DB.Create(&item)
	scoreValue := 88.0
	evaluation := models.KPIEvaluation{EmployeeID: employee.ID, TemplateID: template.ID, Period: "monthly", Year: 2026, Status: status, TotalScore: 88}
	models.DB.Create(&evaluation)
	models.DB.Create(&models.KPIScore{EvaluationID: evaluation.ID, ItemID: item.ID, HRScore: &scoreValue, HRComment: "表现良好"})
	return employee, other, evaluation
}

func assignScope(t *testing.T, userID uint, code string) {
	t.Helper()
	role := models.Role{Code: "role-" + code + "-" + fmt.Sprint(userID), Name: code}
	scope := models.DataScope{Code: code, Name: code}
	models.DB.Create(&role)
	models.DB.Where("code = ?", code).FirstOrCreate(&scope)
	models.DB.Create(&models.UserRole{UserID: userID, RoleID: role.ID})
	models.DB.Create(&models.RoleDataScope{RoleID: role.ID, DataScopeID: scope.ID})
}

func TestOnlineConfirmationCreatesEvidenceAndLocksEvaluation(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	employee, _, evaluation := seedConfirmationEvaluation(t, "pending_confirm")
	assignScope(t, employee.ID, "SELF")
	r := gin.New()
	r.POST("/evaluations/:id/confirm-online", func(c *gin.Context) { c.Set("user_id", employee.ID) }, ConfirmEvaluationOnline)
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/evaluations/%d/confirm-online", evaluation.ID), nil)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var confirmation models.EvaluationConfirmation
	if err := models.DB.Where("evaluation_id = ?", evaluation.ID).First(&confirmation).Error; err != nil {
		t.Fatal(err)
	}
	if confirmation.Method != "online" || confirmation.ConfirmedBy != employee.ID {
		t.Fatalf("unexpected confirmation: %+v", confirmation)
	}
	models.DB.First(&evaluation, evaluation.ID)
	if evaluation.Status != "completed" {
		t.Fatalf("status=%s, want completed", evaluation.Status)
	}
	var score models.KPIScore
	models.DB.Where("evaluation_id = ?", evaluation.ID).First(&score)
	if score.FinalScore == nil || *score.FinalScore != 88 {
		t.Fatalf("final score=%v, want 88", score.FinalScore)
	}
	if err := ensureEvaluationNotCompleted(evaluation.ID); err == nil {
		t.Fatal("completed evaluation must be locked")
	}
}

func TestOnlineConfirmationRejectsAnotherEmployeeAndObjection(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	employee, other, evaluation := seedConfirmationEvaluation(t, "pending_confirm")
	assignScope(t, employee.ID, "SELF")
	assignScope(t, other.ID, "SELF")
	r := gin.New()
	r.POST("/evaluations/:id/confirm-online", func(c *gin.Context) { c.Set("user_id", other.ID) }, ConfirmEvaluationOnline)
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/evaluations/%d/confirm-online", evaluation.ID), nil)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s, want 403", response.Code, response.Body.String())
	}
	models.DB.Model(&evaluation).Update("has_objection", true)
	r = gin.New()
	r.POST("/evaluations/:id/confirm-online", func(c *gin.Context) { c.Set("user_id", employee.ID) }, ConfirmEvaluationOnline)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request.Clone(request.Context()))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "异议") {
		t.Fatalf("status=%d body=%s, want objection rejection", response.Code, response.Body.String())
	}
}

func TestGenericEvaluationUpdateCannotBypassConfirmation(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	employee, _, evaluation := seedConfirmationEvaluation(t, "pending_confirm")
	assignScope(t, employee.ID, "SELF")
	r := gin.New()
	r.PUT("/evaluations/:id", func(c *gin.Context) { c.Set("user_id", employee.ID) }, UpdateEvaluation)
	request := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/evaluations/%d", evaluation.ID), strings.NewReader(`{"status":"completed"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "确认接口") {
		t.Fatalf("status=%d body=%s, want confirmation-endpoint rejection", response.Code, response.Body.String())
	}
	models.DB.First(&evaluation, evaluation.ID)
	if evaluation.Status != "pending_confirm" {
		t.Fatalf("status=%s, generic update bypassed confirmation", evaluation.Status)
	}
}

func TestConfirmationReadHonorsEvaluationScope(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	employee, other, evaluation := seedConfirmationEvaluation(t, "completed")
	assignScope(t, employee.ID, "SELF")
	assignScope(t, other.ID, "SELF")
	snapshot, _, err := GetOrCreateResultSnapshot(models.DB, evaluation.ID, employee.ID)
	if err != nil {
		t.Fatal(err)
	}
	models.DB.Create(&models.EvaluationConfirmation{EvaluationID: evaluation.ID, SnapshotID: snapshot.ID, Method: "online", ConfirmedBy: employee.ID})
	r := gin.New()
	r.GET("/evaluations/:id/confirmation", func(c *gin.Context) { c.Set("user_id", other.ID) }, GetEvaluationConfirmation)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/evaluations/%d/confirmation", evaluation.ID), nil))
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), snapshot.Checksum) {
		t.Fatalf("status=%d body=%s, want protected 403", response.Code, response.Body.String())
	}
}

func TestEvaluationExportRejectsMissingPermissionScopeAndDraftStatus(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	employee, other, completed := seedConfirmationEvaluation(t, "completed")
	assignScope(t, employee.ID, "SELF")
	assignScope(t, other.ID, "SELF")

	permissionRouter := gin.New()
	permissionRouter.GET("/export/evaluation/:id", func(c *gin.Context) { c.Set("user_id", other.ID) }, PermissionMiddleware("report:export"), ExportEvaluationToExcel)
	response := httptest.NewRecorder()
	permissionRouter.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/export/evaluation/%d", completed.ID), nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("missing permission status=%d body=%s, want 403", response.Code, response.Body.String())
	}

	scopeRouter := gin.New()
	scopeRouter.GET("/export/evaluation/:id", func(c *gin.Context) { c.Set("user_id", other.ID) }, ExportEvaluationToExcel)
	response = httptest.NewRecorder()
	scopeRouter.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/export/evaluation/%d", completed.ID), nil))
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), employee.Name) {
		t.Fatalf("scope status=%d body=%s, want protected 403", response.Code, response.Body.String())
	}

	draftEmployee, _, draft := seedConfirmationEvaluation(t, "pending")
	assignScope(t, draftEmployee.ID, "SELF")
	draftRouter := gin.New()
	draftRouter.GET("/export/evaluation/:id", func(c *gin.Context) { c.Set("user_id", draftEmployee.ID) }, ExportEvaluationToExcel)
	response = httptest.NewRecorder()
	draftRouter.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/export/evaluation/%d", draft.ID), nil))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"EXPORT_RESULT_NOT_FINAL"`) || !strings.Contains(response.Body.String(), "尚未定稿") {
		t.Fatalf("draft status=%d body=%s, want final-state rejection", response.Code, response.Body.String())
	}
}

func TestSnapshotVersionChangesOnlyWhenResultChanges(t *testing.T) {
	setupConfirmationDB(t)
	employee, _, evaluation := seedConfirmationEvaluation(t, "pending_confirm")
	first, _, err := GetOrCreateResultSnapshot(models.DB, evaluation.ID, employee.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, _, _ := GetOrCreateResultSnapshot(models.DB, evaluation.ID, employee.ID)
	if first.ID != second.ID || second.Version != 1 {
		t.Fatalf("unchanged snapshot should be reused: first=%+v second=%+v", first, second)
	}
	models.DB.Model(&evaluation).Update("total_score", 90)
	third, _, err := GetOrCreateResultSnapshot(models.DB, evaluation.ID, employee.ID)
	if err != nil {
		t.Fatal(err)
	}
	if third.Version != 2 || third.Checksum == first.Checksum {
		t.Fatalf("changed result should create V2: first=%+v third=%+v", first, third)
	}
}

func TestAcceptedSignoffTypes(t *testing.T) {
	for _, tc := range []struct{ name, mime string }{{"signed.pdf", "application/pdf"}, {"signed.jpg", "image/jpeg"}, {"signed.png", "image/png"}} {
		if _, ok := acceptedSignoffType(tc.name, tc.mime); !ok {
			t.Fatalf("expected %s to be accepted", tc.name)
		}
	}
	if _, ok := acceptedSignoffType("signed.exe", "application/octet-stream"); ok {
		t.Fatal("executable must be rejected")
	}
}

func TestPaperConfirmationRequiresCurrentExportAndArchivesFile(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	employee, handler, evaluation := seedConfirmationEvaluation(t, "pending_confirm")
	assignScope(t, handler.ID, "ALL")
	uploadDir := t.TempDir()
	t.Setenv("SIGNOFF_UPLOAD_DIR", uploadDir)
	r := gin.New()
	r.POST("/evaluations/:id/confirm-paper", func(c *gin.Context) { c.Set("user_id", handler.ID) }, ConfirmEvaluationPaper)

	response := httptest.NewRecorder()
	r.ServeHTTP(response, newPaperConfirmationRequest(t, evaluation.ID))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "先导出") {
		t.Fatalf("status=%d body=%s, want export-required rejection", response.Code, response.Body.String())
	}
	if _, _, err := GetOrCreateResultSnapshot(models.DB, evaluation.ID, handler.ID); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	r.ServeHTTP(response, newPaperConfirmationRequest(t, evaluation.ID))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var confirmation models.EvaluationConfirmation
	if err := models.DB.Where("evaluation_id = ?", evaluation.ID).First(&confirmation).Error; err != nil {
		t.Fatal(err)
	}
	if confirmation.Method != "paper" || confirmation.ConfirmedBy != employee.ID || confirmation.HandledBy == nil || *confirmation.HandledBy != handler.ID {
		t.Fatalf("unexpected confirmation: %+v", confirmation)
	}
	if _, err := os.Stat(uploadDir + "/" + confirmation.AttachmentStoredName); err != nil {
		t.Fatalf("signed file was not archived: %v", err)
	}
}

func TestPaperConfirmationRejectsStaleSnapshot(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	_, handler, evaluation := seedConfirmationEvaluation(t, "pending_confirm")
	assignScope(t, handler.ID, "ALL")
	t.Setenv("SIGNOFF_UPLOAD_DIR", t.TempDir())
	if _, _, err := GetOrCreateResultSnapshot(models.DB, evaluation.ID, handler.ID); err != nil {
		t.Fatal(err)
	}
	models.DB.Model(&evaluation).Update("total_score", 91)
	r := gin.New()
	r.POST("/evaluations/:id/confirm-paper", func(c *gin.Context) { c.Set("user_id", handler.ID) }, ConfirmEvaluationPaper)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, newPaperConfirmationRequest(t, evaluation.ID))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "重新导出") {
		t.Fatalf("status=%d body=%s, want stale-version rejection", response.Code, response.Body.String())
	}
}

func TestCompletedEvaluationAcceptsSignoffSupplementWithoutChangingConfirmation(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	employee, handler, evaluation := seedConfirmationEvaluation(t, "completed")
	assignScope(t, handler.ID, "ALL")
	uploadDir := t.TempDir()
	t.Setenv("SIGNOFF_UPLOAD_DIR", uploadDir)
	snapshot, _, err := GetOrCreateResultSnapshot(models.DB, evaluation.ID, handler.ID)
	if err != nil {
		t.Fatal(err)
	}
	confirmedAt := time.Now().Add(-time.Hour).Truncate(time.Second)
	original := models.EvaluationConfirmation{EvaluationID: evaluation.ID, SnapshotID: snapshot.ID, Method: "online", ConfirmedBy: employee.ID, ConfirmedAt: confirmedAt}
	if err := models.DB.Create(&original).Error; err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.POST("/evaluations/:id/confirmation/supplements", func(c *gin.Context) { c.Set("user_id", handler.ID) }, UploadConfirmationSupplement)
	r.GET("/evaluations/:id/confirmation/supplements", func(c *gin.Context) { c.Set("user_id", handler.ID) }, ListConfirmationSupplements)
	r.GET("/evaluations/:id/confirmation/supplements/:supplement_id/attachment", func(c *gin.Context) { c.Set("user_id", handler.ID) }, DownloadConfirmationSupplement)
	response := httptest.NewRecorder()
	uploadRequest := newSupplementRequest(t, evaluation.ID)
	uploadRequest.Header.Set("X-Request-ID", "req-signoff-supplement-success")
	r.ServeHTTP(response, uploadRequest)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var uploadEnvelope struct {
		Data       models.EvaluationSignoffSupplement `json:"data"`
		Evaluation struct {
			ID     uint   `json:"id"`
			Status string `json:"status"`
		} `json:"evaluation"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &uploadEnvelope); err != nil {
		t.Fatalf("decode upload envelope: %v; body=%s", err, response.Body.String())
	}
	if uploadEnvelope.RequestID != "req-signoff-supplement-success" || uploadEnvelope.Data.ID == 0 || uploadEnvelope.Evaluation.ID != evaluation.ID || uploadEnvelope.Evaluation.Status != "completed" {
		t.Fatalf("unexpected upload envelope: %+v", uploadEnvelope)
	}
	var after models.EvaluationConfirmation
	if err := models.DB.Where("evaluation_id = ?", evaluation.ID).First(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after.ID != original.ID || after.Method != "online" || !after.ConfirmedAt.Equal(confirmedAt) {
		t.Fatalf("original confirmation changed: before=%+v after=%+v", original, after)
	}
	var stored models.EvaluationSignoffSupplement
	if err := models.DB.Where("evaluation_id = ?", evaluation.ID).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.UploadedBy != handler.ID || stored.AttachmentOriginalName != "signed-supplement.png" {
		t.Fatalf("unexpected supplement: %+v", stored)
	}
	var current models.KPIEvaluation
	if err := models.DB.First(&current, evaluation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.Status != "completed" {
		t.Fatalf("status=%s, want completed", current.Status)
	}
	var audit models.AuditLog
	if err := models.DB.Where("action = ? AND resource_id = ?", "upload_signoff_supplement", fmt.Sprint(evaluation.ID)).First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(audit.Details, "纸质归档") {
		t.Fatal("audit details must not include sensitive remark content")
	}
	listResponse := httptest.NewRecorder()
	r.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/evaluations/%d/confirmation/supplements", evaluation.ID), nil))
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), "signed-supplement.png") {
		t.Fatalf("list status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	var listEnvelope struct {
		Data      []models.EvaluationSignoffSupplement `json:"data"`
		RequestID string                               `json:"request_id"`
	}
	if err := json.Unmarshal(listResponse.Body.Bytes(), &listEnvelope); err != nil {
		t.Fatalf("decode list envelope: %v; body=%s", err, listResponse.Body.String())
	}
	if len(listEnvelope.Data) != 1 || listEnvelope.RequestID == "" {
		t.Fatalf("unexpected list envelope: %+v", listEnvelope)
	}
	downloadResponse := httptest.NewRecorder()
	r.ServeHTTP(downloadResponse, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/evaluations/%d/confirmation/supplements/%d/attachment", evaluation.ID, stored.ID), nil))
	if downloadResponse.Code != http.StatusOK || downloadResponse.Body.Len() == 0 {
		t.Fatalf("download status=%d len=%d", downloadResponse.Code, downloadResponse.Body.Len())
	}
}

func TestSignoffSupplementRejectsNonCompletedEvaluation(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	_, handler, evaluation := seedConfirmationEvaluation(t, "awaiting_employee_confirmation")
	assignScope(t, handler.ID, "ALL")
	t.Setenv("SIGNOFF_UPLOAD_DIR", t.TempDir())
	r := gin.New()
	r.POST("/evaluations/:id/confirmation/supplements", func(c *gin.Context) { c.Set("user_id", handler.ID) }, UploadConfirmationSupplement)
	response := httptest.NewRecorder()
	request := newSupplementRequest(t, evaluation.ID)
	request.Header.Set("X-Request-ID", "req-signoff-supplement-state")
	r.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s, want conflict", response.Code, response.Body.String())
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode state error envelope: %v; body=%s", err, response.Body.String())
	}
	if envelope.Error.Code != "SIGNOFF_SUPPLEMENT_STATE_CONFLICT" || envelope.RequestID != "req-signoff-supplement-state" {
		t.Fatalf("unexpected state error envelope: %+v", envelope)
	}
}

func TestSignoffSupplementRejectsCompletedEvaluationWithoutConfirmation(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	_, handler, evaluation := seedConfirmationEvaluation(t, "completed")
	assignScope(t, handler.ID, "ALL")
	t.Setenv("SIGNOFF_UPLOAD_DIR", t.TempDir())
	r := gin.New()
	r.POST("/evaluations/:id/confirmation/supplements", func(c *gin.Context) { c.Set("user_id", handler.ID) }, UploadConfirmationSupplement)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, newSupplementRequest(t, evaluation.ID))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "缺少最终确认记录") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var count int64
	models.DB.Model(&models.EvaluationSignoffSupplement{}).Count(&count)
	if count != 0 {
		t.Fatal("missing confirmation must not create supplement")
	}
}

func TestSignoffSupplementPermissionScopeAndAssessmentBinding(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	employee, other, evaluation := seedConfirmationEvaluation(t, "completed")
	assignScope(t, employee.ID, "SELF")
	assignScope(t, other.ID, "SELF")
	r := gin.New()
	r.POST("/evaluations/:id/confirmation/supplements", func(c *gin.Context) { c.Set("user_id", employee.ID) }, PermissionMiddleware("assessment:approve"), UploadConfirmationSupplement)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, newSupplementRequest(t, evaluation.ID))
	if response.Code != http.StatusForbidden {
		t.Fatalf("owner without approval must be rejected: %d %s", response.Code, response.Body.String())
	}
	uploadDir := t.TempDir()
	t.Setenv("SIGNOFF_UPLOAD_DIR", uploadDir)
	// Scope enforcement is independent of the approval permission. An in-scope
	// permission bypass must still be rejected before a file is written.
	r = gin.New()
	r.POST("/evaluations/:id/confirmation/supplements", func(c *gin.Context) { c.Set("user_id", other.ID) }, UploadConfirmationSupplement)
	r.GET("/evaluations/:id/confirmation/supplements", func(c *gin.Context) { c.Set("user_id", other.ID) }, ListConfirmationSupplements)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, newSupplementRequest(t, evaluation.ID))
	if response.Code != http.StatusForbidden {
		t.Fatalf("out-of-scope upload must be rejected: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/evaluations/%d/confirmation/supplements", evaluation.ID), nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("out-of-scope list must be rejected: %d %s", response.Code, response.Body.String())
	}
	if err := os.WriteFile(uploadDir+"/scope.png", []byte("signed"), 0600); err != nil {
		t.Fatal(err)
	}
	stored := models.EvaluationSignoffSupplement{EvaluationID: evaluation.ID, UploadedBy: employee.ID, UploadedAt: time.Now(), AttachmentStoredName: "scope.png", AttachmentOriginalName: "signed.png", AttachmentContentType: "image/png"}
	if err := models.DB.Create(&stored).Error; err != nil {
		t.Fatal(err)
	}
	r = gin.New()
	r.GET("/evaluations/:id/confirmation/supplements/:supplement_id/attachment", func(c *gin.Context) { c.Set("user_id", other.ID) }, DownloadConfirmationSupplement)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/evaluations/%d/confirmation/supplements/%d/attachment", evaluation.ID, stored.ID), nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("out-of-scope download must be rejected: %d %s", response.Code, response.Body.String())
	}
	otherEvaluation := models.KPIEvaluation{EmployeeID: other.ID, TemplateID: evaluation.TemplateID, Status: "completed"}
	if err := models.DB.Create(&otherEvaluation).Error; err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/evaluations/%d/confirmation/supplements/%d/attachment", otherEvaluation.ID, stored.ID), nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("supplement from another assessment must be rejected: %d %s", response.Code, response.Body.String())
	}
}

func seedCompletedWithConfirmation(t *testing.T) (models.Employee, models.Employee, models.KPIEvaluation, string) {
	t.Helper()
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	employee, handler, evaluation := seedConfirmationEvaluation(t, "completed")
	assignScope(t, handler.ID, "ALL")
	uploadDir := t.TempDir()
	t.Setenv("SIGNOFF_UPLOAD_DIR", uploadDir)
	snapshot, _, err := GetOrCreateResultSnapshot(models.DB, evaluation.ID, handler.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := models.DB.Create(&models.EvaluationConfirmation{EvaluationID: evaluation.ID, SnapshotID: snapshot.ID, Method: "online", ConfirmedBy: employee.ID, ConfirmedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	return employee, handler, evaluation, uploadDir
}

func TestSignoffSupplementRejectsInvalidFilesAndFields(t *testing.T) {
	_, handler, evaluation, uploadDir := seedCompletedWithConfirmation(t)
	r := gin.New()
	r.POST("/evaluations/:id/confirmation/supplements", func(c *gin.Context) { c.Set("user_id", handler.ID) }, UploadConfirmationSupplement)
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	cases := []struct {
		name, filename, signedAt, remark string
		payload                          []byte
	}{
		{"invalid content type", "bad.txt", "2026-09-18", "", []byte("not an image")},
		{"empty file", "empty.png", "2026-09-18", "", nil},
		{"invalid date", "bad-date.png", "18-09-2026", "", png},
		{"oversized remark", "long-remark.png", "2026-09-18", strings.Repeat("x", 501), png},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			r.ServeHTTP(response, newSupplementRequestOptions(t, evaluation.ID, test.filename, test.payload, test.signedAt, test.remark))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
	large := make([]byte, maxSignoffFileSize+1)
	copy(large, png)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, newSupplementRequestOptions(t, evaluation.ID, "large.png", large, "2026-09-18", ""))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized file status=%d body=%s", response.Code, response.Body.String())
	}
	var count int64
	if err := models.DB.Model(&models.EvaluationSignoffSupplement{}).Where("evaluation_id = ?", evaluation.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid uploads created %d supplement rows", count)
	}
	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid uploads left %d attachment files", len(entries))
	}
}

func TestSignoffSupplementConcurrentAppendKeepsEachAttachment(t *testing.T) {
	_, handler, evaluation, uploadDir := seedCompletedWithConfirmation(t)
	r := gin.New()
	r.POST("/evaluations/:id/confirmation/supplements", func(c *gin.Context) { c.Set("user_id", handler.ID) }, UploadConfirmationSupplement)
	type result struct{ code int }
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func(index int) {
			response := httptest.NewRecorder()
			request := newSupplementRequestOptions(t, evaluation.ID, fmt.Sprintf("concurrent-%d.png", index), []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, "2026-09-18", "")
			r.ServeHTTP(response, request)
			results <- result{code: response.Code}
		}(i)
	}
	for i := 0; i < 2; i++ {
		if current := <-results; current.code != http.StatusOK {
			t.Fatalf("concurrent upload status=%d", current.code)
		}
	}
	var count int64
	if err := models.DB.Model(&models.EvaluationSignoffSupplement{}).Where("evaluation_id = ?", evaluation.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("concurrent uploads count=%d, want 2", count)
	}
	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("stored attachments=%d, want 2", len(entries))
	}
}

func TestSignoffSupplementRemovesFileWhenDatabaseWriteFails(t *testing.T) {
	_, handler, evaluation, uploadDir := seedCompletedWithConfirmation(t)
	callbackName := "test_fail_signoff_supplement_create"
	if err := models.DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "evaluation_signoff_supplements" {
			tx.AddError(fmt.Errorf("forced supplement write failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer models.DB.Callback().Create().Remove(callbackName)
	r := gin.New()
	r.POST("/evaluations/:id/confirmation/supplements", func(c *gin.Context) { c.Set("user_id", handler.ID) }, UploadConfirmationSupplement)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, newSupplementRequest(t, evaluation.ID))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("database failure left %d attachment files", len(entries))
	}
	var count int64
	models.DB.Model(&models.EvaluationSignoffSupplement{}).Where("evaluation_id = ?", evaluation.ID).Count(&count)
	if count != 0 {
		t.Fatalf("database failure created %d supplement rows", count)
	}
}
