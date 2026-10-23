package handlers

import (
	"strings"
	"testing"

	"dootask-kpi-server/models"
)

// Feature 013 keeps the single-result and batch-signoff paths on the same
// immutable snapshot document. This focused regression names the v2 contract
// explicitly so future export changes cannot silently drop provenance fields.
func TestResultExportV2SnapshotProvenanceAndPortraitParity(t *testing.T) {
	layout := defaultExportLayout(exportPresetFinalSignoff)
	layout.Columns = []string{"dimension_name", "item_name", "weighted_contribution", "grade", "final_score"}
	payload := resultSnapshotPayload{
		EvaluationID: 42,
		TemplateName: "荭杏研发模板",
		Period:       "monthly",
		Year:         2026,
		TotalScore:   88.4,
		Grade:        "A",
		ExportLayout: layout,
		Items: []resultSnapshotItem{{
			DimensionName:        "交付质量",
			Name:                 "版本质量",
			WeightedContribution: 17.68,
			FinalScore:           88.4,
			HasWeightedFields:    true,
		}},
	}
	snapshot := models.EvaluationResultSnapshot{ID: 9, EvaluationID: 42, Version: 3, Checksum: "source-sha-checksum"}
	single := buildExportDocument(snapshot, payload)
	batch := buildExportDocument(snapshot, payload)
	if single.Orientation != "portrait" || batch.Orientation != "portrait" {
		t.Fatalf("exports must be portrait: single=%q batch=%q", single.Orientation, batch.Orientation)
	}
	if single.Checksum != snapshot.Checksum || batch.Checksum != snapshot.Checksum {
		t.Fatalf("snapshot checksum was not preserved: single=%q batch=%q", single.Checksum, batch.Checksum)
	}
	if strings.Join(single.Headers, "|") != strings.Join(batch.Headers, "|") ||
		strings.Join(single.Rows[0], "|") != strings.Join(batch.Rows[0], "|") {
		t.Fatalf("single/batch document fields diverged: single=%+v batch=%+v", single, batch)
	}
	if !strings.Contains(strings.Join(single.Headers, "|"), "加权贡献") ||
		!strings.Contains(strings.Join(single.Headers, "|"), "评分等级") {
		t.Fatalf("v2 export omitted weighted evidence: %v", single.Headers)
	}
}
