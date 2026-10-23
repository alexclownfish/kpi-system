package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"dootask-kpi-server/models"

	"github.com/gin-gonic/gin"
)

type evaluationWorkSummaryRequest struct {
	WorkSummary   string `json:"work_summary"`
	WorkRisks     string `json:"work_risks"`
	SupportNeeded string `json:"support_needed"`
	NextPlan      string `json:"next_plan"`
}

// SubmitEvaluationWorkSummary records the subject's mandatory summary and
// next-period plan before any self-score can be written.
func SubmitEvaluationWorkSummary(c *gin.Context) {
	evaluationID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || evaluationID == 0 {
		respondError(c, http.StatusBadRequest, "INVALID_EVALUATION_ID", "考核ID无效", nil)
		return
	}
	var request evaluationWorkSummaryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_WORK_SUMMARY", "总结和计划参数无效", nil)
		return
	}
	request.WorkSummary = strings.TrimSpace(request.WorkSummary)
	request.WorkRisks = strings.TrimSpace(request.WorkRisks)
	request.SupportNeeded = strings.TrimSpace(request.SupportNeeded)
	request.NextPlan = strings.TrimSpace(request.NextPlan)
	for label, value := range map[string]string{
		"工作总结": request.WorkSummary, "问题与风险": request.WorkRisks,
		"所需支持": request.SupportNeeded, "后续计划": request.NextPlan,
	} {
		if value == "" {
			respondError(c, http.StatusBadRequest, "WORK_SUMMARY_REQUIRED", "请填写"+label, nil)
			return
		}
		if len([]rune(value)) > 5000 {
			respondError(c, http.StatusBadRequest, "WORK_SUMMARY_TOO_LONG", label+"不能超过5000字", nil)
			return
		}
	}
	var evaluation models.KPIEvaluation
	if err := models.DB.First(&evaluation, uint(evaluationID)).Error; err != nil {
		respondError(c, http.StatusNotFound, "EVALUATION_NOT_FOUND", "考核不存在", nil)
		return
	}
	if evaluation.EmployeeID != c.GetUint("user_id") {
		respondError(c, http.StatusForbidden, "WORK_SUMMARY_FORBIDDEN", "只有被考核员工本人可以提交总结", nil)
		return
	}
	if evaluation.Status != workflowStateSelfEvaluating && evaluation.Status != workflowStateDraft {
		respondError(c, http.StatusConflict, "WORK_SUMMARY_LOCKED", "当前考核已不在总结填写阶段", nil)
		return
	}
	now := time.Now()
	if err := models.DB.Model(&evaluation).Updates(map[string]any{
		"work_summary": request.WorkSummary, "work_risks": request.WorkRisks,
		"support_needed": request.SupportNeeded, "next_plan": request.NextPlan,
		"work_summary_submitted_at": now,
	}).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "WORK_SUMMARY_WRITE_FAILED", "保存工作总结失败", nil)
		return
	}
	RecordAuditDetails(c, "submit_work_summary", "evaluation", strconv.FormatUint(uint64(evaluation.ID), 10), "SUCCESS", "sections=4")
	evaluation.WorkSummary = request.WorkSummary
	evaluation.WorkRisks = request.WorkRisks
	evaluation.SupportNeeded = request.SupportNeeded
	evaluation.NextPlan = request.NextPlan
	evaluation.WorkSummarySubmittedAt = &now
	respondData(c, http.StatusOK, evaluation, "工作总结与后续计划已提交")
}

func workSummaryCompletion(evaluation models.KPIEvaluation) (bool, []any) {
	if !evaluation.WorkSummaryRequired || evaluation.WorkSummarySubmittedAt != nil {
		return true, nil
	}
	return false, []any{map[string]any{"stage": "work_summary", "fields": []string{"work_summary", "work_risks", "support_needed", "next_plan"}}}
}
