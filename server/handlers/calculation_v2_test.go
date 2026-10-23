package handlers

import (
	"math"
	"testing"

	"dootask-kpi-server/models"
)

// T044 keeps the canonical weighted-v2 boundaries explicit. The result must
// use the immutable snapshot, average all submitted scorer values, and round
// each contribution with decimal HALF_UP before assigning a grade.
func TestCalculationV2HalfUpAveragesAndGradeBoundaries(t *testing.T) {
	for _, test := range []struct {
		name  string
		score float64
		grade string
	}{
		{name: "S lower boundary", score: 90, grade: "S"},
		{name: "A lower boundary", score: 80, grade: "A"},
		{name: "B lower boundary", score: 70, grade: "B"},
		{name: "C lower boundary", score: 60, grade: "C"},
		{name: "D below C", score: 59.99, grade: "D"},
	} {
		t.Run(test.name, func(t *testing.T) {
			score := test.score
			result, err := calculateCanonicalResult(parsedEvaluationSnapshot{
				Items: []workflowSnapshotItem{{ID: 1, Name: "指标", MaxScore: 100, CompositeWeight: 100, Required: true}},
			}, []models.EvaluationScoringTask{{ItemID: 1, Status: "submitted", Score: &score}})
			if err != nil {
				t.Fatal(err)
			}
			if result.TotalScore != test.score || result.Grade != test.grade {
				t.Fatalf("score=%v result=%+v", test.score, result)
			}
		})
	}

	// 63 * 7.5% = 4.725, which must round to 4.73 instead of the binary
	// floating-point result 4.72.
	score := 63.0
	result, err := calculateCanonicalResult(parsedEvaluationSnapshot{
		Items: []workflowSnapshotItem{{ID: 1, Name: "半入分", MaxScore: 100, CompositeWeight: 7.5, Required: true}},
	}, []models.EvaluationScoringTask{{ItemID: 1, Status: "submitted", Score: &score}})
	if err != nil || result.TotalScore != 4.73 || result.Items[0].Contribution != 4.73 {
		t.Fatalf("HALF_UP result=%+v err=%v", result, err)
	}

	a, b := 60.0, 66.125
	result, err = calculateCanonicalResult(parsedEvaluationSnapshot{
		Items: []workflowSnapshotItem{{ID: 1, Name: "多评分人", MaxScore: 100, CompositeWeight: 8, Required: true}},
	}, []models.EvaluationScoringTask{
		{ItemID: 1, Status: "submitted", Score: &a, ScorerUserID: 1},
		{ItemID: 1, Status: "submitted", Score: &b, ScorerUserID: 2},
	})
	if err != nil || result.TotalScore != 5.05 || result.Items[0].AggregatedScore == nil || *result.Items[0].AggregatedScore != 63.06 {
		t.Fatalf("multi-scorer result=%+v err=%v", result, err)
	}
}

func TestCalculationV2RejectsDimensionAndScoreAnomalies(t *testing.T) {
	invalid := parsedEvaluationSnapshot{
		Dimensions: []workflowSnapshotDimension{{ID: 1, Name: "异常维度", Weight: 70, Items: []workflowSnapshotItem{{ID: 1, Name: "指标", ItemWeight: 80, CompositeWeight: 56, MaxScore: 100}}}},
		Items:      []workflowSnapshotItem{{ID: 1, Name: "指标", ItemWeight: 80, CompositeWeight: 56, MaxScore: 100}},
	}
	if _, err := calculateCanonicalResult(invalid, nil); err == nil {
		t.Fatal("dimension weights not totaling 100 must block calculation")
	}
	for _, score := range []float64{math.NaN(), math.Inf(1), -1, 101} {
		if _, err := calculateCanonicalResult(parsedEvaluationSnapshot{
			Items: []workflowSnapshotItem{{ID: 1, Name: "异常评分", MaxScore: 100, CompositeWeight: 100, Required: true}},
		}, []models.EvaluationScoringTask{{ItemID: 1, Status: "submitted", Score: &score}}); err == nil {
			t.Fatalf("invalid score %v was accepted", score)
		}
	}
}
