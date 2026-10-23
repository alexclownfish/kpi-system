package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"dootask-kpi-server/models"
)

func TestParseEvaluationSnapshot(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantLegacy bool
		wantItems  int
		wantErr    string
	}{
		{name: "missing", raw: "", wantErr: "缺少模板快照"},
		{name: "malformed", raw: "{", wantErr: "不是有效JSON"},
		{
			name: "legacy",
			raw: mustSnapshotJSON(t, evaluationTemplateSnapshot{
				TemplateID: 1, TemplateName: "旧模板", TemplateVersion: 2, CalculationVersion: "v1", Period: "monthly",
				Items:   []models.KPIItem{{ID: 10, Name: "交付", MaxScore: 100, Required: true, ScorerTypes: []string{"PM"}}},
				Scorers: map[string]uint{"PM": 8},
			}),
			wantLegacy: true, wantItems: 1,
		},
		{
			name: "weighted v2",
			raw: mustSnapshotJSON(t, workflowEvaluationSnapshot{
				Template:           workflowTemplateIdentity{ID: 2, Name: "新模板", Version: 3, Period: "quarterly"},
				CalculationVersion: canonicalCalculationVersion,
				Dimensions:         []workflowSnapshotDimension{{ID: 1, Name: "质量", Weight: 100, Items: []workflowSnapshotItem{{ID: 20, Name: "缺陷", MaxScore: 100, ItemWeight: 100, CompositeWeight: 100, Required: true, ScorerTypes: []string{"PM"}, Aggregation: "average"}}}},
				ResolvedScorers:    []evaluationResolvedScorer{{ItemID: 20, ScorerType: "PM", UserID: 9}},
			}),
			wantItems: 1,
		},
		{
			name: "duplicate item",
			raw: mustSnapshotJSON(t, workflowEvaluationSnapshot{
				Template:           workflowTemplateIdentity{ID: 2, Name: "重复模板", Version: 1, Period: "monthly"},
				CalculationVersion: canonicalCalculationVersion,
				Dimensions:         []workflowSnapshotDimension{{ID: 1, Name: "质量", Weight: 100, Items: []workflowSnapshotItem{{ID: 20, Name: "A", MaxScore: 100}, {ID: 20, Name: "B", MaxScore: 100}}}},
			}),
			wantErr: "重复指标ID",
		},
		{
			name: "duplicate scorer",
			raw: mustSnapshotJSON(t, workflowEvaluationSnapshot{
				Template:           workflowTemplateIdentity{ID: 2, Name: "重复评分人", Version: 1, Period: "monthly"},
				CalculationVersion: canonicalCalculationVersion,
				Dimensions:         []workflowSnapshotDimension{{ID: 1, Name: "质量", Weight: 100, Items: []workflowSnapshotItem{{ID: 20, Name: "A", MaxScore: 100}}}},
				ResolvedScorers:    []evaluationResolvedScorer{{ItemID: 20, ScorerType: "PM", UserID: 9}, {ItemID: 20, ScorerType: "PM", UserID: 9}},
			}),
			wantErr: "重复评分人",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot, err := parseEvaluationSnapshot(test.raw)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("err=%v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Legacy != test.wantLegacy || len(snapshot.Items) != test.wantItems || len(snapshot.Checksum) != 64 {
				t.Fatalf("unexpected snapshot: %+v", snapshot)
			}
		})
	}
}

func TestWorkflowSnapshotDerivesResponsibilitySigningLabels(t *testing.T) {
	dimensionID := uint(10)
	template := models.KPITemplate{
		ID: 1, Name: "负责人模板", Version: 2, Period: "monthly",
		ExportLayout: defaultExportLayout(exportPresetFinalSignoff),
		Dimensions:   []models.KPIDimension{{ID: dimensionID, Name: "交付", Weight: 100}},
		Items:        []models.KPIItem{{ID: 11, Name: "质量", MaxScore: 100, ItemWeight: 100, CompositeWeight: 100, DimensionID: &dimensionID, ScorerTypes: []string{"PM", "CTO", "PRODUCT_MANAGER"}}},
	}
	raw, err := buildWorkflowTemplateSnapshot(template, []workflowScoringAssignment{
		{ItemID: 11, ScorerType: "PM", UserID: 21, UserName: "张项目"},
		{ItemID: 11, ScorerType: "CTO", UserID: 22, UserName: "李CTO"},
		{ItemID: 11, ScorerType: "PRODUCT_MANAGER", UserID: 23, UserName: "王产品"},
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseEvaluationSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"员工签字", "项目经理签字", "CTO签字", "产品经理签字"}
	if !sameStrings(parsed.ExportLayout.SignatureLabels, want) {
		t.Fatalf("derived labels=%v, want=%v", parsed.ExportLayout.SignatureLabels, want)
	}
	custom := template
	custom.ExportLayout.SignatureLabels = []string{"员工确认", "法务签字"}
	raw, err = buildWorkflowTemplateSnapshot(custom, []workflowScoringAssignment{{ItemID: 11, ScorerType: "PM", UserID: 21, UserName: "张项目"}})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = parseEvaluationSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !sameStrings(parsed.ExportLayout.SignatureLabels, custom.ExportLayout.SignatureLabels) {
		t.Fatalf("custom labels changed: %v", parsed.ExportLayout.SignatureLabels)
	}
}

func mustSnapshotJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
