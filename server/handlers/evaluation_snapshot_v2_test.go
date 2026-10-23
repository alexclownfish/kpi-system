package handlers

import (
	"testing"

	"dootask-kpi-server/models"
)

func TestWorkflowSnapshotCapturesSourceScorersSignoffLayoutAndChecksum(t *testing.T) {
	departmentID := uint(42)
	layout := defaultExportLayout(exportPresetFinalSignoff)
	template := models.KPITemplate{
		ID: 9, Name: "荭杏工程师模板", Version: 3, Period: "monthly", PositionType: "工程师",
		DepartmentID: &departmentID, SourceCatalogVersion: "hongxing-catalog-v2",
		SourceSHA256: "50bcf21db2fb357a5a471ecef459353b974355c2b7191e3f920fa708c3735fb3",
		ExportLayout: layout,
		Dimensions:   []models.KPIDimension{{ID: 1, Name: "交付", Weight: 100}},
		Items:        []models.KPIItem{{ID: 101, Name: "质量", MaxScore: 100, ItemWeight: 100, CompositeWeight: 100, Required: true, ScorerTypes: []string{"PM", "PRODUCT_MANAGER"}}},
	}
	raw, err := buildWorkflowTemplateSnapshot(template, []workflowScoringAssignment{
		{ItemID: 101, ScorerType: "PM", UserID: 12, UserName: "项目经理", SourceRule: "evaluation_responsibility"},
		{ItemID: 101, ScorerType: "PRODUCT_MANAGER", UserID: 13, UserName: "产品经理", SourceRule: "evaluation_responsibility"},
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseEvaluationSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.SourceSHA256 != template.SourceSHA256 || parsed.SourceCatalogVersion != template.SourceCatalogVersion {
		t.Fatalf("source provenance missing: %+v", parsed)
	}
	if parsed.DepartmentID == nil || *parsed.DepartmentID != departmentID || parsed.PositionType != "工程师" {
		t.Fatalf("organization snapshot missing: %+v", parsed)
	}
	if len(parsed.ResolvedScorers) != 2 || parsed.ResolvedScorers[0].ItemID != 101 {
		t.Fatalf("scorer snapshot missing: %+v", parsed.ResolvedScorers)
	}
	if len(parsed.SignoffRules) != 3 || parsed.SignoffRules[1].SignerType != "PM" || parsed.SignoffRules[2].SignerType != "PRODUCT_MANAGER" {
		t.Fatalf("engineer signoff snapshot mismatch: %+v", parsed.SignoffRules)
	}
	if parsed.ExportLayout.Orientation != "portrait" || parsed.CalculationVersion != canonicalCalculationVersion {
		t.Fatalf("layout/calculation snapshot mismatch: %+v %s", parsed.ExportLayout, parsed.CalculationVersion)
	}
	if len(parsed.Checksum) != 64 {
		t.Fatalf("checksum=%q", parsed.Checksum)
	}
	// Mutating the source template after creation must not affect the raw
	// assessment snapshot or its checksum.
	template.Name = "被修改后的模板"
	template.SourceSHA256 = "changed"
	reparsed, err := parseEvaluationSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if reparsed.TemplateName != "荭杏工程师模板" || reparsed.SourceSHA256 != parsed.SourceSHA256 || reparsed.Checksum != parsed.Checksum {
		t.Fatalf("historical snapshot changed: before=%+v after=%+v", parsed, reparsed)
	}
}

func TestWorkflowSnapshotDepartmentManagerSignoffUsesCTO(t *testing.T) {
	template := models.KPITemplate{ID: 1, Name: "负责人模板", Version: 1, Period: "monthly", PositionType: "部门负责人", Items: []models.KPIItem{{ID: 1, Name: "综合评价", MaxScore: 100, Required: true, ScorerTypes: []string{"CTO"}}}}
	raw, err := buildWorkflowTemplateSnapshot(template, []workflowScoringAssignment{{ItemID: 1, ScorerType: "CTO", UserID: 8, UserName: "CTO"}})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseEvaluationSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.SignoffRules) != 2 || parsed.SignoffRules[1].SignerType != "CTO" {
		t.Fatalf("leader signoff mismatch: %+v", parsed.SignoffRules)
	}
}
