package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func catalogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newEvaluationWorkflowTestDB(t)
	if err := db.AutoMigrate(&models.JobPosition{}, &models.BusinessInitialization{}); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SourceWorkbook{}, &models.SourceSheet{}, &models.DepartmentMapping{}, &models.ImportDiagnostic{}, &models.FormulaCell{}, &models.HRAggregationRule{}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SEED_HONGXING_CATALOG", "")
	return db
}

func catalogCount(t *testing.T, db *gorm.DB, entity any, want int64) {
	t.Helper()
	var got int64
	if err := db.Model(entity).Count(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%T count=%d want=%d", entity, got, want)
	}
}

func TestHongxingCatalogSourceAndFreshInitialization(t *testing.T) {
	db := catalogTestDB(t)
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	catalogCount(t, db, &models.Department{}, 10)
	catalogCount(t, db, &models.JobPosition{}, 18)
	catalogCount(t, db, &models.KPITemplate{}, 18)
	catalogCount(t, db, &models.KPIItem{}, 137)
	catalogCount(t, db, &models.Employee{}, 0)
	catalogCount(t, db, &models.KPIEvaluation{}, 0)
	catalogCount(t, db, &models.EvaluationResponsibility{}, 0)
	catalogCount(t, db, &models.BusinessInitialization{}, 2)
	catalogCount(t, db, &models.AuditLog{}, 2)
	var definitions hongxingCatalog
	if err := json.Unmarshal(hongxingCatalogJSON, &definitions); err != nil {
		t.Fatal(err)
	}
	if definitions.SourceSHA256 != "50bcf21db2fb357a5a471ecef459353b974355c2b7191e3f920fa708c3735fb3" || definitions.SourceSheetCount != 21 || definitions.SourceTemplateCount != 18 || definitions.SourceDepartmentCount != 9 || len(definitions.SourceDepartments) != 9 {
		t.Fatal("source changed")
	}
	itemCount := 0
	for _, profile := range definitions.Profiles {
		for _, dimension := range profile.Dimensions {
			itemCount += len(dimension.Items)
		}
	}
	if itemCount != 137 {
		t.Fatalf("item count=%d want 137", itemCount)
	}
	if strings.Contains(string(hongxingCatalogJSON), "synthetic") || strings.Contains(string(hongxingCatalogJSON), `"score"`) {
		t.Fatal("scores included in business definitions")
	}
	for _, profile := range definitions.Profiles {
		var dept models.Department
		if err := db.Where("name = ?", profile.Department).First(&dept).Error; err != nil {
			t.Fatal(err)
		}
		var template models.KPITemplate
		if err := db.Where("department_id = ? AND position_type = ?", dept.ID, profile.Position).First(&template).Error; err != nil {
			t.Fatal(err)
		}
		template, err := loadCalculationTemplate(db, template.ID)
		if err != nil {
			t.Fatal(err)
		}
		if template.Period != "monthly" || template.CalculationVersion != canonicalCalculationVersion || template.ExportLayout.Orientation != "portrait" {
			t.Fatalf("incorrect template %s", template.Name)
		}
		if template.Status != "published" || !template.IsActive || template.SourceBlocked {
			t.Fatalf("valid rounded composite weights should be publishable: %s", template.Name)
		}
		if len(profile.SourceAnomalies) > 0 && len(template.SourceIssues) == 0 {
			t.Fatalf("source warnings should be retained: %s", template.Name)
		}
		if err := validateTemplateForPublish(template); err != nil {
			t.Fatalf("%s: %v", template.Name, err)
		}
		if len(template.Dimensions) != len(profile.Dimensions) {
			t.Fatal("dimension count mismatch")
		}
		itemIndex := 0
		for index, dimension := range profile.Dimensions {
			actual := template.Dimensions[index]
			if actual.Name != dimension.Name || actual.Weight != dimension.Weight {
				t.Fatal("dimension altered")
			}
			for _, item := range dimension.Items {
				actualItem := template.Items[itemIndex]
				if actualItem.Name != item.Name || actualItem.ItemWeight != item.ItemWeight || actualItem.ScoringGuidance != item.ScoringGuidance || actualItem.CompositeWeight != CompositeWeight(dimension.Weight, item.ItemWeight) || !actualItem.Required || actualItem.MaxScore != 100 {
					t.Fatalf("item altered %s", item.Name)
				}
				if strings.Join(actualItem.ScorerTypes, ",") != strings.Join(item.ScorerTypes, ",") {
					t.Fatal("scorer rule altered")
				}
				itemIndex++
			}
		}
	}
}

func TestHongxingCatalogPreservesHRMaintenanceAndExistingPeople(t *testing.T) {
	db := catalogTestDB(t)
	person := models.Employee{Name: "Existing", Email: "existing@example.test", Password: "unchanged"}
	if err := db.Create(&person).Error; err != nil {
		t.Fatal(err)
	}
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	var templates []models.KPITemplate
	db.Order("id").Find(&templates)
	if err := db.Model(&templates[0]).Updates(map[string]any{"name": "HR maintained", "status": "retired", "is_active": false}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&templates[1]).Error; err != nil {
		t.Fatal(err)
	}
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	catalogCount(t, db, &models.KPITemplate{}, 17)
	catalogCount(t, db, &models.Employee{}, 1)
	catalogCount(t, db, &models.AuditLog{}, 2)
	var preserved models.KPITemplate
	db.First(&preserved, templates[0].ID)
	if preserved.Name != "HR maintained" || preserved.Status != "retired" || preserved.IsActive {
		t.Fatal("HR changes overwritten")
	}
	db.First(&person, person.ID)
	if person.Password != "unchanged" {
		t.Fatal("existing credential changed")
	}
}

func TestHongxingCatalogAdoptsExistingProfileWithoutOverwriting(t *testing.T) {
	db := catalogTestDB(t)
	dept := models.Department{Name: "ID 工业设计部", Description: "HR maintained"}
	db.Create(&dept)
	template := models.KPITemplate{Name: "Custom template", DepartmentID: &dept.ID, PositionType: "部门负责人", Period: "monthly", Status: "retired"}
	db.Create(&template)
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	catalogCount(t, db, &models.Department{}, 10)
	catalogCount(t, db, &models.KPITemplate{}, 18)
	db.First(&template, template.ID)
	if template.Name != "Custom template" || template.Status != "retired" {
		t.Fatal("existing template overwritten")
	}
	db.First(&dept, dept.ID)
	if dept.Description != "HR maintained" {
		t.Fatal("department overwritten")
	}
}

func TestHongxingCatalogRollbackAndAmbiguousDepartment(t *testing.T) {
	db := catalogTestDB(t)
	var definitions hongxingCatalog
	json.Unmarshal(hongxingCatalogJSON, &definitions)
	definitions.Profiles[len(definitions.Profiles)-1].Dimensions[0].Items[0].ItemWeight = -1
	if err := seedHongxingCatalog(db, definitions); err == nil {
		t.Fatal("invalid seed accepted")
	}
	catalogCount(t, db, &models.KPITemplate{}, 0)
	catalogCount(t, db, &models.Department{}, 0)
	catalogCount(t, db, &models.BusinessInitialization{}, 0)
	for i := 0; i < 2; i++ {
		db.Create(&models.Department{Name: "ID 工业设计部"})
	}
	if err := SeedHongxingCatalog(db); err == nil {
		t.Fatal("ambiguous department accepted")
	}
	catalogCount(t, db, &models.KPITemplate{}, 0)
}

func TestHongxingCatalogLocalWeightWarningsPreserveSourceAndAllowPublication(t *testing.T) {
	db := catalogTestDB(t)
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	var templates []models.KPITemplate
	if err := db.Find(&templates).Error; err != nil {
		t.Fatal(err)
	}
	warned := 0
	for _, template := range templates {
		loaded, err := loadCalculationTemplate(db, template.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(loaded.SourceIssues) > 0 {
			warned++
			for _, issue := range loaded.SourceIssues {
				if issue["severity"] != "warning" || issue["cells"] == nil {
					t.Fatalf("warning provenance lost: %+v", issue)
				}
			}
		}
		if err := validateTemplateForPublish(loaded); err != nil {
			t.Fatalf("original source weights should validate: %v", err)
		}
	}
	if warned != 5 {
		t.Fatalf("warned templates=%d want 5", warned)
	}
}

func TestHongxingCatalogOptOutAndInvalidSwitch(t *testing.T) {
	db := catalogTestDB(t)
	t.Setenv("SEED_HONGXING_CATALOG", "false")
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	catalogCount(t, db, &models.Department{}, 0)
	t.Setenv("SEED_HONGXING_CATALOG", "tru")
	if err := SeedHongxingCatalog(db); err == nil {
		t.Fatal("invalid switch accepted")
	}
}

func TestHongxingPositionCleanupWhenDepartmentDeleted(t *testing.T) {
	db := catalogTestDB(t)
	previous := models.DB
	models.DB = db
	t.Cleanup(func() { models.DB = previous })
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.DELETE("/departments/:id", DeleteDepartment)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/departments/1", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("department with templates should be protected: %d %s", response.Code, response.Body.String())
	}
	catalogCount(t, db, &models.JobPosition{}, 18)
	department := models.Department{Name: "Deletable"}
	db.Create(&department)
	db.Create(&models.JobPosition{DepartmentID: department.ID, Name: "Custom"})
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/departments/"+fmt.Sprint(department.ID), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", response.Code, response.Body.String())
	}
	catalogCount(t, db, &models.JobPosition{}, 18)
	var orphanCount int64
	db.Model(&models.JobPosition{}).Where("department_id = ?", department.ID).Count(&orphanCount)
	if orphanCount != 0 {
		t.Fatal("deleted department positions retained")
	}
}

func TestHongxingPositionQueryPermissionsAndDepartmentBoundary(t *testing.T) {
	db := catalogTestDB(t)
	previous := models.DB
	models.DB = db
	t.Cleanup(func() { models.DB = previous })
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	var depts []models.Department
	db.Order("id").Find(&depts)
	role := models.Role{Code: "position_reader", Name: "Reader"}
	db.Create(&role)
	permission := models.Permission{Code: "department:view"}
	db.Create(&permission)
	db.Create(&models.RolePermission{RoleID: role.ID, PermissionID: permission.ID})
	person := models.Employee{Name: "Reader", Email: "reader@test", Password: "unused"}
	db.Create(&person)
	db.Create(&models.UserRole{UserID: person.ID, RoleID: role.ID})
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		path       string
		authorized bool
		status     int
	}{
		{"/positions/1", true, 200}, {"/positions/2", true, 200}, {"/positions/1", false, 403}, {"/positions/0", true, 400}, {"/positions/not-an-id", true, 400}, {"/positions/99999", true, 404},
	} {
		r := gin.New()
		r.GET("/positions/:id", func(c *gin.Context) {
			if test.authorized {
				c.Set("user_id", person.ID)
			} else {
				c.Set("user_id", uint(99999))
			}
		}, PermissionAnyMiddleware("department:view", "employee:create", "employee:edit"), GetDepartmentPositions)
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.status {
			t.Fatalf("%s status=%d body=%s", test.path, response.Code, response.Body.String())
		}
		if test.status == 200 {
			var body struct {
				Data []models.JobPosition `json:"data"`
			}
			json.Unmarshal(response.Body.Bytes(), &body)
			if len(body.Data) != 2 {
				t.Fatal("wrong positions")
			}
			for _, p := range body.Data {
				if p.DepartmentID != depts[0].ID && test.path == "/positions/1" {
					t.Fatal("wrong department")
				}
			}
		}
	}
}
