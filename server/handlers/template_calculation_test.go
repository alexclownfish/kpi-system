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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTemplateCalculationDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:template-calculation-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	models.DB = db
	if err := db.AutoMigrate(&models.Department{}, &models.Employee{}, &models.KPITemplate{}, &models.KPIDimension{}, &models.KPIItem{}, &models.AuditLog{}); err != nil {
		t.Fatal(err)
	}
}

func templateCalculationRouter(method, path string, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Handle(method, path, func(c *gin.Context) { c.Set("user_id", uint(1)) }, handler)
	return router
}

func performTemplateCalculationRequest(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestSaveTemplateStructureAndPublish(t *testing.T) {
	setupTemplateCalculationDB(t)
	department := models.Department{Name: "软件部"}
	models.DB.Create(&department)
	template := models.KPITemplate{Name: "软件工程师月度", DepartmentID: &department.ID, PositionType: "工程师", Period: "monthly", Status: "draft", Version: 1}
	models.DB.Create(&template)
	path := fmt.Sprintf("/templates/%d/calculation-structure", template.ID)
	body := `{"dimensions":[{"name":"交付及时性","weight":20,"items":[{"name":"任务按期完成率","item_weight":100,"required":true,"scorer_types":["PM"]}]},{"name":"交付质量","weight":40,"items":[{"name":"缺陷密度","item_weight":50,"required":true,"scorer_types":["PM"]},{"name":"负责人评价","item_weight":50,"required":true,"scorer_types":["DEPARTMENT_MANAGER"]}]},{"name":"协同与变更","weight":15,"items":[{"name":"联调响应","item_weight":100,"required":true,"scorer_types":["PM"]}]},{"name":"成本与规范","weight":5,"items":[{"name":"代码规范","item_weight":100,"required":true,"scorer_types":["PM"]}]},{"name":"产品经理评价","weight":20,"items":[{"name":"需求符合度","item_weight":100,"required":true,"scorer_types":["PRODUCT_MANAGER"]}]}]}`
	response := performTemplateCalculationRequest(templateCalculationRouter(http.MethodPut, "/templates/:id/calculation-structure", SaveTemplateCalculationStructure), http.MethodPut, path, body)
	if response.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", response.Code, response.Body.String())
	}
	publishPath := fmt.Sprintf("/templates/%d/publish", template.ID)
	response = performTemplateCalculationRequest(templateCalculationRouter(http.MethodPost, "/templates/:id/publish", PublishTemplate), http.MethodPost, publishPath, "")
	if response.Code != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", response.Code, response.Body.String())
	}
	var saved models.KPITemplate
	models.DB.First(&saved, template.ID)
	if saved.Status != "published" || !saved.IsActive {
		t.Fatalf("unexpected published template: %+v", saved)
	}
}

func TestSaveTemplateStructureRejectsInvalidWeights(t *testing.T) {
	setupTemplateCalculationDB(t)
	template := models.KPITemplate{Name: "错误模板", Period: "monthly", Status: "draft"}
	models.DB.Create(&template)
	path := fmt.Sprintf("/templates/%d/calculation-structure", template.ID)
	body := `{"dimensions":[{"name":"交付","weight":90,"items":[{"name":"指标","item_weight":100,"required":true,"scorer_types":["PM"]}]}]}`
	response := performTemplateCalculationRequest(templateCalculationRouter(http.MethodPut, "/templates/:id/calculation-structure", SaveTemplateCalculationStructure), http.MethodPut, path, body)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var count int64
	models.DB.Model(&models.KPIDimension{}).Where("template_id = ?", template.ID).Count(&count)
	if count != 0 {
		t.Fatal("invalid structure must not be partially written")
	}
}

func TestSavePublishPreservesSourceHeadWeights(t *testing.T) {
	for _, fixture := range sourceHeadWeightCases(t) {
		t.Run(fixture.Sheet, func(t *testing.T) {
			setupTemplateCalculationDB(t)
			department := models.Department{Name: fixture.Sheet}
			models.DB.Create(&department)
			template := models.KPITemplate{Name: fixture.Sheet, DepartmentID: &department.ID, PositionType: fixture.Position, Period: "monthly", Status: "draft", Version: 1}
			models.DB.Create(&template)
			body, err := json.Marshal(templateStructureRequest{Dimensions: fixture.Dimensions})
			if err != nil {
				t.Fatal(err)
			}
			response := performTemplateCalculationRequest(templateCalculationRouter(http.MethodPut, "/templates/:id/calculation-structure", SaveTemplateCalculationStructure), http.MethodPut, fmt.Sprintf("/templates/%d/calculation-structure", template.ID), string(body))
			if response.Code != http.StatusOK {
				t.Fatalf("save=%d %s", response.Code, response.Body.String())
			}
			saved, err := loadCalculationTemplate(models.DB, template.ID)
			if err != nil {
				t.Fatal(err)
			}
			for index, original := range fixture.Dimensions {
				if saved.Dimensions[index].Weight != original.Weight {
					t.Fatal("dimension weight normalized")
				}
				for _, item := range original.Items {
					found := false
					for _, actual := range saved.Items {
						if actual.Name == item.Name {
							found = true
							if actual.ItemWeight != item.ItemWeight || actual.CompositeWeight != CompositeWeight(original.Weight, item.ItemWeight) {
								t.Fatal("item weight altered")
							}
						}
					}
					if !found {
						t.Fatalf("missing item %s", item.Name)
					}
				}
			}
			response = performTemplateCalculationRequest(templateCalculationRouter(http.MethodPost, "/templates/:id/publish", PublishTemplate), http.MethodPost, fmt.Sprintf("/templates/%d/publish", template.ID), "")
			if response.Code != http.StatusOK {
				t.Fatalf("publish=%d %s", response.Code, response.Body.String())
			}
		})
	}
}
