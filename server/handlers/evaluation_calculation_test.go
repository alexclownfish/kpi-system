package handlers

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"dootask-kpi-server/models"
)

// TestCanonicalCalculationCoversEveryHongxingPositionProfile turns every
// source profile into a small immutable snapshot. This keeps calculation
// coverage tied to the workbook catalog instead of testing only the three
// department-head fixtures. The software-leader source contains intentionally
// anomalous internal item sums (73% and 140%), but its rounded composite
// weights still total 100%. Per the weight policy, calculation preserves that
// source rule; publication is blocked until HR repairs the source template.
func TestCanonicalCalculationCoversEveryHongxingPositionProfile(t *testing.T) {
	raw, err := os.ReadFile("data/hongxing-catalog-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Profiles []struct {
			Sheet      string `json:"sheet"`
			Dimensions []struct {
				Name   string  `json:"name"`
				Weight float64 `json:"weight"`
				Items  []struct {
					Name       string  `json:"name"`
					ItemWeight float64 `json:"item_weight"`
					Required   bool    `json:"required"`
				} `json:"items"`
			} `json:"dimensions"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Profiles) != 18 {
		t.Fatalf("profiles=%d, want 18", len(catalog.Profiles))
	}
	for _, profile := range catalog.Profiles {
		t.Run(profile.Sheet, func(t *testing.T) {
			snapshot := parsedEvaluationSnapshot{CalculationVersion: canonicalCalculationVersion}
			var tasks []models.EvaluationScoringTask
			var itemID uint
			for dimensionID, sourceDimension := range profile.Dimensions {
				dimension := workflowSnapshotDimension{ID: uint(dimensionID + 1), Name: sourceDimension.Name, Weight: sourceDimension.Weight}
				for _, sourceItem := range sourceDimension.Items {
					itemID++
					item := workflowSnapshotItem{ID: itemID, Name: sourceItem.Name, MaxScore: 100, ItemWeight: sourceItem.ItemWeight, CompositeWeight: CompositeWeight(sourceDimension.Weight, sourceItem.ItemWeight), Required: sourceItem.Required}
					dimension.Items = append(dimension.Items, item)
					snapshot.Items = append(snapshot.Items, item)
					score := float64(100)
					tasks = append(tasks, models.EvaluationScoringTask{ItemID: itemID, Status: "submitted", Score: &score, ScorerUserID: 1})
				}
				snapshot.Dimensions = append(snapshot.Dimensions, dimension)
			}
			result, err := calculateCanonicalResult(snapshot, tasks)
			if strings.Contains(profile.Sheet, "04-软件应用开发-部门负责人") {
				if err != nil {
					t.Fatalf("internal source-weight anomaly must not block calculation when rounded composite total is valid: %v", err)
				}
				if result.TotalScore != 100 || result.Grade != "S" {
					t.Fatalf("result=%+v, want total 100 grade S", result)
				}
				return
			}
			if err != nil {
				t.Fatalf("profile calculation failed: %v", err)
			}
			if result.TotalScore != 100 || result.Grade != "S" {
				t.Fatalf("result=%+v, want total 100 grade S", result)
			}
		})
	}
}

func TestCalculateCanonicalResultUsesWeightedAverageAndZero(t *testing.T) {
	scoreA := float64(80)
	scoreB := float64(100)
	scoreC1 := float64(70)
	scoreC2 := float64(90)
	snapshot := parsedEvaluationSnapshot{
		CalculationVersion: canonicalCalculationVersion,
		Checksum:           "checksum",
		Dimensions: []workflowSnapshotDimension{
			{ID: 1, Name: "交付质量", Weight: 40, Items: []workflowSnapshotItem{
				{ID: 1, Name: "指标A", MaxScore: 100, ItemWeight: 50, CompositeWeight: 20, Required: true},
				{ID: 2, Name: "指标B", MaxScore: 100, ItemWeight: 50, CompositeWeight: 20, Required: true},
			}},
			{ID: 2, Name: "协同", Weight: 60, Items: []workflowSnapshotItem{
				{ID: 3, Name: "指标C", MaxScore: 100, ItemWeight: 100, CompositeWeight: 60, Required: true},
			}},
		},
		Items: []workflowSnapshotItem{
			{ID: 1, Name: "指标A", MaxScore: 100, ItemWeight: 50, CompositeWeight: 20, Required: true},
			{ID: 2, Name: "指标B", MaxScore: 100, ItemWeight: 50, CompositeWeight: 20, Required: true},
			{ID: 3, Name: "指标C", MaxScore: 100, ItemWeight: 100, CompositeWeight: 60, Required: true},
		},
	}
	tasks := []models.EvaluationScoringTask{
		{ID: 1, ItemID: 1, Status: "submitted", Score: &scoreA, ScorerUserID: 1, ScorerType: "PM"},
		{ID: 2, ItemID: 2, Status: "submitted", Score: &scoreB, ScorerUserID: 1, ScorerType: "PM"},
		{ID: 3, ItemID: 3, Status: "submitted", Score: &scoreC1, ScorerUserID: 1, ScorerType: "PM"},
		{ID: 4, ItemID: 3, Status: "submitted", Score: &scoreC2, ScorerUserID: 2, ScorerType: "PRODUCT_MANAGER"},
	}
	result, err := calculateCanonicalResult(snapshot, tasks)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalScore != 84 || result.Grade != "A" {
		t.Fatalf("result=%+v, want total 84 grade A", result)
	}
}

func TestCalculateCanonicalResultAcceptsZeroAndOptionalNotApplicable(t *testing.T) {
	zero := float64(0)
	snapshot := parsedEvaluationSnapshot{Items: []workflowSnapshotItem{
		{ID: 1, Name: "必填零分", MaxScore: 100, CompositeWeight: 100, Required: true},
		{ID: 2, Name: "可选项", MaxScore: 100, CompositeWeight: 20, Required: false},
	}}
	result, err := calculateCanonicalResult(snapshot, []models.EvaluationScoringTask{
		{ID: 1, ItemID: 1, Status: "submitted", Score: &zero, ScorerUserID: 1, ScorerType: "PM"},
		{ID: 2, ItemID: 2, Status: "not_applicable", NotApplicable: true, ScorerUserID: 1, ScorerType: "PM"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalScore != 0 || !result.Items[1].NotApplicable {
		t.Fatalf("result=%+v", result)
	}
}

func TestCalculateCanonicalResultRejectsMissingAndInvalidWeights(t *testing.T) {
	snapshot := parsedEvaluationSnapshot{Items: []workflowSnapshotItem{{ID: 1, Name: "必填", MaxScore: 100, CompositeWeight: 100, Required: true}}}
	_, err := calculateCanonicalResult(snapshot, nil)
	if err == nil || !strings.Contains(err.Error(), "必填") {
		t.Fatalf("missing score err=%v", err)
	}
	invalidWeights := parsedEvaluationSnapshot{Dimensions: []workflowSnapshotDimension{{ID: 1, Name: "错误", Weight: 70, Items: []workflowSnapshotItem{{ID: 1, Name: "指标", ItemWeight: 80, MaxScore: 100}}}}, Items: []workflowSnapshotItem{{ID: 1, Name: "指标", MaxScore: 100, CompositeWeight: 56, Required: false}}}
	if _, err := calculateCanonicalResult(invalidWeights, []models.EvaluationScoringTask{}); err == nil {
		t.Fatal("expected invalid dimension/item weights")
	}
}

func TestCanonicalOriginalHeadProfiles(t *testing.T) {
	for _, fixture := range sourceHeadWeightCases(t) {
		t.Run(fixture.Sheet, func(t *testing.T) {
			snapshot := parsedEvaluationSnapshot{CalculationVersion: canonicalCalculationVersion}
			var tasks []models.EvaluationScoringTask
			var id uint
			for index, original := range fixture.Dimensions {
				dimension := workflowSnapshotDimension{ID: uint(index + 1), Name: original.Name, Weight: original.Weight}
				for _, input := range original.Items {
					id++
					item := workflowSnapshotItem{ID: id, Name: input.Name, MaxScore: 100, ItemWeight: input.ItemWeight, CompositeWeight: CompositeWeight(original.Weight, input.ItemWeight), Required: true}
					dimension.Items = append(dimension.Items, item)
					snapshot.Items = append(snapshot.Items, item)
					for _, source := range fixture.Items {
						if source.Name == input.Name {
							score := source.Score
							tasks = append(tasks, models.EvaluationScoringTask{ID: id, ItemID: id, ScorerUserID: 1, Status: "submitted", Score: &score})
						}
					}
				}
				snapshot.Dimensions = append(snapshot.Dimensions, dimension)
			}
			result, err := calculateCanonicalResult(snapshot, tasks)
			if err != nil {
				t.Fatal(err)
			}
			if result.TotalScore != fixture.ExpectedTotal || result.Grade != "D" {
				t.Fatalf("total=%v want=%v grade=%s", result.TotalScore, fixture.ExpectedTotal, result.Grade)
			}
			for _, actual := range result.Items {
				for _, expected := range fixture.Items {
					if actual.Name == expected.Name && actual.Contribution != expected.Contribution {
						t.Fatalf("%s contribution=%v want=%v", actual.Name, actual.Contribution, expected.Contribution)
					}
				}
			}
		})
	}
}

func TestCanonicalRejectsInconsistentCompositeWithoutMutation(t *testing.T) {
	item := workflowSnapshotItem{ID: 1, Name: "指标", MaxScore: 100, ItemWeight: 100, CompositeWeight: 99}
	snapshot := parsedEvaluationSnapshot{Dimensions: []workflowSnapshotDimension{{ID: 1, Weight: 100, Items: []workflowSnapshotItem{item}}}, Items: []workflowSnapshotItem{item}}
	score := float64(80)
	if _, err := calculateCanonicalResult(snapshot, []models.EvaluationScoringTask{{ItemID: 1, Status: "submitted", Score: &score}}); err == nil {
		t.Fatal("inconsistent stored composite accepted")
	}
	if snapshot.Items[0].CompositeWeight != 99 {
		t.Fatal("snapshot was rewritten")
	}
}
