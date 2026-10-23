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

func TestExportContentTypeFollowsFileExtension(t *testing.T) {
	if got := exportContentType("result.xlsx"); got != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("xlsx content type=%q", got)
	}
	if got := exportContentType("result.pdf"); got != "application/pdf" {
		t.Fatalf("pdf content type=%q", got)
	}
	if got := exportContentType("result.unknown"); got != "application/octet-stream" {
		t.Fatalf("unknown content type=%q", got)
	}
}

func TestExportErrorIncludesStableDiagnostics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/export", func(c *gin.Context) {
		version := 7
		respondExportError(c, http.StatusInternalServerError, "EXPORT_RENDER_FAILED", "生成导出文件失败", true, "请稍后重试", &version)
	})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/export", nil)
	request.Header.Set("X-Request-ID", "req_export_test")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload apiErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Code != "EXPORT_RENDER_FAILED" || payload.RequestID != "req_export_test" {
		t.Fatalf("unexpected error envelope: %+v", payload)
	}
	details, ok := payload.Error.Details.(map[string]any)
	if !ok || details["retryable"] != true || details["retry_guidance"] != "请稍后重试" || details["result_version"] != float64(7) {
		t.Fatalf("unexpected export diagnostics: %#v", payload.Error.Details)
	}
}

func TestExportLayoutValidationAndFallback(t *testing.T) {
	defaultLayout, err := normalizeStoredExportLayout("")
	if err != nil {
		t.Fatal(err)
	}
	if defaultLayout.Preset != exportPresetFinalSignoff || len(defaultLayout.Columns) != 6 {
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

func TestResultPDFUsesPortraitA4Layout(t *testing.T) {
	if !strings.Contains(resultPDFHTML, "@page{size:A4 portrait") {
		t.Fatal("result PDF must use A4 portrait orientation")
	}
	if strings.Contains(resultPDFHTML, "A4 landscape") {
		t.Fatal("result PDF must not use landscape orientation")
	}
	for _, required := range []string{"table-layout:fixed", "overflow-wrap:anywhere", "display:table-header-group"} {
		if !strings.Contains(resultPDFHTML, required) {
			t.Fatalf("portrait PDF missing compact pagination style %q", required)
		}
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

func TestWeightedExportColumnsIncludeWeightsScorersAndContribution(t *testing.T) {
	layout := defaultExportLayout(exportPresetFinalSignoff)
	layout.Columns = []string{"dimension_name", "dimension_weight", "item_name", "item_weight", "composite_weight", "scorer_type", "scorer_name", "weighted_contribution", "final_score"}
	payload := resultSnapshotPayload{
		EvaluationID: 11, TemplateName: "研发模板", Period: "monthly", Year: 2026, TotalScore: 86.5, ExportLayout: layout,
		Items: []resultSnapshotItem{{
			ItemID: 7, DimensionName: "交付质量", DimensionWeight: 40, Name: "缺陷密度", ItemWeight: 50, CompositeWeight: 20,
			ScorerType: "项目经理", ScorerName: "项目经理", FinalScore: 86.5, WeightedContribution: 17.3, HasWeightedFields: true,
		}},
	}
	doc := buildExportDocument(models.EvaluationResultSnapshot{Version: 2, Checksum: "weighted"}, payload)
	wantHeaders := []string{"维度", "维度权重", "指标", "指标权重", "综合权重", "评分职责", "实际评分人", "加权贡献", "最终得分"}
	if strings.Join(doc.Headers, "|") != strings.Join(wantHeaders, "|") {
		t.Fatalf("headers=%v, want=%v", doc.Headers, wantHeaders)
	}
	if len(doc.Rows) != 1 || strings.Join(doc.Rows[0], "|") != "交付质量|40.00|缺陷密度|50.00|20.00|项目经理|项目经理|17.30|86.50" {
		t.Fatalf("row=%v", doc.Rows)
	}
}

func TestExportDocumentIncludesCanonicalGradeAndWeightedContributionInPDFMeta(t *testing.T) {
	layout := defaultExportLayout(exportPresetFinalSignoff)
	payload := resultSnapshotPayload{
		EvaluationID: 12, TemplateName: "研发模板", Period: "monthly", Year: 2026,
		TotalScore: 84.25, Grade: "B", ExportLayout: layout,
		Items: []resultSnapshotItem{{Name: "交付质量", MaxScore: 100, HasWeightedFields: true, WeightedContribution: 42.13, FinalScore: 84.25}},
	}
	doc := buildExportDocument(models.EvaluationResultSnapshot{Version: 3, Checksum: "grade-checksum"}, payload)
	if doc.Grade != "B" {
		t.Fatalf("grade=%q, want B", doc.Grade)
	}
	if !strings.Contains(strings.Join(doc.Headers, "|"), "加权贡献") || !strings.Contains(strings.Join(doc.Headers, "|"), "评分等级") {
		t.Fatalf("default signoff headers=%v, want contribution and grade", doc.Headers)
	}
	if len(doc.Rows) != 1 || !strings.Contains(strings.Join(doc.Rows[0], "|"), "42.13") || !strings.Contains(strings.Join(doc.Rows[0], "|"), "B") {
		t.Fatalf("default signoff row=%v, want contribution and grade values", doc.Rows)
	}
	if !strings.Contains(resultPDFHTML, "评分等级：{{.Grade}}") {
		t.Fatal("PDF metadata must render canonical grade")
	}
	if !strings.Contains(resultPDFHTML, "{{range .Headers}}") || !strings.Contains(resultPDFHTML, "{{range .Rows}}") {
		t.Fatal("PDF table must render the selected export headers and rows")
	}
}

func TestExportDocumentDerivesGradeForHistoricalPayloadWithoutGrade(t *testing.T) {
	layout := defaultExportLayout(exportPresetFinalSignoff)
	payload := resultSnapshotPayload{EvaluationID: 13, TemplateName: "旧模板", Period: "monthly", Year: 2026, TotalScore: 95, ExportLayout: layout}
	doc := buildExportDocument(models.EvaluationResultSnapshot{Version: 1}, payload)
	if doc.Grade != GradeForScore(95) {
		t.Fatalf("grade=%q, want derived grade %q", doc.Grade, GradeForScore(95))
	}
}

func TestCompletedCanonicalSnapshotBackfillsExportEvidenceWithoutChangingSnapshot(t *testing.T) {
	setupConfirmationDB(t)
	department := models.Department{Name: "研发部"}
	models.DB.Create(&department)
	employee := models.Employee{Name: "加权员工", Email: "weighted-snapshot@test", Password: "hash", DepartmentID: department.ID, IsActive: true}
	models.DB.Create(&employee)
	dimensionID := uint(301)
	layoutRaw, err := json.Marshal(defaultExportLayout(exportPresetFinalSignoff))
	if err != nil {
		t.Fatal(err)
	}
	templateModel := models.KPITemplate{Name: "加权模板", Period: "monthly", Version: 1, Status: "published", IsActive: true, ExportLayoutJSON: string(layoutRaw)}
	models.DB.Create(&templateModel)
	models.DB.Create(&models.KPIDimension{ID: dimensionID, TemplateID: templateModel.ID, Name: "交付", Weight: 100})
	itemModel := models.KPIItem{TemplateID: templateModel.ID, DimensionID: &dimensionID, Name: "质量", MaxScore: 100, ItemWeight: 100, CompositeWeight: 100, Required: true}
	models.DB.Create(&itemModel)
	raw, err := buildWorkflowTemplateSnapshot(models.KPITemplate{
		ID: templateModel.ID, Name: templateModel.Name, Period: templateModel.Period, Version: 1,
		ExportLayout: defaultExportLayout(exportPresetFinalSignoff),
		Dimensions:   []models.KPIDimension{{ID: dimensionID, Name: "交付", Weight: 100}},
		Items:        []models.KPIItem{{ID: itemModel.ID, Name: itemModel.Name, MaxScore: 100, ItemWeight: 100, CompositeWeight: 100, Required: true, DimensionID: &dimensionID}},
	}, []workflowScoringAssignment{{ItemID: itemModel.ID, ScorerType: "PM", UserID: employee.ID, UserName: "项目经理"}})
	if err != nil {
		t.Fatal(err)
	}
	evaluation := models.KPIEvaluation{EmployeeID: employee.ID, TemplateID: templateModel.ID, Period: "monthly", Year: 2026, Status: workflowStateCompleted, CalculationVersion: canonicalCalculationVersion, TemplateSnapshotJSON: raw}
	models.DB.Create(&evaluation)
	score := 84.0
	models.DB.Create(&models.EvaluationScoringTask{EvaluationID: evaluation.ID, ItemID: itemModel.ID, ScorerUserID: employee.ID, ScorerType: "PM", Required: true, Status: "submitted", Score: &score})
	if _, _, err := persistCanonicalEvaluationResult(models.DB, evaluation.ID, employee.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadEvaluationForSnapshot(models.DB, evaluation.ID)
	if err != nil {
		t.Fatal(err)
	}
	canonical, _, _, err := buildCanonicalResultSnapshotWithDB(models.DB, loaded)
	if err != nil {
		t.Fatal(err)
	}
	canonical.Grade = ""
	for index := range canonical.Items {
		canonical.Items[index].HasWeightedFields = false
		canonical.Items[index].WeightedContribution = 0
	}
	legacyRaw, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	stored := models.EvaluationResultSnapshot{EvaluationID: evaluation.ID, Version: 1, SnapshotJSON: string(legacyRaw), Checksum: "immutable-old-checksum", CreatedBy: employee.ID}
	models.DB.Create(&stored)
	loadedSnapshot, payload, err := GetOrCreateResultSnapshot(models.DB, evaluation.ID, employee.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedSnapshot.ID != stored.ID || loadedSnapshot.Checksum != stored.Checksum {
		t.Fatalf("snapshot identity changed: loaded=%+v stored=%+v", loadedSnapshot, stored)
	}
	if payload.Grade == "" || !payload.Items[0].HasWeightedFields || payload.Items[0].WeightedContribution != 84 {
		t.Fatalf("canonical export evidence was not restored: %+v", payload)
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
	var exportPayload ExportResponse
	if err := json.Unmarshal(response.Body.Bytes(), &exportPayload); err != nil {
		t.Fatal(err)
	}
	if exportPayload.RequestID == "" || exportPayload.ResultVersion == 0 || exportPayload.Checksum == "" {
		t.Fatalf("export diagnostics missing: %+v", exportPayload)
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
