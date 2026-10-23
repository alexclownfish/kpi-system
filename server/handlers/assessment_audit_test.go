package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAssessmentAuditDB(t *testing.T) (*models.Employee, *models.KPIEvaluation, *models.KPIEvaluation) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:assessment-audit-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.Role{}, &models.Permission{}, &models.UserRole{}, &models.RolePermission{}, &models.DataScope{}, &models.RoleDataScope{},
		&models.Department{}, &models.Employee{}, &models.KPITemplate{}, &models.KPIDimension{}, &models.KPIItem{},
		&models.KPIEvaluation{}, &models.KPIScore{}, &models.EvaluationInvitation{}, &models.InvitedScore{},
		&models.EvaluationResponsibility{}, &models.EvaluationScoringTask{}, &models.EvaluationCalculationResult{}, &models.EvaluationResultSnapshot{},
		&models.EvaluationConfirmation{}, &models.EvaluationComment{},
		&models.AssessmentAuditFinding{}, &models.MigrationBatch{},
	); err != nil {
		t.Fatal(err)
	}
	models.DB = db
	department := models.Department{Name: "审计研发部"}
	db.Create(&department)
	employee := models.Employee{Name: "审计员工", Email: "audit-employee@test", Password: "hash", Position: "工程师", DepartmentID: department.ID, IsActive: true}
	db.Create(&employee)
	allScope := models.DataScope{Code: "ALL", Name: "全公司"}
	db.Create(&allScope)
	auditRole := models.Role{Code: "assessment_auditor", Name: "考核审计员"}
	db.Create(&auditRole)
	db.Create(&models.UserRole{UserID: employee.ID, RoleID: auditRole.ID})
	db.Create(&models.RoleDataScope{RoleID: auditRole.ID, DataScopeID: allScope.ID})
	template := models.KPITemplate{ID: 1, Name: "审计模板", Period: "monthly", Version: 2, CalculationVersion: "v1"}
	db.Create(&template)
	item := models.KPIItem{ID: 10, TemplateID: template.ID, Name: "交付", MaxScore: 100, Required: true, ItemWeight: 100}
	db.Create(&item)
	validSnapshot, err := buildEvaluationTemplateSnapshotWithScorers(models.KPITemplate{ID: template.ID, Name: template.Name, Period: template.Period, Version: template.Version, CalculationVersion: template.CalculationVersion, Items: []models.KPIItem{item}}, map[string]uint{"PM": employee.ID})
	if err != nil {
		t.Fatal(err)
	}
	month := 9
	valid := models.KPIEvaluation{ID: 1, EmployeeID: employee.ID, TemplateID: template.ID, Period: "monthly", Year: 2026, Month: &month, Status: "pending", TemplateSnapshotJSON: validSnapshot}
	db.Create(&valid)
	score := float64(80)
	db.Create(&models.KPIScore{EvaluationID: valid.ID, ItemID: item.ID, FinalScore: &score})
	missingMonth := 10
	missingSnapshot := models.KPIEvaluation{ID: 2, EmployeeID: employee.ID, TemplateID: template.ID, Period: "monthly", Year: 2026, Month: &missingMonth, Status: "pending"}
	db.Create(&missingSnapshot)
	return &employee, &valid, &missingSnapshot
}

func TestPreviewAssessmentAuditIsReadOnlyAndDeterministic(t *testing.T) {
	actor, valid, missing := setupAssessmentAuditDB(t)
	count := func(model any) int64 {
		var value int64
		if err := models.DB.Model(model).Count(&value).Error; err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := []int64{count(&models.KPIEvaluation{}), count(&models.KPIScore{}), count(&models.EvaluationInvitation{}), count(&models.EvaluationScoringTask{})}

	run := func() map[uint][]string {
		gin.SetMode(gin.TestMode)
		router := gin.New()
		router.POST("/assessment-audits/preview", func(c *gin.Context) { c.Set("user_id", actor.ID); PreviewAssessmentAudit(c) })
		record := httptest.NewRecorder()
		request := httptest.NewRequest("POST", "/assessment-audits/preview", nil)
		router.ServeHTTP(record, request)
		if record.Code != 200 {
			t.Fatalf("status=%d body=%s", record.Code, record.Body.String())
		}
		var envelope struct {
			Data struct {
				Findings []assessmentAuditFindingResponse `json:"findings"`
			} `json:"data"`
		}
		if err := json.Unmarshal(record.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		result := map[uint][]string{}
		for _, finding := range envelope.Data.Findings {
			if finding.EvaluationID != nil {
				result[*finding.EvaluationID] = append(result[*finding.EvaluationID], finding.Code)
			}
		}
		for id := range result {
			sort.Strings(result[id])
		}
		return result
	}

	first := run()
	second := run()
	if fmt.Sprint(first) != fmt.Sprint(second) {
		t.Fatalf("audit findings changed: first=%v second=%v", first, second)
	}
	if _, ok := first[valid.ID]; !ok || len(first[valid.ID]) == 0 {
		t.Fatalf("valid evaluation missing findings: %v", first)
	}
	if _, ok := first[missing.ID]; !ok {
		t.Fatalf("missing-snapshot evaluation missing findings: %v", first)
	}
	after := []int64{count(&models.KPIEvaluation{}), count(&models.KPIScore{}), count(&models.EvaluationInvitation{}), count(&models.EvaluationScoringTask{})}
	for index := range before {
		if before[index] != after[index] {
			t.Fatalf("audit changed row count at %d: before=%v after=%v", index, before, after)
		}
	}
}

func TestPreviewAssessmentAuditRejectsMalformedFilter(t *testing.T) {
	actor, _, _ := setupAssessmentAuditDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/assessment-audits/preview", func(c *gin.Context) { c.Set("user_id", actor.ID); PreviewAssessmentAudit(c) })
	record := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/assessment-audits/preview", strings.NewReader(`{"evaluation_ids":`))
	router.ServeHTTP(record, request)
	if record.Code != 400 {
		t.Fatalf("status=%d body=%s", record.Code, record.Body.String())
	}
}

func TestPreviewAssessmentAuditAcceptsDraftStatus(t *testing.T) {
	actor, valid, _ := setupAssessmentAuditDB(t)
	if err := models.DB.Model(&models.KPIEvaluation{}).Where("id = ?", valid.ID).Update("status", "draft").Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/assessment-audits/preview", func(c *gin.Context) { c.Set("user_id", actor.ID); PreviewAssessmentAudit(c) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("POST", "/assessment-audits/preview", strings.NewReader(fmt.Sprintf(`{"evaluation_ids":[%d]}`, valid.ID))))
	if recorder.Code != 200 {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Data struct {
			Findings []assessmentAuditFindingResponse `json:"findings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	for _, finding := range envelope.Data.Findings {
		if finding.EvaluationID != nil && *finding.EvaluationID == valid.ID && finding.Code == "STATUS_INVALID" {
			t.Fatalf("draft status was incorrectly reported invalid: %s", recorder.Body.String())
		}
	}
}

func TestPreviewAssessmentAuditFindsInvalidCompletedResult(t *testing.T) {
	actor, _, _ := setupAssessmentAuditDB(t)
	item := models.KPIItem{ID: 20, TemplateID: 1, Name: "加权指标", MaxScore: 100, ItemWeight: 100, CompositeWeight: 100, Required: true, ScorerTypesJSON: `["PM"]`}
	weightedTemplate := models.KPITemplate{ID: 1, Name: "加权模板", Period: "monthly", Version: 3, CalculationVersion: canonicalCalculationVersion, Items: []models.KPIItem{item}}
	raw, err := buildWorkflowTemplateSnapshot(weightedTemplate, []workflowScoringAssignment{{ItemID: item.ID, ScorerType: "PM", UserID: actor.ID, UserName: actor.Name, SourceRule: "audit-test"}})
	if err != nil {
		t.Fatal(err)
	}
	month := 13
	evaluation := models.KPIEvaluation{ID: 3, EmployeeID: actor.ID, TemplateID: 1, Period: "monthly", Year: 2026, Month: &month, Status: workflowStateCompleted, TotalScore: 999, CalculationVersion: canonicalCalculationVersion, TemplateSnapshotJSON: raw}
	if err := models.DB.Create(&evaluation).Error; err != nil {
		t.Fatal(err)
	}
	parsed, err := parseEvaluationSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := models.DB.Create(&models.EvaluationCalculationResult{EvaluationID: evaluation.ID, Version: 1, CalculationVersion: canonicalCalculationVersion, SnapshotChecksum: parsed.Checksum, ResultJSON: `{}`, TotalScore: 80}).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/assessment-audits/preview", func(c *gin.Context) { c.Set("user_id", actor.ID); PreviewAssessmentAudit(c) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("POST", "/assessment-audits/preview", strings.NewReader(`{"evaluation_ids":[3],"include_completed":true}`)))
	if recorder.Code != 200 {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Data struct {
			Findings []assessmentAuditFindingResponse `json:"findings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, finding := range envelope.Data.Findings {
		if finding.EvaluationID != nil && *finding.EvaluationID == evaluation.ID {
			codes[finding.Code] = true
		}
	}
	for _, code := range []string{"PERIOD_INVALID", "TOTAL_SCORE_MISMATCH", "CONFIRMATION_MISSING"} {
		if !codes[code] {
			t.Fatalf("expected %s finding, got %v", code, codes)
		}
	}
}

func TestPreviewAssessmentAuditFindsForgedLegacyCompletedTotal(t *testing.T) {
	actor, valid, _ := setupAssessmentAuditDB(t)
	if err := models.DB.Model(&models.KPIEvaluation{}).Where("id = ?", valid.ID).Updates(map[string]any{"status": workflowStateCompleted, "total_score": 999}).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/assessment-audits/preview", func(c *gin.Context) { c.Set("user_id", actor.ID); PreviewAssessmentAudit(c) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("POST", "/assessment-audits/preview", strings.NewReader(fmt.Sprintf(`{"evaluation_ids":[%d],"include_completed":true}`, valid.ID))))
	if recorder.Code != 200 {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Data struct {
			Findings []assessmentAuditFindingResponse `json:"findings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	for _, finding := range envelope.Data.Findings {
		if finding.EvaluationID != nil && *finding.EvaluationID == valid.ID && finding.Code == "TOTAL_SCORE_MISMATCH" {
			return
		}
	}
	t.Fatalf("forged legacy completed total was not reported: %s", recorder.Body.String())
}

func TestPreviewAssessmentAuditOrphansAreGlobalOnlyAndReadErrorsAreFindings(t *testing.T) {
	actor, _, _ := setupAssessmentAuditDB(t)
	// Populate one row for every workflow child table. These rows have no
	// parent assessment/invitation and must be visible only to an ALL auditor.
	mustCreate := func(value any) {
		t.Helper()
		if err := models.DB.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	mustCreate(&models.EvaluationScoringTask{EvaluationID: 999, ItemID: 20, ScorerUserID: actor.ID, ScorerType: "PM"})
	mustCreate(&models.EvaluationCalculationResult{EvaluationID: 999, Version: 1, ResultJSON: `{}`})
	mustCreate(&models.EvaluationResultSnapshot{EvaluationID: 999, Version: 1, SnapshotJSON: `{}`, Checksum: "orphan", CreatedBy: actor.ID})
	mustCreate(&models.EvaluationConfirmation{EvaluationID: 999, SnapshotID: 999, Method: "online", ConfirmedBy: actor.ID})
	mustCreate(&models.EvaluationInvitation{EvaluationID: 999, InviterID: actor.ID, InviteeID: actor.ID})
	mustCreate(&models.EvaluationComment{EvaluationID: 999, UserID: actor.ID, Content: "orphan"})
	mustCreate(&models.InvitedScore{InvitationID: 999, ItemID: 20})

	previewFindings := func(userID uint) []assessmentAuditFindingResponse {
		gin.SetMode(gin.TestMode)
		router := gin.New()
		router.POST("/assessment-audits/preview", func(c *gin.Context) { c.Set("user_id", userID); PreviewAssessmentAudit(c) })
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest("POST", "/assessment-audits/preview", nil))
		if recorder.Code != 200 {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		var envelope struct {
			Data struct {
				Findings []assessmentAuditFindingResponse `json:"findings"`
			} `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		return envelope.Data.Findings
	}
	preview := func(userID uint) []string {
		findings := previewFindings(userID)
		codes := make([]string, 0)
		for _, finding := range findings {
			if finding.Category == "orphan" {
				codes = append(codes, finding.Code)
			}
		}
		sort.Strings(codes)
		return codes
	}
	globalCodes := preview(actor.ID)
	if len(globalCodes) != 7 {
		t.Fatalf("global auditor should see all orphan categories, got %v", globalCodes)
	}
	// Each orphan finding carries a deterministic, bounded sample of row IDs
	// so an administrator can identify stale records for controlled cleanup.
	// The preview remains read-only and exposes no row content.
	for _, finding := range previewFindings(actor.ID) {
		if finding.Category != "orphan" || finding.Code == "ORPHAN_COMMENTS_READ_FAILED" {
			continue
		}
		details, ok := finding.Details.(map[string]any)
		if !ok {
			t.Fatalf("orphan finding details should be an object: %+v", finding)
		}
		if count, ok := details["count"].(float64); !ok || count != 1 {
			t.Fatalf("orphan finding should report count=1: %+v", finding)
		}
		sampleIDs, ok := details["sample_ids"].([]any)
		if !ok || len(sampleIDs) != 1 || sampleIDs[0].(float64) < 1 {
			t.Fatalf("orphan finding should report one sample ID: %+v", finding)
		}
	}

	scoped := models.Employee{Name: "范围审计员", Email: "scoped-auditor@test", Password: "hash", Position: "工程师", DepartmentID: actor.DepartmentID, IsActive: true}
	mustCreate(&scoped)
	selfScope := models.DataScope{Code: "SELF", Name: "本人"}
	mustCreate(&selfScope)
	role := models.Role{Code: "scoped_auditor", Name: "范围审计员"}
	mustCreate(&role)
	mustCreate(&models.UserRole{UserID: scoped.ID, RoleID: role.ID})
	mustCreate(&models.RoleDataScope{RoleID: role.ID, DataScopeID: selfScope.ID})
	if scopedCodes := preview(scoped.ID); len(scopedCodes) != 0 {
		t.Fatalf("scoped auditor must not learn global orphan rows, got %v", scopedCodes)
	}

	if err := models.DB.Migrator().DropTable(&models.EvaluationComment{}); err != nil {
		t.Fatal(err)
	}
	globalCodes = preview(actor.ID)
	foundReadFailure := false
	for _, code := range globalCodes {
		if code == "ORPHAN_COMMENTS_READ_FAILED" {
			foundReadFailure = true
			break
		}
	}
	if !foundReadFailure {
		t.Fatalf("missing orphan table must be an error finding, got %v", globalCodes)
	}
}
