package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

func importWorkbookRequest(t *testing.T, db *gorm.DB, workbook models.SourceWorkbook, mode string, acknowledge bool, mappings []map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	old := models.DB
	models.DB = db
	defer func() { models.DB = old }()
	body, err := json.Marshal(map[string]any{"source_sha256": workbook.SourceSHA256, "mode": mode, "allow_warning_diagnostics": acknowledge, "explicit_department_mappings": mappings})
	if err != nil {
		t.Fatal(err)
	}
	router := templateCalculationRouter(http.MethodPost, "/source-workbooks/:id/import", ImportSourceWorkbook)
	return performTemplateCalculationRequest(router, http.MethodPost, "/source-workbooks/"+sourceWorkbookID(workbook.ID)+"/import", string(body))
}

func legacySourceRecord(t *testing.T, db *gorm.DB) models.SourceWorkbook {
	t.Helper()
	var catalog hongxingCatalog
	if err := json.Unmarshal(hongxingCatalogJSON, &catalog); err != nil {
		t.Fatal(err)
	}
	workbook := models.SourceWorkbook{SourceName: catalog.SourceFilename, SourceSHA256: catalog.SourceSHA256, ParserVersion: "v2", Status: "validated", SheetCount: 21}
	if err := db.Create(&workbook).Error; err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range buildSourceDiagnostics(catalog) {
		diagnostic.WorkbookID = workbook.ID
		if err := db.Create(&diagnostic).Error; err != nil {
			t.Fatal(err)
		}
	}
	return workbook
}

func TestSourceWorkbookOriginalDiagnosticsUseSelectedWeightAndRealRowRules(t *testing.T) {
	var catalog hongxingCatalog
	if err := json.Unmarshal(hongxingCatalogJSON, &catalog); err != nil {
		t.Fatal(err)
	}
	diagnostics := buildSourceDiagnostics(catalog)
	if countBlockingDiagnostics(diagnostics) != 0 || len(diagnostics) != 22 {
		t.Fatalf("original diagnostics: blocking=%d total=%d want 0/22", countBlockingDiagnostics(diagnostics), len(diagnostics))
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity != "warning" || diagnostic.Coordinate == "" {
			t.Fatalf("source warning lost: %+v", diagnostic)
		}
		if diagnostic.Code == "HR_FORMULA_GAP" && diagnostic.IsRealDataRow {
			t.Fatal("numbered blank HR row treated as business data")
		}
	}
	catalog.HRSummaryDiagnostics = append(catalog.HRSummaryDiagnostics, hongxingHRDiagnostic{Sheet: "HR真实数据", Kind: "monthly", FormulaGaps: []hongxingFormulaGap{{Cell: "I5", Row: 5, Reason: "实际员工行缺公式", IsRealDataRow: true}}})
	if got := countBlockingDiagnostics(buildSourceDiagnostics(catalog)); got != 1 {
		t.Fatalf("real HR row gap blockers=%d want 1", got)
	}
	catalog.Profiles[0].Dimensions[0].Items[0].ItemWeight = 0
	if countBlockingDiagnostics(buildSourceDiagnostics(catalog)) < 2 {
		t.Fatal("invalid composite total was not blocked")
	}
}

func TestSourceWorkbookDraftImportNeverPublishesAndSameSourcePreservesPublished(t *testing.T) {
	for _, seeded := range []bool{false, true} {
		t.Run(fmt.Sprintf("seeded=%v", seeded), func(t *testing.T) {
			db := catalogTestDB(t)
			if seeded {
				if err := SeedHongxingCatalog(db); err != nil {
					t.Fatal(err)
				}
			}
			workbook := legacySourceRecord(t, db)
			response := importWorkbookRequest(t, db, workbook, "draft", false, nil)
			if response.Code != http.StatusCreated {
				t.Fatalf("import=%d %s", response.Code, response.Body.String())
			}
			var envelope struct {
				Data sourceWorkbookImportResponse `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			wantCreated, wantReused, status := 18, 0, "draft"
			if seeded {
				wantCreated, wantReused, status = 0, 18, "published"
			}
			if envelope.Data.CreatedTemplates != wantCreated || envelope.Data.ReusedTemplates != wantReused || envelope.Data.Mode != "draft" || len(envelope.Data.TemplateIDs) != 18 {
				t.Fatalf("incorrect import result: %+v", envelope.Data)
			}
			var templates []models.KPITemplate
			db.Find(&templates)
			for _, template := range templates {
				if template.Status != status || template.IsActive != seeded || template.SourceBlocked {
					t.Fatalf("draft import changed publication: %+v", template)
				}
			}
			catalogCount(t, db, &models.KPIItem{}, 137)
			retry := importWorkbookRequest(t, db, workbook, "draft", false, nil)
			if retry.Code != http.StatusOK || !bytes.Contains(retry.Body.Bytes(), []byte("\"reused_templates\":18")) {
				t.Fatalf("duplicate import=%d %s", retry.Code, retry.Body.String())
			}
		})
	}
}

func TestSourceWorkbookPublishAcknowledgesWarningsAndRefusesExistingDraftReuse(t *testing.T) {
	db := catalogTestDB(t)
	workbook := legacySourceRecord(t, db)
	response := importWorkbookRequest(t, db, workbook, "publish", false, nil)
	if response.Code != http.StatusConflict || !bytes.Contains(response.Body.Bytes(), []byte("IMPORT_WARNINGS_NOT_ACKNOWLEDGED")) {
		t.Fatalf("unacknowledged warnings=%d %s", response.Code, response.Body.String())
	}
	catalogCount(t, db, &models.KPITemplate{}, 0)
	response = importWorkbookRequest(t, db, workbook, "publish", true, nil)
	if response.Code != http.StatusCreated {
		t.Fatalf("publish=%d %s", response.Code, response.Body.String())
	}
	var active int64
	db.Model(&models.KPITemplate{}).Where("status = 'published' AND is_active = ?", true).Count(&active)
	if active != 18 {
		t.Fatalf("published=%d want 18", active)
	}
	// Simulate a source validated before a bootstrap draft was explicitly
	// published. Import must not claim success while silently leaving it draft.
	db.Model(&models.SourceWorkbook{}).Where("id = ?", workbook.ID).Update("status", "validated")
	var template models.KPITemplate
	db.First(&template)
	db.Model(&template).Updates(map[string]any{"status": "draft", "is_active": false})
	response = importWorkbookRequest(t, db, workbook, "publish", true, nil)
	if response.Code != http.StatusConflict || !bytes.Contains(response.Body.Bytes(), []byte("IMPORT_PUBLISH_CONFLICT")) {
		t.Fatalf("draft reuse reported publish success=%d %s", response.Code, response.Body.String())
	}
	db.First(&workbook, workbook.ID)
	if workbook.Status != "validated" {
		t.Fatal("failed publication changed import marker")
	}
}

func TestSourceWorkbookChangedSHAPublishesNewRevisionAndRetiresPrevious(t *testing.T) {
	db := catalogTestDB(t)
	first := sourceParserTestWorkbook(t, nil)
	workbook, _, err := validateSourceWorkbookRecord(db, 1, first, sourceParserSHA(first), "first.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if response := importWorkbookRequest(t, db, workbook, "publish", true, nil); response.Code != http.StatusCreated {
		t.Fatalf("first import=%d %s", response.Code, response.Body.String())
	}
	var old models.KPITemplate
	db.First(&old)
	second := sourceParserTestWorkbook(t, func(book *excelize.File) {
		book.SetCellValue("01-ID工业设计-工程师", "D12", "修订指标原文")
		book.SetCellValue("01-ID工业设计-工程师", "E12", 55)
		book.SetCellValue("01-ID工业设计-工程师", "E13", 45)
	})
	revised, _, err := validateSourceWorkbookRecord(db, 1, second, sourceParserSHA(second), "revised.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if response := importWorkbookRequest(t, db, revised, "publish", true, nil); response.Code != http.StatusCreated {
		t.Fatalf("revised publish=%d %s", response.Code, response.Body.String())
	}
	var retired, next models.KPITemplate
	db.First(&retired, old.ID)
	db.Where("source_sha256 = ?", revised.SourceSHA256).First(&next)
	if retired.Status != "retired" || retired.IsActive || retired.Version != old.Version || retired.SourceSHA256 != old.SourceSHA256 {
		t.Fatalf("previous version not retained as history: %+v", retired)
	}
	if next.Version != old.Version+1 || next.Status != "published" || !next.IsActive || next.SourceSHA256 != revised.SourceSHA256 {
		t.Fatalf("revised version not active: %+v", next)
	}
	var active int64
	db.Model(&models.KPITemplate{}).Where("department_id = ? AND LOWER(position_type) = LOWER(?) AND period = ? AND status = ? AND is_active = ?", *next.DepartmentID, next.PositionType, next.Period, "published", true).Count(&active)
	if active != 1 {
		t.Fatalf("matching key has %d active versions", active)
	}
	var item models.KPIItem
	db.Where("template_id = ?", next.ID).Order("`order`, id").First(&item)
	if item.Name != "修订指标原文" || item.ItemWeight != 55 {
		t.Fatalf("revised source was ignored: %+v", item)
	}
}

func TestSourceWorkbookRevalidationRefreshesStaleDiagnosticsAndPreservesSnapshot(t *testing.T) {
	db := catalogTestDB(t)
	data := sourceParserTestWorkbook(t, nil)
	workbook := models.SourceWorkbook{SourceName: "original.xlsx", SourceSHA256: sourceParserSHA(data), ParserVersion: "v2", Status: "imported", SheetCount: 4}
	db.Create(&workbook)
	sheet := models.SourceSheet{WorkbookID: workbook.ID, Name: "HR汇总（季度）", Kind: "hr_quarterly", DiagnosticStatus: "blocking"}
	db.Create(&sheet)
	db.Create(&models.ImportDiagnostic{WorkbookID: workbook.ID, SheetID: &sheet.ID, Severity: "blocking", Code: "HR_FORMULA_GAP", Coordinate: "I17", Message: "old false blocker"})
	template := models.KPITemplate{Name: "HR maintained", Status: "published", IsActive: true, SourceSHA256: workbook.SourceSHA256}
	db.Create(&template)
	refreshed, response, err := validateSourceWorkbookRecord(db, 99, data, workbook.SourceSHA256, "different-name.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.ID != workbook.ID || refreshed.Status != "imported" || response.CanImportDraft || countBlockingDiagnostics(response.Diagnostics) != 0 || len(response.Diagnostics) != 5 {
		t.Fatalf("same SHA refresh: workbook=%+v response=%+v", refreshed, response)
	}
	snapshot := refreshed.SourceMetadataJSON
	refreshed, _, err = validateSourceWorkbookRecord(db, 99, data, workbook.SourceSHA256, "renamed-again.xlsx")
	if err != nil || refreshed.SourceMetadataJSON != snapshot || !strings.Contains(snapshot, `"source_filename":"original.xlsx"`) {
		t.Fatalf("immutable snapshot replaced: err=%v", err)
	}
	var shared models.FormulaCell
	if err := db.Where("workbook_id = ? AND coordinate = ? AND formula_type = ?", workbook.ID, "I6", "shared").First(&shared).Error; err != nil || shared.ExpandedFormula != `=IF(H6="","",H6*G6)` {
		t.Fatalf("shared audit missing: %+v err=%v", shared, err)
	}
	var preserved models.KPITemplate
	db.First(&preserved, template.ID)
	if preserved.Name != template.Name || preserved.Status != template.Status || !preserved.IsActive {
		t.Fatal("source revalidation changed a business template")
	}
	catalogCount(t, db, &models.SourceWorkbook{}, 1)
}

func TestSourceWorkbookActualDepartmentMappingAndTrueBlockerDraft(t *testing.T) {
	db := catalogTestDB(t)
	target := models.Department{Name: "已维护的真实部门"}
	db.Create(&target)
	data := sourceParserTestWorkbook(t, func(book *excelize.File) {
		book.SetCellValue("01-ID工业设计-工程师", "G12", "尚未配置的评分人")
	})
	workbook, preview, err := validateSourceWorkbookRecord(db, 1, data, sourceParserSHA(data), "blocked.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if !preview.CanImportDraft || preview.CanPublish || countBlockingDiagnostics(preview.Diagnostics) != 1 {
		t.Fatalf("unknown scorer publication status=%+v", preview)
	}
	if response := importWorkbookRequest(t, db, workbook, "publish", true, nil); response.Code != http.StatusConflict {
		t.Fatalf("true blocker published=%d %s", response.Code, response.Body.String())
	}
	response := importWorkbookRequest(t, db, workbook, "draft", false, []map[string]any{{"raw_value": "真实来源部门", "department_id": target.ID}})
	if response.Code != http.StatusCreated {
		t.Fatalf("mapped draft=%d %s", response.Code, response.Body.String())
	}
	var template models.KPITemplate
	db.First(&template)
	if template.DepartmentID == nil || *template.DepartmentID != target.ID || !template.SourceBlocked || template.Status != "draft" || template.IsActive {
		t.Fatalf("mapping/blocker not applied: %+v", template)
	}
	catalogCount(t, db, &models.Department{}, 1)
	catalogCount(t, db, &models.DepartmentMapping{}, 1)
	var snapshot models.SourceWorkbook
	db.First(&snapshot, workbook.ID)
	catalog, err := sourceCatalogFromMetadata(snapshot.SourceMetadataJSON)
	if err != nil || catalog.Profiles[0].Department != "真实来源部门" {
		t.Fatal("explicit mapping rewrote original source snapshot")
	}
}

func TestSaveTemplateStructureRetainsSourceWarningsAndUnrelatedBlocks(t *testing.T) {
	setupTemplateCalculationDB(t)
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		code    string
		blocked bool
	}{{"dimension_item_weight_sum", false}, {"unknown_scorer", true}} {
		issues := fmt.Sprintf(`[{"code":%q,"cells":["E12"],"weight_sum":73}]`, tc.code)
		template := models.KPITemplate{Name: tc.code, Status: "draft", SourceBlocked: true, SourceIssuesJSON: issues}
		models.DB.Create(&template)
		body := `{"dimensions":[{"name":"维度","weight":100,"items":[{"name":"指标","item_weight":100,"required":true,"scorer_types":["PM"]}]}]}`
		response := performTemplateCalculationRequest(templateCalculationRouter(http.MethodPut, "/templates/:id/calculation-structure", SaveTemplateCalculationStructure), http.MethodPut, fmt.Sprintf("/templates/%d/calculation-structure", template.ID), body)
		if response.Code != http.StatusOK {
			t.Fatalf("save=%d %s", response.Code, response.Body.String())
		}
		var stored models.KPITemplate
		models.DB.First(&stored, template.ID)
		if stored.SourceBlocked != tc.blocked || !strings.Contains(stored.SourceIssuesJSON, `"E12"`) || !strings.Contains(stored.SourceIssuesJSON, tc.code) {
			t.Fatalf("source issues silently acknowledged: %+v", stored)
		}
	}
}
