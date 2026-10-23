package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCanonicalSnapshotKeepsHistoricalLabelsAfterTemplateEdit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:snapshot-history-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Department{}, &models.Employee{}, &models.KPITemplate{}, &models.KPIDimension{}, &models.KPIItem{}, &models.KPIEvaluation{}, &models.KPIScore{}, &models.EvaluationScoringTask{}, &models.EvaluationCalculationResult{}, &models.EvaluationResultSnapshot{}, &models.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	models.DB = db
	department := models.Department{Name: "研发部"}
	db.Create(&department)
	employee := models.Employee{Name: "历史员工", Email: "history@test", Password: "hash", DepartmentID: department.ID, IsActive: true}
	db.Create(&employee)
	dimensionID := uint(20)
	originalLayout := defaultExportLayout(exportPresetFinalSignoff)
	originalLayout.Title = "历史签字表"
	originalLayout.SignatureLabels = []string{"员工签字", "PM签字"}
	layoutJSON, _, err := marshalExportLayout(originalLayout)
	if err != nil {
		t.Fatal(err)
	}
	template := models.KPITemplate{ID: 1, Name: "研发模板", Period: "monthly", Version: 1, Status: "published", IsActive: true, ExportLayoutJSON: layoutJSON}
	db.Create(&template)
	db.Create(&models.KPIDimension{ID: dimensionID, TemplateID: template.ID, Name: "交付", Weight: 100})
	db.Create(&models.KPIItem{ID: 10, TemplateID: template.ID, DimensionID: &dimensionID, Name: "旧指标名称", MaxScore: 100, ItemWeight: 100, CompositeWeight: 100, Required: true})
	raw, err := buildWorkflowTemplateSnapshot(models.KPITemplate{ID: template.ID, Name: template.Name, Period: template.Period, Version: 1, ExportLayout: originalLayout, Dimensions: []models.KPIDimension{{ID: dimensionID, Name: "交付", Weight: 100}}, Items: []models.KPIItem{{ID: 10, Name: "旧指标名称", MaxScore: 100, ItemWeight: 100, CompositeWeight: 100, Required: true, DimensionID: &dimensionID}}}, []workflowScoringAssignment{{ItemID: 10, ScorerType: "PM", UserID: 2, UserName: "PM", SourceRule: "evaluation_responsibility"}})
	if err != nil {
		t.Fatal(err)
	}
	calculated := canonicalEvaluationResult{EvaluationID: 1, CalculationVersion: canonicalCalculationVersion, SnapshotChecksum: "source", ResultVersion: 1, Items: []canonicalItemResult{{ItemID: 10, Name: "旧指标名称", MaxScore: 100, Required: true, CompositeWeight: 100, AggregatedScore: floatPointer(88), Contribution: 88}}, TotalScore: 88, Grade: "A"}
	resultJSON, _ := json.Marshal(calculated)
	evaluation := models.KPIEvaluation{ID: 1, EmployeeID: employee.ID, TemplateID: template.ID, Period: "monthly", Year: 2026, Status: "completed", CalculationVersion: canonicalCalculationVersion, TemplateSnapshotJSON: raw}
	db.Create(&evaluation)
	db.Create(&models.EvaluationCalculationResult{EvaluationID: evaluation.ID, Version: 1, CalculationVersion: canonicalCalculationVersion, SnapshotChecksum: "source", ResultJSON: string(resultJSON), TotalScore: 88, Grade: "A"})
	first, payload, err := GetOrCreateResultSnapshot(db, evaluation.ID, employee.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 || len(payload.Items) != 1 || payload.Items[0].Name != "旧指标名称" {
		t.Fatalf("first snapshot=%+v payload=%+v", first, payload)
	}
	if !reflect.DeepEqual(payload.ExportLayout, originalLayout) {
		t.Fatalf("initial snapshot layout=%+v", payload.ExportLayout)
	}
	// The UI must clone first; bypassing it cannot mutate the published source.
	response := performTemplateCalculationRequest(templateCalculationRouter(http.MethodPut, "/templates/:id", UpdateTemplate), http.MethodPut, "/templates/1", `{"export_layout":{"title":"非法改写"}}`)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "PUBLISHED_TEMPLATE_IMMUTABLE") {
		t.Fatalf("published mutation=%d %s", response.Code, response.Body.String())
	}
	response = performTemplateCalculationRequest(templateCalculationRouter(http.MethodPost, "/templates/:id/clone", CloneTemplate), http.MethodPost, "/templates/1/clone", `{"name":"研发模板 V2"}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("clone=%d %s", response.Code, response.Body.String())
	}
	var cloned struct {
		Data models.KPITemplate `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &cloned); err != nil {
		t.Fatal(err)
	}
	if cloned.Data.Status != "draft" || cloned.Data.IsActive || cloned.Data.Version != 2 || !reflect.DeepEqual(cloned.Data.ExportLayout, originalLayout) {
		t.Fatalf("cloned template=%+v", cloned.Data)
	}
	newLayout := defaultExportLayout(exportPresetFullProcess)
	newLayout.Title = "下一版签字表"
	newLayout.Orientation = "landscape"
	newLayout.SignatureLabels = []string{"CTO签字"}
	body, _ := json.Marshal(map[string]any{"export_layout": newLayout})
	response = performTemplateCalculationRequest(templateCalculationRouter(http.MethodPut, "/templates/:id", UpdateTemplate), http.MethodPut, fmt.Sprintf("/templates/%d", cloned.Data.ID), string(body))
	if response.Code != http.StatusOK {
		t.Fatalf("draft layout mutation=%d %s", response.Code, response.Body.String())
	}
	var updated struct {
		Data models.KPITemplate `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if len(updated.Data.Dimensions) != 1 || len(updated.Data.Items) != 1 {
		t.Fatalf("layout save lost calculation structure: %+v", updated.Data)
	}
	var saved models.KPITemplate
	db.First(&saved, cloned.Data.ID)
	hydrateTemplateLayout(&saved)
	if !reflect.DeepEqual(saved.ExportLayout, newLayout) {
		t.Fatalf("draft layout not persisted: %+v", saved.ExportLayout)
	}
	newRaw, err := buildWorkflowTemplateSnapshot(updated.Data, []workflowScoringAssignment{{ItemID: updated.Data.Items[0].ID, ScorerType: "PM", UserID: 2, UserName: "PM", SourceRule: "evaluation_responsibility"}})
	if err != nil {
		t.Fatal(err)
	}
	newSnapshot, err := parseEvaluationSnapshot(newRaw)
	if err != nil || newSnapshot.TemplateVersion != 2 || !reflect.DeepEqual(newSnapshot.ExportLayout, newLayout) {
		t.Fatalf("new snapshot=%+v err=%v", newSnapshot, err)
	}
	db.Model(&models.KPIItem{}).Where("id = ?", 10).Update("name", "新指标名称")
	second, payload, err := GetOrCreateResultSnapshot(db, evaluation.ID, employee.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.Checksum != first.Checksum || payload.Items[0].Name != "旧指标名称" || !reflect.DeepEqual(payload.ExportLayout, originalLayout) {
		t.Fatalf("historical snapshot changed after template edit: first=%+v second=%+v payload=%+v", first, second, payload)
	}
}
