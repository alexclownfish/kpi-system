package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDeleteTemplateCleansDimensionsAndItems(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:kpi-delete-template?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.KPITemplate{}, &models.KPIDimension{}, &models.KPIItem{}, &models.KPIEvaluation{}); err != nil {
		t.Fatal(err)
	}
	models.DB = db
	template := models.KPITemplate{Name: "模板删除测试", Period: "monthly"}
	if err := db.Create(&template).Error; err != nil {
		t.Fatal(err)
	}
	dimension := models.KPIDimension{TemplateID: template.ID, Name: "维度"}
	if err := db.Create(&dimension).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.KPIItem{TemplateID: template.ID, DimensionID: &dimension.ID, Name: "指标", MaxScore: 100}).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/templates/:id", DeleteTemplate)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("DELETE", "/templates/1", nil))
	if recorder.Code != 200 {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, model := range []any{&models.KPITemplate{}, &models.KPIDimension{}, &models.KPIItem{}} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rows remain in %T: %d", model, count)
		}
	}
}

func TestDeleteTemplateRejectsReferencedTemplateAndPreservesChildren(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:kpi-delete-template-reference?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.KPITemplate{}, &models.KPIDimension{}, &models.KPIItem{}, &models.KPIEvaluation{}); err != nil {
		t.Fatal(err)
	}
	models.DB = db
	template := models.KPITemplate{Name: "被引用模板", Period: "monthly"}
	if err := db.Create(&template).Error; err != nil {
		t.Fatal(err)
	}
	dimension := models.KPIDimension{TemplateID: template.ID, Name: "维度"}
	if err := db.Create(&dimension).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.KPIItem{TemplateID: template.ID, DimensionID: &dimension.ID, Name: "指标", MaxScore: 100}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.KPIEvaluation{TemplateID: template.ID, Period: "monthly", Year: 2026, Status: "pending"}).Error; err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.DELETE("/templates/:id", DeleteTemplate)
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest("DELETE", "/templates/"+uintStringForDeleteTest(template.ID), nil))
	if recorder.Code != 400 || !strings.Contains(recorder.Body.String(), "评估记录") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, model := range []any{&models.KPITemplate{}, &models.KPIDimension{}, &models.KPIItem{}} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("template child %T should remain, count=%d", model, count)
		}
	}
}

func TestDeleteTemplateRejectsActivePublishedVersion(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:kpi-delete-published-template?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.KPITemplate{}, &models.KPIDimension{}, &models.KPIItem{}, &models.KPIEvaluation{}); err != nil {
		t.Fatal(err)
	}
	models.DB = db
	template := models.KPITemplate{Name: "当前发布模板", Period: "monthly", Status: "published", IsActive: true}
	if err := db.Create(&template).Error; err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.DELETE("/templates/:id", DeleteTemplate)
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest("DELETE", "/templates/"+uintStringForDeleteTest(template.ID), nil))
	if recorder.Code != 409 || !strings.Contains(recorder.Body.String(), "当前发布模板不能直接删除") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var count int64
	db.Model(&models.KPITemplate{}).Where("id = ?", template.ID).Count(&count)
	if count != 1 {
		t.Fatal("published template was deleted")
	}
}
