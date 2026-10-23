package handlers

import (
	"strings"
	"testing"

	"dootask-kpi-server/models"
)

func TestEvaluationOutputsShareCanonicalPayloadAndPortraitLayout(t *testing.T) {
	layout := defaultExportLayout(exportPresetFinalSignoff)
	payload := resultSnapshotPayload{EvaluationID: 7, EmployeeName: "员工", Department: "研发部", Position: "工程师", Period: "monthly", Year: 2026, TemplateName: "研发模板", TotalScore: 84, ExportLayout: layout, Items: []resultSnapshotItem{{ItemID: 1, Name: "交付", MaxScore: 100, FinalScore: 84}}}
	snapshot := models.EvaluationResultSnapshot{ID: 8, EvaluationID: 7, Version: 2, Checksum: "checksum"}
	doc := buildExportDocument(snapshot, payload)
	if doc.Orientation != "portrait" || doc.TotalScore != "84.00" || len(doc.Rows) != 1 {
		t.Fatalf("document=%+v", doc)
	}
	if !strings.Contains(resultPDFHTML, "A4 portrait") {
		t.Fatal("single result PDF must stay portrait")
	}
	if !strings.Contains(resultPDFHTML, "@page{size:A4 portrait") {
		t.Fatal("shared result PDF renderer must declare A4 portrait page size")
	}
}

func TestSingleAndBatchExportDocumentsShareSelectedFieldsAndEvidence(t *testing.T) {
	layout := defaultExportLayout(exportPresetFullProcess)
	layout.Columns = []string{"dimension_name", "item_name", "scorer_name", "weighted_contribution", "final_score"}
	layout.SignatureLabels = []string{"员工签字", "项目经理签字"}
	payload := resultSnapshotPayload{
		EvaluationID: 7, EmployeeName: "员工", Department: "研发部", Position: "工程师", Period: "monthly", Year: 2026,
		TemplateName: "研发模板", TotalScore: 84, ExportLayout: layout,
		Items: []resultSnapshotItem{{DimensionName: "交付质量", Name: "交付", ScorerName: "项目经理", WeightedContribution: 84, FinalScore: 84, HasWeightedFields: true}},
	}
	snapshot := models.EvaluationResultSnapshot{ID: 8, EvaluationID: 7, Version: 2, Checksum: "evidence-checksum"}
	single := buildExportDocument(snapshot, payload)
	batch := buildExportDocument(snapshot, payload)
	if single.Orientation != "portrait" || single.TotalScore != "84.00" || single.Checksum != "evidence-checksum" {
		t.Fatalf("single evidence metadata=%+v", single)
	}
	if strings.Join(single.Headers, "|") != strings.Join(batch.Headers, "|") || strings.Join(single.Rows[0], "|") != strings.Join(batch.Rows[0], "|") || strings.Join(single.SignatureLabels, "|") != strings.Join(batch.SignatureLabels, "|") {
		t.Fatalf("single/batch export documents diverged: single=%+v batch=%+v", single, batch)
	}
	for _, required := range []string{"维度", "实际评分人", "加权贡献", "最终得分"} {
		if !strings.Contains(strings.Join(single.Headers, "|"), required) {
			t.Fatalf("shared export document missing %q: %v", required, single.Headers)
		}
	}
}
