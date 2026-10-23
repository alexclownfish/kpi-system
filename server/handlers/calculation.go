package handlers

import (
	"errors"
	"fmt"
	"math"
	"math/big"
)

// CalculationItemInput is the immutable calculation input copied from a template.
type CalculationItemInput struct {
	DimensionWeight float64
	ItemWeight      float64
	Score           *float64
	Required        bool
}

// ValidateTemplateWeights preserves workbook inputs; only rounded composite
// weights must total 100, not each dimension's internal item weights.
func ValidateTemplateWeights(dimensionWeights []float64, itemWeightsByDimension [][]float64) error {
	if len(dimensionWeights) == 0 || len(dimensionWeights) != len(itemWeightsByDimension) {
		return errors.New("考核维度与指标权重分组必须完整对应")
	}
	var compositeTotal float64
	for index, weights := range itemWeightsByDimension {
		if !validWeight(dimensionWeights[index]) {
			return errors.New("维度权重必须为0到100之间的有限数值")
		}
		if len(weights) == 0 {
			return errors.New("每个考核维度至少需要一个指标")
		}
		for _, weight := range weights {
			if !validWeight(weight) {
				return errors.New("指标权重必须为0到100之间的有限数值")
			}
			compositeTotal += CompositeWeight(dimensionWeights[index], weight)
		}
	}
	if !approximately100(compositeTotal) {
		return fmt.Errorf("指标综合权重合计必须为100%%，当前为%.1f%%", compositeTotal)
	}
	return nil
}

func validWeight(weight float64) bool {
	return !math.IsNaN(weight) && !math.IsInf(weight, 0) && weight >= 0 && weight <= 100
}

func CompositeWeight(dimensionWeight, itemWeight float64) float64 {
	if !validWeight(dimensionWeight) || !validWeight(itemWeight) {
		return math.NaN()
	}
	product := new(big.Rat).Mul(decimalValue(dimensionWeight), decimalValue(itemWeight))
	// Scaling by 1/1000 lets the shared cents-rounder produce integer tenths.
	product.Quo(product, big.NewRat(1000, 1))
	result, _ := new(big.Rat).SetFrac(decimalHalfUpCents(product), big.NewInt(10)).Float64()
	return result
}

func CalculateWeightedScore(items []CalculationItemInput) (float64, error) {
	var total float64
	for _, item := range items {
		if item.DimensionWeight < 0 || item.ItemWeight < 0 || item.DimensionWeight > 100 || item.ItemWeight > 100 {
			return 0, errors.New("权重必须在0到100之间")
		}
		if item.Required && item.Score == nil {
			return 0, errors.New("仍有必填指标未完成评分")
		}
		if item.Score == nil {
			continue
		}
		if *item.Score < 0 || *item.Score > 100 {
			return 0, errors.New("评分必须在0到100之间")
		}
		total += CompositeWeight(item.DimensionWeight, item.ItemWeight) * *item.Score / 100
	}
	return math.Round(total*100) / 100, nil
}

func GradeForScore(score float64) string {
	switch {
	case score >= 90:
		return "S"
	case score >= 80:
		return "A"
	case score >= 70:
		return "B"
	case score >= 60:
		return "C"
	default:
		return "D"
	}
}

func sum(values []float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}
	return total
}

func approximately100(value float64) bool { return math.Abs(value-100) < 0.0001 }
