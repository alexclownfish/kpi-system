package handlers

import (
	"math"
	"net/http"
	"strconv"
	"strings"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
)

const defaultAllocationThreshold = 0.20

// allocationContribution mirrors the HR workbook rule:
// IF(OR(total_score="", allocation_ratio=""), "",
//
//	ROUND(total_score*IF(allocation_ratio>=0.2, allocation_ratio, 0), 1))
//
// The caller keeps a missing ratio distinct from a real zero ratio.
func allocationContribution(totalScore, allocationRatio, threshold float64) (float64, error) {
	if math.IsNaN(totalScore) || math.IsInf(totalScore, 0) || totalScore < 0 || totalScore > 100 {
		return 0, &statisticsInputError{message: "总分必须为0到100之间的有限数值"}
	}
	if math.IsNaN(allocationRatio) || math.IsInf(allocationRatio, 0) || allocationRatio < 0 || allocationRatio > 1 {
		return 0, &statisticsInputError{message: "投入占比必须为0到1之间的有限数值"}
	}
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 1 {
		return 0, &statisticsInputError{message: "投入占比阈值必须为0到1之间的有限数值"}
	}
	if allocationRatio < threshold {
		return 0, nil
	}
	return math.Round(totalScore*allocationRatio*10) / 10, nil
}

type statisticsInputError struct{ message string }

func (e *statisticsInputError) Error() string { return e.message }

type performanceSummaryResponse struct {
	EvaluationID         uint                 `json:"evaluation_id"`
	EmployeeID           uint                 `json:"employee_id"`
	PeriodType           string               `json:"period_type"`
	Period               string               `json:"period"`
	TotalScore           float64              `json:"total_score"`
	Grade                string               `json:"grade"`
	EvaluationCount      int                  `json:"evaluation_count"`
	AllocationThreshold  float64              `json:"allocation_threshold"`
	AllocationRatio      *float64             `json:"allocation_ratio,omitempty"`
	WeightedContribution *float64             `json:"weighted_contribution,omitempty"`
	Projects             []performanceProject `json:"projects,omitempty"`
	SourceSnapshotSHA256 string               `json:"source_snapshot_sha256,omitempty"`
	CalculationVersion   string               `json:"calculation_version"`
	DiagnosticLevel      string               `json:"diagnostic_level"`
	Diagnostics          []string             `json:"diagnostics,omitempty"`
}

type performanceProject struct {
	EvaluationID         uint     `json:"evaluation_id"`
	ProjectName          string   `json:"project_name,omitempty"`
	TotalScore           float64  `json:"total_score"`
	AllocationRatio      *float64 `json:"allocation_ratio,omitempty"`
	WeightedContribution *float64 `json:"weighted_contribution,omitempty"`
	CalculationVersion   string   `json:"calculation_version"`
}

func parseAllocationRatio(c *gin.Context) (*float64, error) {
	raw := strings.TrimSpace(c.Query("allocation_ratio"))
	if raw == "" {
		return nil, nil
	}
	ratio, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, &statisticsInputError{message: "投入占比参数无效"}
	}
	return &ratio, nil
}

func performancePeriodMatches(evaluation models.KPIEvaluation, periodType, period string) bool {
	periodType = strings.ToLower(strings.TrimSpace(periodType))
	period = strings.TrimSpace(period)
	switch periodType {
	case "monthly":
		if evaluation.Period != "monthly" {
			return false
		}
		if period == "" {
			return true
		}
		return period == strconv.Itoa(evaluation.Year)+"-"+twoDigits(pointerIntValue(evaluation.Month))
	case "quarterly":
		if evaluation.Period != "quarterly" {
			return false
		}
		if period == "" {
			return true
		}
		return period == strconv.Itoa(evaluation.Year)+"-Q"+strconv.Itoa(pointerIntValue(evaluation.Quarter))
	case "annual", "yearly":
		targetYear := evaluation.Year
		if period != "" {
			parsed, err := strconv.Atoi(period)
			if err != nil {
				return false
			}
			targetYear = parsed
		}
		return evaluation.Year == targetYear
	default:
		return false
	}
}

func pointerIntValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func twoDigits(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

// GetPerformanceSummary aggregates every completed project assessment in the
// requested period. The legacy query parameter remains supported for old
// clients, but new evaluations persist their own project allocation ratio.
func GetPerformanceSummary(c *gin.Context) {
	employeeID64, err := strconv.ParseUint(c.Query("employee_id"), 10, 32)
	if err != nil || employeeID64 == 0 {
		respondError(c, http.StatusBadRequest, "INVALID_EMPLOYEE_ID", "员工ID参数无效", nil)
		return
	}
	employeeID := uint(employeeID64)
	userID := c.GetUint("user_id")
	if !CanAccessEmployee(userID, employeeID) {
		respondError(c, http.StatusForbidden, "ASSESSMENT_SCOPE_FORBIDDEN", "超出员工数据范围", nil)
		return
	}
	periodType := strings.ToLower(strings.TrimSpace(c.DefaultQuery("period_type", "monthly")))
	if periodType == "annual" {
		periodType = "yearly"
	}
	if periodType != "monthly" && periodType != "quarterly" && periodType != "yearly" {
		respondError(c, http.StatusBadRequest, "INVALID_PERIOD_TYPE", "周期类型必须为 monthly、quarterly 或 annual", nil)
		return
	}
	ratio, ratioErr := parseAllocationRatio(c)
	if ratioErr != nil {
		respondError(c, http.StatusBadRequest, "INVALID_ALLOCATION_RATIO", ratioErr.Error(), nil)
		return
	}
	threshold := defaultAllocationThreshold
	if raw := strings.TrimSpace(c.Query("allocation_threshold")); raw != "" {
		parsed, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil {
			respondError(c, http.StatusBadRequest, "INVALID_ALLOCATION_THRESHOLD", "投入占比阈值参数无效", nil)
			return
		}
		threshold = parsed
	}
	period := strings.TrimSpace(c.Query("period"))
	var evaluations []models.KPIEvaluation
	if dbErr := ApplyEvaluationScope(models.DB.Preload("Template").Where("employee_id = ?", employeeID), userID).
		Where("status = ?", "completed").
		Order("updated_at DESC, id DESC").Find(&evaluations).Error; dbErr != nil {
		respondError(c, http.StatusInternalServerError, "PERFORMANCE_SUMMARY_READ_FAILED", "读取绩效汇总失败", nil)
		return
	}
	selectedEvaluations := make([]models.KPIEvaluation, 0)
	for index := range evaluations {
		if performancePeriodMatches(evaluations[index], periodType, period) {
			selectedEvaluations = append(selectedEvaluations, evaluations[index])
		}
	}
	if len(selectedEvaluations) == 0 {
		respondError(c, http.StatusNotFound, "PERFORMANCE_SUMMARY_NOT_FOUND", "没有找到该员工对应周期的已完成考核", map[string]any{"employee_id": employeeID, "period_type": periodType, "period": period})
		return
	}
	projects := make([]performanceProject, 0, len(selectedEvaluations))
	projectTotals := make([]float64, 0, len(selectedEvaluations))
	effectiveRatios := 0.0
	effectiveContributions := 0.0
	hasRatio := false
	hasMissingRatio := false
	for _, evaluation := range selectedEvaluations {
		total := authoritativeEvaluationTotal(models.DB, evaluation)
		projectTotals = append(projectTotals, total)
		ratio := ratio
		if ratio == nil {
			ratio = evaluation.AllocationRatio
		}
		version := strings.TrimSpace(evaluation.CalculationVersion)
		if version == "" {
			version = "legacy-v1"
		}
		project := performanceProject{EvaluationID: evaluation.ID, ProjectName: evaluation.ProjectName, TotalScore: total, CalculationVersion: version}
		if ratio != nil {
			contribution, contributionErr := allocationContribution(total, *ratio, threshold)
			if contributionErr == nil {
				hasRatio = true
				project.AllocationRatio = ratio
				project.WeightedContribution = &contribution
				if *ratio >= threshold {
					effectiveRatios += *ratio
					effectiveContributions += contribution
				}
			}
		} else {
			hasMissingRatio = true
		}
		projects = append(projects, project)
	}
	total := aggregatePerformanceScore(projectTotals, effectiveRatios, effectiveContributions)
	version := strings.TrimSpace(selectedEvaluations[0].CalculationVersion)
	if version == "" {
		version = "legacy-v1"
	}
	response := performanceSummaryResponse{
		EvaluationID: selectedEvaluations[0].ID, EmployeeID: employeeID, PeriodType: periodType,
		Period:     defaultPerformancePeriod(periodType, period, selectedEvaluations[0]),
		TotalScore: total, Grade: GradeForScore(total), EvaluationCount: len(selectedEvaluations),
		AllocationThreshold: threshold, CalculationVersion: version,
		DiagnosticLevel: "ok", Diagnostics: make([]string, 0), Projects: projects,
	}
	if selectedEvaluations[0].CalculationVersion == canonicalCalculationVersion {
		if snapshot, snapshotErr := parseEvaluationSnapshot(selectedEvaluations[0].TemplateSnapshotJSON); snapshotErr == nil {
			response.SourceSnapshotSHA256 = snapshot.SourceSHA256
		} else {
			response.DiagnosticLevel = "blocking"
			response.Diagnostics = append(response.Diagnostics, "SNAPSHOT_INVALID")
		}
	} else {
		response.DiagnosticLevel = "warning"
		response.Diagnostics = append(response.Diagnostics, "LEGACY_CALCULATION_VERSION")
	}
	if !hasRatio || hasMissingRatio {
		response.Diagnostics = append(response.Diagnostics, "ALLOCATION_RATIO_REQUIRED")
	} else {
		summaryRatio := effectiveRatios
		response.AllocationRatio = &summaryRatio
		summaryContribution := effectiveContributions
		response.WeightedContribution = &summaryContribution
	}
	if len(response.Diagnostics) > 0 && response.DiagnosticLevel == "ok" {
		response.DiagnosticLevel = "warning"
	}
	if c.Query("diagnostics") == "true" {
		RecordAuditDetails(c, "aggregation_diagnosed", "evaluation", strconv.FormatUint(uint64(selectedEvaluations[0].ID), 10), "SUCCESS", strings.Join(response.Diagnostics, ","))
	}
	RecordAuditDetails(c, "aggregation_calculated", "evaluation", strconv.FormatUint(uint64(selectedEvaluations[0].ID), 10), "SUCCESS", "period_type="+periodType+";evaluations="+strconv.Itoa(len(selectedEvaluations)))
	respondData(c, http.StatusOK, response, "绩效汇总读取成功")
}

func aggregatePerformanceScore(totals []float64, effectiveRatios, effectiveContributions float64) float64 {
	if len(totals) == 0 {
		return 0
	}
	if len(totals) == 1 {
		return totals[0]
	}
	if effectiveRatios > 0 {
		return math.Round(effectiveContributions/effectiveRatios*10) / 10
	}
	sum := 0.0
	for _, total := range totals {
		sum += total
	}
	return math.Round(sum/float64(len(totals))*10) / 10
}

func defaultPerformancePeriod(periodType, period string, evaluation models.KPIEvaluation) string {
	if period != "" {
		return period
	}
	switch periodType {
	case "monthly":
		return strconv.Itoa(evaluation.Year) + "-" + twoDigits(pointerIntValue(evaluation.Month))
	case "quarterly":
		return strconv.Itoa(evaluation.Year) + "-Q" + strconv.Itoa(pointerIntValue(evaluation.Quarter))
	default:
		return strconv.Itoa(evaluation.Year)
	}
}
