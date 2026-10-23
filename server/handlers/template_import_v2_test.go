package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"
)

func TestTemplateImportV2CreateAndUpdateCannotActivateDraft(t *testing.T) {
	for _, activeField := range []string{"", `,"is_active":false`, `,"is_active":true`} {
		t.Run("request="+activeField, func(t *testing.T) {
			setupTemplateCalculationDB(t)
			body := `{"name":"待发布模板","period":"monthly"` + activeField + `}`
			response := performTemplateCalculationRequest(templateCalculationRouter(http.MethodPost, "/templates", CreateTemplate), http.MethodPost, "/templates", body)
			if response.Code != http.StatusCreated {
				t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
			}
			var envelope struct {
				Data models.KPITemplate `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Data.Status != "draft" || envelope.Data.IsActive {
				t.Fatalf("create returned active draft: %+v", envelope.Data)
			}
			var stored models.KPITemplate
			if err := models.DB.First(&stored, envelope.Data.ID).Error; err != nil {
				t.Fatal(err)
			}
			if stored.Status != "draft" || stored.IsActive {
				t.Fatalf("create persisted active draft: %+v", stored)
			}
			path := fmt.Sprintf("/templates/%d", stored.ID)
			response = performTemplateCalculationRequest(templateCalculationRouter(http.MethodPut, "/templates/:id", UpdateTemplate), http.MethodPut, path, `{"name":"已编辑草稿","is_active":true}`)
			if response.Code != http.StatusOK {
				t.Fatalf("update status=%d body=%s", response.Code, response.Body.String())
			}
			if err := models.DB.First(&stored, stored.ID).Error; err != nil {
				t.Fatal(err)
			}
			if stored.Name != "已编辑草稿" || stored.Status != "draft" || stored.IsActive {
				t.Fatalf("metadata update activated draft: %+v", stored)
			}
		})
	}
}

func TestTemplateImportV2CloneStaysInactiveAndPreservesPublishedSource(t *testing.T) {
	setupTemplateCalculationDB(t)
	source := models.KPITemplate{Name: "已发布模板", Period: "monthly", Status: "published", IsActive: true, Version: 3}
	if err := models.DB.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/templates/%d/clone", source.ID)
	response := performTemplateCalculationRequest(templateCalculationRouter(http.MethodPost, "/templates/:id/clone", CloneTemplate), http.MethodPost, path, `{}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("clone status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data models.KPITemplate `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	var clone models.KPITemplate
	if err := models.DB.First(&clone, envelope.Data.ID).Error; err != nil {
		t.Fatal(err)
	}
	if clone.ID == source.ID || clone.Status != "draft" || clone.IsActive || envelope.Data.IsActive || clone.Version != 4 {
		t.Fatalf("clone not an inactive draft: %+v", clone)
	}
	if err := models.DB.First(&source, source.ID).Error; err != nil {
		t.Fatal(err)
	}
	if source.Status != "published" || !source.IsActive || source.Version != 3 {
		t.Fatalf("clone altered published source: %+v", source)
	}
}

func TestTemplateImportV2PublishSwitchesDuplicateMatchKey(t *testing.T) {
	setupTemplateCalculationDB(t)
	department := models.Department{Name: "研发部"}
	if err := models.DB.Create(&department).Error; err != nil {
		t.Fatal(err)
	}
	published := models.KPITemplate{Name: "已发布版本", DepartmentID: &department.ID, PositionType: "工程师", Period: "monthly", Status: "published", IsActive: true, Version: 1}
	draft := models.KPITemplate{Name: "重复草稿", DepartmentID: &department.ID, PositionType: "工程师", Period: "monthly", Status: "draft", IsActive: false, Version: 2}
	for _, template := range []*models.KPITemplate{&published, &draft} {
		if err := models.DB.Create(template).Error; err != nil {
			t.Fatal(err)
		}
		if err := models.DB.Create(&models.KPIDimension{TemplateID: template.ID, Name: "交付", Weight: 100}).Error; err != nil {
			t.Fatal(err)
		}
		var dimension models.KPIDimension
		models.DB.Where("template_id = ?", template.ID).First(&dimension)
		if err := models.DB.Create(&models.KPIItem{TemplateID: template.ID, DimensionID: &dimension.ID, Name: "交付质量", MaxScore: 100, ItemWeight: 100, CompositeWeight: 100, Required: true, ScorerTypesJSON: `["PM"]`}).Error; err != nil {
			t.Fatal(err)
		}
	}
	router := templateCalculationRouter(http.MethodPost, "/templates/:id/publish", PublishTemplate)
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/templates/%d/publish", draft.ID), strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("duplicate publish status=%d body=%s", response.Code, response.Body.String())
	}
	var retired, publishedNow models.KPITemplate
	models.DB.First(&retired, published.ID)
	models.DB.First(&publishedNow, draft.ID)
	if retired.Status != "retired" || retired.IsActive {
		t.Fatalf("previous active version was not retired: %+v", retired)
	}
	if publishedNow.Status != "published" || !publishedNow.IsActive {
		t.Fatalf("draft was not published: %+v", publishedNow)
	}
	var active int64
	models.DB.Model(&models.KPITemplate{}).Where("department_id = ? AND LOWER(position_type) = LOWER(?) AND period = ? AND status = ? AND is_active = ?", department.ID, "工程师", "monthly", "published", true).Count(&active)
	if active != 1 {
		t.Fatalf("active versions=%d want 1", active)
	}
}

func TestTemplateImportV2RequiredItemNeedsScorerAndLeavesDraftAtomic(t *testing.T) {
	setupTemplateCalculationDB(t)
	template := models.KPITemplate{Name: "职责缺失", Period: "monthly", Status: "draft", Version: 1}
	if err := models.DB.Create(&template).Error; err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/templates/%d/calculation-structure", template.ID)
	body := `{"dimensions":[{"name":"交付","weight":100,"items":[{"name":"必须评分","item_weight":100,"required":true,"scorer_types":[] }]}]}`
	response := performTemplateCalculationRequest(templateCalculationRouter(http.MethodPut, "/templates/:id/calculation-structure", SaveTemplateCalculationStructure), http.MethodPut, path, body)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "评分人类型") {
		t.Fatalf("missing scorer status=%d body=%s", response.Code, response.Body.String())
	}
	var dimensions int64
	models.DB.Model(&models.KPIDimension{}).Where("template_id = ?", template.ID).Count(&dimensions)
	var items int64
	models.DB.Model(&models.KPIItem{}).Where("template_id = ?", template.ID).Count(&items)
	if dimensions != 0 || items != 0 {
		t.Fatalf("invalid template left partial structure dimensions=%d items=%d", dimensions, items)
	}
}
