package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dootask-kpi-server/models"

	"github.com/gin-gonic/gin"
)

func TestExportLayoutValidationAndFallback(t *testing.T) {
	defaultLayout, err := normalizeStoredExportLayout("")
	if err != nil {
		t.Fatal(err)
	}
	if defaultLayout.Preset != exportPresetFinalSignoff || len(defaultLayout.Columns) != 4 {
		t.Fatalf("unexpected default layout: %+v", defaultLayout)
	}
	invalid := defaultLayout
	invalid.Columns = []string{"item_name", "max_score"}
	if _, err := validateExportLayout(invalid); err == nil || !strings.Contains(err.Error(), "最终得分") {
		t.Fatalf("expected required final score error, got %v", err)
	}
	invalid = defaultLayout
	invalid.SignatureLabels = []string{"员工签字", "员工签字"}
	if _, err := validateExportLayout(invalid); err == nil || !strings.Contains(err.Error(), "不能重复") {
		t.Fatalf("expected duplicate signature error, got %v", err)
	}
}

func TestPendingConfirmationSnapshotRefreshesTemplateLayout(t *testing.T) {
	setupConfirmationDB(t)
	employee, _, evaluation := seedConfirmationEvaluation(t, "pending_confirm")
	full := defaultExportLayout(exportPresetFullProcess)
	raw, _, err := marshalExportLayout(full)
	if err != nil {
		t.Fatal(err)
	}
	models.DB.Model(&models.KPITemplate{}).Where("id = ?", evaluation.TemplateID).Update("export_layout_json", raw)
	first, payload, err := GetOrCreateResultSnapshot(models.DB, evaluation.ID, employee.ID)
	if err != nil {
		t.Fatal(err)
	}
	if payload.ExportLayout.Preset != exportPresetFullProcess {
		t.Fatalf("layout=%+v, want full process", payload.ExportLayout)
	}
	doc := buildExportDocument(first, payload)
	if len(doc.Headers) != len(full.Columns) || len(doc.Rows) != 1 {
		t.Fatalf("unexpected document: %+v", doc)
	}
	simpleRaw, _, _ := marshalExportLayout(defaultExportLayout(exportPresetFinalSignoff))
	models.DB.Model(&models.KPITemplate{}).Where("id = ?", evaluation.TemplateID).Update("export_layout_json", simpleRaw)
	second, secondPayload, err := GetOrCreateResultSnapshot(models.DB, evaluation.ID, employee.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID || second.Version != first.Version+1 || second.Checksum == first.Checksum || secondPayload.ExportLayout.Preset != exportPresetFinalSignoff {
		t.Fatalf("pending snapshot did not refresh layout: first=%+v second=%+v layout=%+v", first, second, secondPayload.ExportLayout)
	}
}

func TestCompletedEvaluationKeepsConfirmedSnapshotLayout(t *testing.T) {
	setupConfirmationDB(t)
	employee, _, evaluation := seedConfirmationEvaluation(t, "pending_confirm")
	fullRaw, _, err := marshalExportLayout(defaultExportLayout(exportPresetFullProcess))
	if err != nil {
		t.Fatal(err)
	}
	models.DB.Model(&models.KPITemplate{}).Where("id = ?", evaluation.TemplateID).Update("export_layout_json", fullRaw)
	first, firstPayload, err := GetOrCreateResultSnapshot(models.DB, evaluation.ID, employee.ID)
	if err != nil {
		t.Fatal(err)
	}
	if firstPayload.ExportLayout.Preset != exportPresetFullProcess {
		t.Fatalf("layout=%+v, want full process", firstPayload.ExportLayout)
	}
	models.DB.Model(&models.KPIEvaluation{}).Where("id = ?", evaluation.ID).Update("status", "completed")
	simpleRaw, _, err := marshalExportLayout(defaultExportLayout(exportPresetFinalSignoff))
	if err != nil {
		t.Fatal(err)
	}
	models.DB.Model(&models.KPITemplate{}).Where("id = ?", evaluation.TemplateID).Update("export_layout_json", simpleRaw)
	second, secondPayload, err := GetOrCreateResultSnapshot(models.DB, evaluation.ID, employee.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Version != first.Version || second.Checksum != first.Checksum || secondPayload.ExportLayout.Preset != exportPresetFullProcess {
		t.Fatalf("completed snapshot layout changed: first=%+v second=%+v layout=%+v", first, second, secondPayload.ExportLayout)
	}
}

func TestResultPDFPathsUseAbsoluteUnicodeFileURL(t *testing.T) {
	pdfPath, htmlPath, htmlURL, err := resultPDFPaths(filepath.Join("public", "exports", "绩效结果 张三.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(pdfPath) || !filepath.IsAbs(htmlPath) {
		t.Fatalf("paths must be absolute: pdf=%q html=%q", pdfPath, htmlPath)
	}
	parsed, err := url.Parse(htmlURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "file" || parsed.Host != "" || !filepath.IsAbs(filepath.FromSlash(parsed.Path)) {
		t.Fatalf("invalid local file URL: %q parsed=%+v", htmlURL, parsed)
	}
	if strings.HasPrefix(htmlURL, "file://public/") || !strings.Contains(htmlURL, "%E7%BB%A9%E6%95%88%E7%BB%93%E6%9E%9C%20%E5%BC%A0%E4%B8%89.html") {
		t.Fatalf("local file URL was not safely encoded: %q", htmlURL)
	}
}

func TestMixedTemplateDocumentsDoNotShareColumns(t *testing.T) {
	simple := defaultExportLayout(exportPresetFinalSignoff)
	full := defaultExportLayout(exportPresetFullProcess)
	for index := 0; index < 100; index++ {
		layout := simple
		if index%2 == 1 {
			layout = full
		}
		payload := resultSnapshotPayload{EvaluationID: uint(index + 1), EmployeeName: fmt.Sprintf("员工%d", index+1), TemplateName: layout.Preset, Period: "monthly", Year: 2026, TotalScore: 88, ExportLayout: layout, Items: []resultSnapshotItem{{Name: "质量", MaxScore: 100, FinalScore: 88}}}
		doc := buildExportDocument(models.EvaluationResultSnapshot{Version: 1, Checksum: fmt.Sprintf("checksum-%d", index)}, payload)
		if len(doc.Columns) != len(layout.Columns) || doc.TemplateName != layout.Preset {
			t.Fatalf("document %d used wrong layout: %+v", index, doc)
		}
	}
}

func TestLegacySnapshotUsesDefaultLayoutWithoutNewVersion(t *testing.T) {
	setupConfirmationDB(t)
	employee, _, evaluation := seedConfirmationEvaluation(t, "completed")
	legacy := resultSnapshotPayload{EvaluationID: evaluation.ID, EmployeeID: employee.ID, EmployeeName: employee.Name, TemplateID: evaluation.TemplateID, TemplateName: "旧模板", Period: "monthly", Year: 2026, TotalScore: 88, Items: []resultSnapshotItem{{ItemID: 1, Name: "质量", MaxScore: 100, FinalScore: 88}}}
	raw, _ := json.Marshal(legacy)
	var legacyFields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &legacyFields)
	delete(legacyFields, "export_layout")
	raw, _ = json.Marshal(legacyFields)
	snapshot := models.EvaluationResultSnapshot{EvaluationID: evaluation.ID, Version: 1, SnapshotJSON: string(raw), Checksum: "legacy-checksum", CreatedBy: employee.ID}
	models.DB.Create(&snapshot)
	loaded, payload, err := GetOrCreateResultSnapshot(models.DB, evaluation.ID, employee.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != snapshot.ID || loaded.Version != 1 || payload.ExportLayout.Preset != exportPresetFinalSignoff {
		t.Fatalf("legacy snapshot not reused: snapshot=%+v payload=%+v", loaded, payload)
	}
}

func TestUpdateTemplateRejectsInvalidLayoutAndReturnsNormalizedLayout(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	templateModel := models.KPITemplate{Name: "测试模板", Period: "monthly", IsActive: true}
	models.DB.Create(&templateModel)
	router := gin.New()
	router.PUT("/templates/:id", func(c *gin.Context) { c.Set("user_id", uint(1)) }, UpdateTemplate)

	invalidBody := `{"export_layout":{"version":1,"preset":"final_signoff","title":"结果表","columns":["item_name","max_score"],"show_summary":true,"show_employee_opinion":true,"signature_labels":["员工签字"]}}`
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/templates/%d", templateModel.ID), strings.NewReader(invalidBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "最终得分") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	validBody := `{"export_layout":{"version":1,"preset":"final_signoff","title":"结果表","columns":["item_name","final_score"],"show_summary":false,"show_employee_opinion":false,"signature_labels":["员工签字"]}}`
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/templates/%d", templateModel.ID), strings.NewReader(validBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"export_layout"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestEvaluationExportRejectsInvalidFormat(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	employee, _, evaluation := seedConfirmationEvaluation(t, "completed")
	assignScope(t, employee.ID, "SELF")
	router := gin.New()
	router.GET("/export/evaluation/:id", func(c *gin.Context) { c.Set("user_id", employee.ID) }, ExportEvaluationToExcel)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/export/evaluation/%d?format=docx", evaluation.ID), nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "xlsx") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestEvaluationExportDefaultsToExcelAndSupportsPDF(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	employee, _, evaluation := seedConfirmationEvaluation(t, "completed")
	assignScope(t, employee.ID, "SELF")
	router := gin.New()
	router.GET("/export/evaluation/:id", func(c *gin.Context) { c.Set("user_id", employee.ID) }, ExportEvaluationToExcel)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/export/evaluation/%d", evaluation.ID), nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `.xlsx`) {
		t.Fatalf("default export status=%d body=%s", response.Code, response.Body.String())
	}

	originalPDFRenderer := renderResultPDFFile
	renderResultPDFFile = func(_ exportDocument, path string) error {
		return os.WriteFile(path, []byte("%PDF-1.4 test"), 0600)
	}
	t.Cleanup(func() { renderResultPDFFile = originalPDFRenderer })
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/export/evaluation/%d?format=pdf", evaluation.ID), nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `.pdf`) {
		t.Fatalf("pdf export status=%d body=%s", response.Code, response.Body.String())
	}
}
