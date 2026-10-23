package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
)

func TestSourceWorkbookModelsMigrateAndValidationIsIdempotent(t *testing.T) {
	db := catalogTestDB(t)
	if err := db.AutoMigrate(&models.SourceWorkbook{}, &models.SourceSheet{}, &models.DepartmentMapping{}, &models.ImportDiagnostic{}, &models.FormulaCell{}, &models.HRAggregationRule{}); err != nil {
		t.Fatal(err)
	}
	first := models.SourceWorkbook{SourceName: "荭杏研发项目绩效考核表 (1).xlsx", SourceSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ParserVersion: "v2", Status: "validated", SheetCount: 21}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	second := models.SourceWorkbook{SourceName: first.SourceName, SourceSHA256: first.SourceSHA256, ParserVersion: "v2", Status: "validated", SheetCount: 21}
	if err := db.Create(&second).Error; err == nil {
		t.Fatal("duplicate source SHA unexpectedly accepted")
	}
}

func TestSourceWorkbookDiagnosticsSortAndSensitiveMetadataFiltering(t *testing.T) {
	db := catalogTestDB(t)
	workbook := models.SourceWorkbook{SourceName: "source.xlsx", SourceSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ParserVersion: "v2", Status: "validated", SheetCount: 21, SourceMetadataJSON: `{"path":"/private/secret.xlsx","password":"hidden"}`}
	if err := db.Create(&workbook).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ImportDiagnostic{WorkbookID: workbook.ID, Severity: "blocking", Code: "FORMULA_GAP", Message: "公式断档", RawValue: "secret-cell-value"}).Error; err != nil {
		t.Fatal(err)
	}
	old := models.DB
	models.DB = db
	t.Cleanup(func() { models.DB = old })
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/source-workbooks/:id/diagnostics", func(c *gin.Context) { c.Set("user_id", uint(1)); GetSourceWorkbookDiagnostics(c) })
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/source-workbooks/workbook-"+itoa(workbook.ID)+"/diagnostics", nil))
	if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte("password")) || bytes.Contains(response.Body.Bytes(), []byte("/private/secret")) {
		t.Fatalf("sensitive metadata leaked: %d %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			Diagnostics []models.ImportDiagnostic `json:"diagnostics"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || len(envelope.Data.Diagnostics) != 1 {
		t.Fatalf("unexpected diagnostics response: %s", response.Body.String())
	}
}

func TestSourceWorkbookRoutesRequireHRTemplatePermission(t *testing.T) {
	db := catalogTestDB(t)
	models.DB = db
	permission := models.Permission{Code: "kpi:view", Name: "查看模板"}
	role := models.Role{Code: "source_reader", Name: "来源读取员"}
	user := models.Employee{Name: "来源测试用户", Email: "source-reader@test", Password: "hash", Role: "employee", IsActive: true}
	if err := db.Create(&permission).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RolePermission{RoleID: role.ID, PermissionID: permission.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.UserRole{UserID: user.ID, RoleID: role.ID}).Error; err != nil {
		t.Fatal(err)
	}

	permissionRouter := func(userID uint) *gin.Engine {
		router := gin.New()
		router.GET("/source-workbooks/:id/diagnostics", func(c *gin.Context) { c.Set("user_id", userID) }, PermissionAnyMiddleware("kpi:view", "system:edit"), func(c *gin.Context) { c.Status(http.StatusNoContent) })
		return router
	}
	denied := httptest.NewRecorder()
	permissionRouter(user.ID+1).ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/source-workbooks/workbook-1/diagnostics", nil))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("unauthorized source preview status=%d body=%s", denied.Code, denied.Body.String())
	}
	allowed := httptest.NewRecorder()
	permissionRouter(user.ID).ServeHTTP(allowed, httptest.NewRequest(http.MethodGet, "/source-workbooks/workbook-1/diagnostics", nil))
	if allowed.Code != http.StatusNoContent {
		t.Fatalf("authorized source preview status=%d body=%s", allowed.Code, allowed.Body.String())
	}
}

func TestSourceWorkbookImportRollsBackCatalogOnInvalidMapping(t *testing.T) {
	db := catalogTestDB(t)
	workbook := models.SourceWorkbook{
		SourceName:    "荭杏研发项目绩效考核表 (1).xlsx",
		SourceSHA256:  "50bcf21db2fb357a5a471ecef459353b974355c2b7191e3f920fa708c3735fb3",
		ParserVersion: "v2", Status: "validated", SheetCount: 21,
	}
	if err := db.Create(&workbook).Error; err != nil {
		t.Fatal(err)
	}
	old := models.DB
	models.DB = db
	t.Cleanup(func() { models.DB = old })
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/source-workbooks/:id/import", func(c *gin.Context) {
		c.Set("user_id", uint(1))
		ImportSourceWorkbook(c)
	})
	body := `{"source_sha256":"50bcf21db2fb357a5a471ecef459353b974355c2b7191e3f920fa708c3735fb3","mode":"draft","explicit_department_mappings":[{"raw_value":"","department_name":"研发部"}]}`
	request := httptest.NewRequest(http.MethodPost, "/source-workbooks/"+sourceWorkbookID(workbook.ID)+"/import", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte("INVALID_DEPARTMENT_MAPPING")) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var stored models.SourceWorkbook
	if err := db.First(&stored, workbook.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != "validated" {
		t.Fatalf("failed import changed source status: %s", stored.Status)
	}
	var templates int64
	db.Model(&models.KPITemplate{}).Count(&templates)
	if templates != 0 {
		t.Fatalf("failed import left %d templates", templates)
	}
}

func itoa(value uint) string {
	if value == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for value > 0 {
		buf = append([]byte{byte('0' + value%10)}, buf...)
		value /= 10
	}
	return string(buf)
}
