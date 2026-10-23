package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type canonicalScorerValue struct {
	TaskID       uint    `json:"task_id"`
	ScorerUserID uint    `json:"scorer_user_id"`
	ScorerType   string  `json:"scorer_type"`
	Score        float64 `json:"score"`
}

type canonicalItemResult struct {
	ItemID          uint                   `json:"item_id"`
	Name            string                 `json:"name"`
	MaxScore        float64                `json:"max_score"`
	Required        bool                   `json:"required"`
	CompositeWeight float64                `json:"composite_weight"`
	ScorerValues    []canonicalScorerValue `json:"scorer_values"`
	AggregatedScore *float64               `json:"aggregated_score,omitempty"`
	Contribution    float64                `json:"contribution"`
	NotApplicable   bool                   `json:"not_applicable"`
}

type canonicalEvaluationResult struct {
	EvaluationID       uint                      `json:"evaluation_id"`
	CalculationVersion string                    `json:"calculation_version"`
	SnapshotChecksum   string                    `json:"snapshot_checksum"`
	ResultVersion      int                       `json:"result_version"`
	Items              []canonicalItemResult     `json:"items"`
	TotalScore         float64                   `json:"total_score"`
	Grade              string                    `json:"grade"`
	ExportLayout       models.ResultExportLayout `json:"export_layout"`
	CalculatedAt       time.Time                 `json:"calculated_at"`
}

var errCanonicalRequiredScoreMissing = errors.New("仍有必填指标未完成评分")

func calculateCanonicalResult(snapshot parsedEvaluationSnapshot, tasks []models.EvaluationScoringTask) (canonicalEvaluationResult, error) {
	if err := validateCanonicalSnapshotWeights(snapshot); err != nil {
		return canonicalEvaluationResult{}, err
	}
	tasksByItem := make(map[uint][]models.EvaluationScoringTask)
	for _, task := range tasks {
		tasksByItem[task.ItemID] = append(tasksByItem[task.ItemID], task)
	}
	result := canonicalEvaluationResult{CalculationVersion: canonicalCalculationVersion, SnapshotChecksum: snapshot.Checksum, Items: make([]canonicalItemResult, 0, len(snapshot.Items)), ExportLayout: snapshot.ExportLayout}
	totalCents := new(big.Int)
	for _, item := range snapshot.Items {
		if math.IsNaN(item.MaxScore) || math.IsInf(item.MaxScore, 0) || item.MaxScore <= 0 || math.IsNaN(item.CompositeWeight) || math.IsInf(item.CompositeWeight, 0) || item.CompositeWeight < 0 || item.CompositeWeight > 100 {
			return result, fmt.Errorf("指标“%s”满分或综合权重无效", item.Name)
		}
		itemResult := canonicalItemResult{ItemID: item.ID, Name: item.Name, MaxScore: item.MaxScore, Required: item.Required, CompositeWeight: item.CompositeWeight, ScorerValues: make([]canonicalScorerValue, 0)}
		var scores []float64
		for _, task := range tasksByItem[item.ID] {
			if item.Required && task.Required && task.Status != "submitted" {
				return result, fmt.Errorf("%w：%s", errCanonicalRequiredScoreMissing, item.Name)
			}
			if task.Status == "not_applicable" || task.NotApplicable {
				if item.Required && task.Required {
					return result, fmt.Errorf("必填指标“%s”不允许标记为不适用", item.Name)
				}
				itemResult.NotApplicable = true
				continue
			}
			if task.Status != "submitted" || task.Score == nil {
				continue
			}
			if item.MaxScore <= 0 || math.IsNaN(*task.Score) || math.IsInf(*task.Score, 0) || *task.Score < 0 || *task.Score > item.MaxScore {
				return result, fmt.Errorf("指标“%s”评分超出范围", item.Name)
			}
			scores = append(scores, *task.Score)
			itemResult.ScorerValues = append(itemResult.ScorerValues, canonicalScorerValue{TaskID: task.ID, ScorerUserID: task.ScorerUserID, ScorerType: task.ScorerType, Score: *task.Score})
		}
		if len(scores) == 0 {
			if item.Required && !itemResult.NotApplicable {
				return result, fmt.Errorf("%w：%s", errCanonicalRequiredScoreMissing, item.Name)
			}
			result.Items = append(result.Items, itemResult)
			continue
		}
		averageDecimal := new(big.Rat)
		for _, score := range scores {
			averageDecimal.Add(averageDecimal, decimalValue(score))
		}
		averageDecimal.Quo(averageDecimal, new(big.Rat).SetInt64(int64(len(scores))))
		average := roundedDecimalFloat(averageDecimal)
		itemResult.AggregatedScore = &average
		// Excel weights the unrounded scorer average, then ROUNDs each item to
		// two decimal places. Decimal rationals avoid binary tie drift (4.725).
		contribution := new(big.Rat).Quo(averageDecimal, decimalValue(item.MaxScore))
		contribution.Mul(contribution, decimalValue(item.CompositeWeight))
		cents := decimalHalfUpCents(contribution)
		itemResult.Contribution, _ = new(big.Rat).SetFrac(cents, big.NewInt(100)).Float64()
		totalCents.Add(totalCents, cents)
		result.Items = append(result.Items, itemResult)
	}
	result.TotalScore, _ = new(big.Rat).SetFrac(totalCents, big.NewInt(100)).Float64()
	result.Grade = GradeForScore(result.TotalScore)
	return result, nil
}

func decimalValue(value float64) *big.Rat {
	result, _ := new(big.Rat).SetString(strconv.FormatFloat(value, 'f', -1, 64))
	return result
}

func decimalHalfUpCents(value *big.Rat) *big.Int {
	scaled := new(big.Rat).Mul(value, big.NewRat(100, 1))
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(scaled.Num(), scaled.Denom(), remainder)
	if new(big.Int).Mul(remainder, big.NewInt(2)).Cmp(scaled.Denom()) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}

func roundedDecimalFloat(value *big.Rat) float64 {
	result, _ := new(big.Rat).SetFrac(decimalHalfUpCents(value), big.NewInt(100)).Float64()
	return result
}

func validateCanonicalSnapshotWeights(snapshot parsedEvaluationSnapshot) error {
	if len(snapshot.Dimensions) == 0 {
		return nil
	}
	dimensionWeights := make([]float64, 0, len(snapshot.Dimensions))
	itemWeights := make([][]float64, 0, len(snapshot.Dimensions))
	expectedComposite := make(map[uint]float64)
	for _, dimension := range snapshot.Dimensions {
		dimensionWeights = append(dimensionWeights, dimension.Weight)
		weights := make([]float64, 0, len(dimension.Items))
		for _, item := range dimension.Items {
			weights = append(weights, item.ItemWeight)
			expected := CompositeWeight(dimension.Weight, item.ItemWeight)
			if _, duplicate := expectedComposite[item.ID]; duplicate {
				return fmt.Errorf("模板快照存在重复指标：%d", item.ID)
			}
			if !validWeight(item.CompositeWeight) || math.IsNaN(expected) || math.Abs(item.CompositeWeight-expected) >= 0.0001 {
				return fmt.Errorf("模板快照指标“%s”的综合权重与原始权重不一致", item.Name)
			}
			expectedComposite[item.ID] = expected
		}
		itemWeights = append(itemWeights, weights)
	}
	if err := ValidateTemplateWeights(dimensionWeights, itemWeights); err != nil {
		return fmt.Errorf("模板快照权重无效：%w", err)
	}
	if len(snapshot.Items) != len(expectedComposite) {
		return fmt.Errorf("模板快照指标与维度明细不一致")
	}
	for _, item := range snapshot.Items {
		expected, exists := expectedComposite[item.ID]
		if !exists || !validWeight(item.CompositeWeight) || math.Abs(item.CompositeWeight-expected) >= 0.0001 {
			return fmt.Errorf("模板快照指标“%s”的综合权重与维度明细不一致", item.Name)
		}
		delete(expectedComposite, item.ID)
	}
	return nil
}

func persistCanonicalEvaluationResult(tx *gorm.DB, evaluationID, calculatedBy uint) (models.EvaluationCalculationResult, canonicalEvaluationResult, error) {
	var evaluation models.KPIEvaluation
	if err := tx.First(&evaluation, evaluationID).Error; err != nil {
		return models.EvaluationCalculationResult{}, canonicalEvaluationResult{}, err
	}
	snapshot, err := parseWorkflowSnapshotForEvaluation(evaluation)
	if err != nil {
		return models.EvaluationCalculationResult{}, canonicalEvaluationResult{}, err
	}
	var tasks []models.EvaluationScoringTask
	if err := tx.Where("evaluation_id = ?", evaluationID).Order("item_id ASC, id ASC").Find(&tasks).Error; err != nil {
		return models.EvaluationCalculationResult{}, canonicalEvaluationResult{}, err
	}
	calculated, err := calculateCanonicalResult(snapshot, tasks)
	if err != nil {
		return models.EvaluationCalculationResult{}, calculated, err
	}
	var latestVersion int
	if err := tx.Model(&models.EvaluationCalculationResult{}).Where("evaluation_id = ?", evaluationID).Select("COALESCE(MAX(version), 0)").Scan(&latestVersion).Error; err != nil {
		return models.EvaluationCalculationResult{}, calculated, err
	}
	calculated.EvaluationID = evaluationID
	calculated.ResultVersion = latestVersion + 1
	calculated.CalculatedAt = time.Now()
	raw, err := json.Marshal(calculated)
	if err != nil {
		return models.EvaluationCalculationResult{}, calculated, err
	}
	result := models.EvaluationCalculationResult{EvaluationID: evaluationID, Version: calculated.ResultVersion, CalculationVersion: canonicalCalculationVersion, SnapshotChecksum: snapshot.Checksum, ResultJSON: string(raw), TotalScore: calculated.TotalScore, Grade: calculated.Grade, CalculatedAt: calculated.CalculatedAt}
	if calculatedBy != 0 {
		result.CalculatedBy = &calculatedBy
	}
	if err := tx.Create(&result).Error; err != nil {
		return models.EvaluationCalculationResult{}, calculated, err
	}
	if err := tx.Model(&models.KPIEvaluation{}).Where("id = ?", evaluationID).Updates(map[string]any{"total_score": calculated.TotalScore, "calculation_version": canonicalCalculationVersion}).Error; err != nil {
		return models.EvaluationCalculationResult{}, calculated, err
	}
	return result, calculated, nil
}

func GetEvaluationCanonicalResult(c *gin.Context) {
	evaluationID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || evaluationID == 0 {
		respondError(c, http.StatusBadRequest, "INVALID_EVALUATION_ID", "考核ID无效", nil)
		return
	}
	userID := c.GetUint("user_id")
	if !canViewWorkflowEvaluation(models.DB, userID, uint(evaluationID)) {
		respondError(c, http.StatusForbidden, "ASSESSMENT_SCOPE_FORBIDDEN", "无权访问该考核结果", nil)
		return
	}
	var stored models.EvaluationCalculationResult
	if err := models.DB.Where("evaluation_id = ?", evaluationID).Order("version DESC").First(&stored).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(c, http.StatusNotFound, "RESULT_NOT_AVAILABLE", "当前考核还没有可用的最终计算结果", nil)
			return
		}
		respondError(c, http.StatusInternalServerError, "RESULT_READ_FAILED", "读取最终计算结果失败", nil)
		return
	}
	var result canonicalEvaluationResult
	if err := json.Unmarshal([]byte(stored.ResultJSON), &result); err != nil {
		respondError(c, http.StatusConflict, "RESULT_INVALID", "最终计算结果格式无效", nil)
		return
	}
	respondData(c, http.StatusOK, result, "最终计算结果读取成功")
}
