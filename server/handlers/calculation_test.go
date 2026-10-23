package handlers

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

type sourceWeightCase struct {
	Sheet      string                   `json:"sheet"`
	Position   string                   `json:"position"`
	Dimensions []templateDimensionInput `json:"dimensions"`
	Items      []struct {
		Name         string  `json:"name"`
		Score        float64 `json:"score"`
		Contribution float64 `json:"contribution"`
	} `json:"items"`
	ExpectedTotal float64 `json:"expected_total"`
}

func sourceHeadWeightCases(t *testing.T) []sourceWeightCase {
	t.Helper()
	raw, err := os.ReadFile("testdata/source-head-weights.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Cases []sourceWeightCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	var cases []sourceWeightCase
	for _, fixture := range manifest.Cases {
		if fixture.Position == "部门负责人" && (fixture.Sheet == "02-结构部门" || fixture.Sheet == "03-电子部门" || fixture.Sheet == "04-软件应用开发") {
			cases = append(cases, fixture)
		}
	}
	if len(cases) != 3 {
		t.Fatalf("source head cases=%d, want 3", len(cases))
	}
	return cases
}

func TestValidateTemplateWeightsPreservesOriginalHeadProfiles(t *testing.T) {
	for _, fixture := range sourceHeadWeightCases(t) {
		t.Run(fixture.Sheet, func(t *testing.T) {
			var dimensions []float64
			var groups [][]float64
			for _, dimension := range fixture.Dimensions {
				dimensions = append(dimensions, dimension.Weight)
				var weights []float64
				for _, item := range dimension.Items {
					weights = append(weights, item.ItemWeight)
				}
				groups = append(groups, weights)
			}
			if err := ValidateTemplateWeights(dimensions, groups); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidateTemplateWeightsCompositePolicyBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		dimensions []float64
		groups     [][]float64
		valid      bool
	}{
		{"composite is authoritative", []float64{40, 80}, [][]float64{{50}, {100}}, true},
		{"bad composite", []float64{50, 50}, [][]float64{{80}, {100}}, false},
		{"empty", nil, nil, false},
		{"missing group", []float64{100}, nil, false},
		{"extra group", []float64{100}, [][]float64{{100}, {0}}, false},
		{"empty dimension", []float64{100, 0}, [][]float64{{100}, {}}, false},
		{"negative dimension", []float64{-1, 100}, [][]float64{{0}, {100}}, false},
		{"large dimension", []float64{101}, [][]float64{{100}}, false},
		{"nan dimension", []float64{math.NaN()}, [][]float64{{100}}, false},
		{"infinite dimension", []float64{math.Inf(1)}, [][]float64{{100}}, false},
		{"negative item", []float64{100}, [][]float64{{-1, 100}}, false},
		{"large item", []float64{100}, [][]float64{{101}}, false},
		{"nan item", []float64{100}, [][]float64{{math.NaN()}}, false},
		{"infinite item", []float64{100}, [][]float64{{math.Inf(-1)}}, false},
		{"zero weight valid", []float64{100, 0}, [][]float64{{100}, {0}}, true},
		{"rounded sum not raw sum", []float64{100}, [][]float64{{33.33, 33.33, 33.34}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplateWeights(tt.dimensions, tt.groups)
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v err=%v", tt.valid, err)
			}
		})
	}
}

func TestCompositeWeightDecimalHalfUp(t *testing.T) {
	for _, tt := range []struct{ dimension, item, want float64 }{{15, 73, 11}, {16.2, 25, 4.1}, {10, 0.5, 0.1}, {100, 33.34, 33.3}} {
		if got := CompositeWeight(tt.dimension, tt.item); got != tt.want {
			t.Errorf("%v * %v composite=%v want=%v", tt.dimension, tt.item, got, tt.want)
		}
	}
}

func TestValidateTemplateWeights(t *testing.T) {
	if err := ValidateTemplateWeights([]float64{20, 40, 15, 5, 20}, [][]float64{{100}, {50, 50}, {100}, {100}, {100}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTemplateWeights([]float64{20, 40}, [][]float64{{100}, {80}}); err == nil {
		t.Fatal("expected invalid item weights")
	}
}

func TestCalculateWeightedScoreAndGrade(t *testing.T) {
	score := 90.0
	items := []CalculationItemInput{{DimensionWeight: 15, ItemWeight: 60, Score: &score, Required: true}}
	got, err := CalculateWeightedScore(items)
	if err != nil || got != 8.1 {
		t.Fatalf("score=%v err=%v, want 8.1", got, err)
	}
	if GradeForScore(90) != "S" || GradeForScore(80) != "A" || GradeForScore(70) != "B" || GradeForScore(60) != "C" || GradeForScore(59.99) != "D" {
		t.Fatal("grade boundaries do not match the workbook")
	}
}

func TestCalculateWeightedScoreRejectsMissingRequiredScore(t *testing.T) {
	_, err := CalculateWeightedScore([]CalculationItemInput{{DimensionWeight: 20, ItemWeight: 100, Required: true}})
	if err == nil {
		t.Fatal("expected missing required score error")
	}
}
