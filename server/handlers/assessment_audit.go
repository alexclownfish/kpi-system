package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type assessmentAuditPreviewRequest struct {
	EvaluationIDs    []uint   `json:"evaluation_ids"`
	DepartmentID     *uint    `json:"department_id,omitempty"`
	Period           string   `json:"period,omitempty"`
	Statuses         []string `json:"statuses,omitempty"`
	IncludeCompleted bool     `json:"include_completed"`
}

type assessmentAuditFindingResponse struct {
	EvaluationID         *uint  `json:"evaluation_id,omitempty"`
	Category             string `json:"category"`
	Severity             string `json:"severity"`
	Code                 string `json:"code"`
	Message              string `json:"message"`
	Details              any    `json:"details,omitempty"`
	EligibleForMigration bool   `json:"eligible_for_migration"`
}

type assessmentAuditSummary struct {
	EvaluationsScanned int `json:"evaluations_scanned"`
	Eligible           int `json:"eligible"`
	ManualReview       int `json:"manual_review"`
	Errors             int `json:"errors"`
}

type assessmentAuditPreviewResponse struct {
	RunID    string                           `json:"run_id"`
	Summary  assessmentAuditSummary           `json:"summary"`
	Findings []assessmentAuditFindingResponse `json:"findings"`
}

type cleanupOrphanScoringTasksRequest struct {
	TaskIDs []uint `json:"task_ids"`
	Confirm string `json:"confirm"`
}

func CleanupOrphanScoringTasks(c *gin.Context) {
	var request cleanupOrphanScoringTasksRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.TaskIDs) == 0 || len(request.TaskIDs) > 200 {
		respondError(c, http.StatusBadRequest, "INVALID_ORPHAN_CLEANUP", "请提供1到200个孤立任务ID", nil)
		return
	}
	if request.Confirm != "DELETE_ORPHAN_SCORING_TASKS" {
		respondError(c, http.StatusBadRequest, "ORPHAN_CLEANUP_CONFIRMATION_REQUIRED", "请确认孤立评分任务清理", nil)
		return
	}
	var tasks []models.EvaluationScoringTask
	if err := models.DB.Where("id IN ? AND evaluation_id NOT IN (SELECT id FROM kpi_evaluations)", request.TaskIDs).Find(&tasks).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "ORPHAN_CLEANUP_READ_FAILED", "读取孤立评分任务失败", nil)
		return
	}
	if len(tasks) == 0 {
		respondData(c, http.StatusOK, gin.H{"deleted": 0, "task_ids": []uint{}}, "没有需要清理的孤立评分任务")
		return
	}
	ids := make([]uint, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	if err := models.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id IN ? AND evaluation_id NOT IN (SELECT id FROM kpi_evaluations)", ids).Delete(&models.EvaluationScoringTask{}).Error; err != nil {
			return err
		}
		return tx.Create(&models.AuditLog{
			UserID: c.GetUint("user_id"), Action: "cleanup_orphan_scoring_tasks", Resource: "evaluation_scoring_task",
			Result: "SUCCESS", Details: fmt.Sprintf("count=%d;ids=%v", len(ids), ids), IPAddress: c.ClientIP(),
		}).Error
	}); err != nil {
		respondError(c, http.StatusInternalServerError, "ORPHAN_CLEANUP_FAILED", "清理孤立评分任务失败", nil)
		return
	}
	respondData(c, http.StatusOK, gin.H{"deleted": len(ids), "task_ids": ids}, "孤立评分任务已清理")
}

func PreviewAssessmentAudit(c *gin.Context) {
	if !models.EvaluationWorkflowAuditEnabled() {
		respondError(c, http.StatusNotFound, "WORKFLOW_AUDIT_DISABLED", "当前部署未启用考核流程审计", nil)
		return
	}
	var request assessmentAuditPreviewRequest
	if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		respondError(c, http.StatusBadRequest, "INVALID_AUDIT_FILTER", "审计筛选条件无效", map[string]any{"reason": err.Error()})
		return
	}
	if len(request.EvaluationIDs) > 500 {
		respondError(c, http.StatusBadRequest, "INVALID_AUDIT_FILTER", "单次最多审计500条考核", nil)
		return
	}

	userID := c.GetUint("user_id")
	query := ApplyEvaluationScope(models.DB.Model(&models.KPIEvaluation{}), userID)
	if request.DepartmentID != nil {
		query = query.Where("kpi_evaluations.employee_id IN (SELECT id FROM employees WHERE department_id = ?)", *request.DepartmentID)
	}
	if strings.TrimSpace(request.Period) != "" {
		query = query.Where("kpi_evaluations.period = ?", strings.TrimSpace(request.Period))
	}
	if len(request.EvaluationIDs) > 0 {
		query = query.Where("kpi_evaluations.id IN ?", request.EvaluationIDs)
	}
	if len(request.Statuses) > 0 {
		query = query.Where("kpi_evaluations.status IN ?", request.Statuses)
	} else if !request.IncludeCompleted {
		query = query.Where("kpi_evaluations.status <> ?", "completed")
	}

	var evaluations []models.KPIEvaluation
	if err := query.Order("kpi_evaluations.id ASC").Find(&evaluations).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "AUDIT_FAILED", "读取待审计考核失败", nil)
		return
	}

	response := assessmentAuditPreviewResponse{RunID: requestID(c), Findings: make([]assessmentAuditFindingResponse, 0)}
	response.Summary.EvaluationsScanned = len(evaluations)
	for index := range evaluations {
		findings := auditEvaluation(models.DB, evaluations[index])
		response.Findings = append(response.Findings, findings...)
		eligible := true
		for _, finding := range findings {
			if !finding.EligibleForMigration || finding.Severity == "error" || finding.Severity == "manual_review" {
				eligible = false
				break
			}
		}
		if eligible {
			response.Summary.Eligible++
		} else {
			response.Summary.ManualReview++
		}
		for _, finding := range findings {
			if finding.Severity == "error" {
				response.Summary.Errors++
			}
		}
	}
	// Workflow children can survive a partially applied legacy delete or a
	// manual database edit. Only an ALL/system administrator may receive the
	// global orphan scan: a scoped auditor must not learn counts for records
	// that cannot be associated with an assessment in their data scope.
	for _, finding := range auditOrphanWorkflowRecords(models.DB, userID) {
		response.Findings = append(response.Findings, finding)
		response.Summary.ManualReview++
		if finding.Severity == "error" {
			response.Summary.Errors++
		}
	}
	sort.SliceStable(response.Findings, func(i, j int) bool {
		left, right := response.Findings[i], response.Findings[j]
		if left.EvaluationID == nil && right.EvaluationID != nil {
			return true
		}
		if left.EvaluationID != nil && right.EvaluationID == nil {
			return false
		}
		if left.EvaluationID != nil && right.EvaluationID != nil && *left.EvaluationID != *right.EvaluationID {
			return *left.EvaluationID < *right.EvaluationID
		}
		if left.Category != right.Category {
			return left.Category < right.Category
		}
		return left.Code < right.Code
	})
	respondData(c, http.StatusOK, response, "审计完成")
}

func auditEvaluation(db *gorm.DB, evaluation models.KPIEvaluation) []assessmentAuditFindingResponse {
	evaluationID := evaluation.ID
	findings := make([]assessmentAuditFindingResponse, 0)
	add := func(category, severity, code, message string, details any, eligible bool) {
		findings = append(findings, assessmentAuditFindingResponse{
			EvaluationID: &evaluationID, Category: category, Severity: severity, Code: code,
			Message: message, Details: details, EligibleForMigration: eligible,
		})
	}

	// Period metadata is part of the assessment itself and must be checked even
	// when a legacy snapshot is missing or malformed. Otherwise an invalid
	// record can be incorrectly classified as merely "missing snapshot".
	if err := validateEvaluationPeriod(evaluation); err != nil {
		add("period", "manual_review", "PERIOD_INVALID", "考核周期字段无效", map[string]any{"reason": err.Error(), "period": evaluation.Period, "year": evaluation.Year, "month": evaluation.Month, "quarter": evaluation.Quarter}, false)
	}
	validStatuses := map[string]bool{
		"draft": true, "pending": true, "self_evaluating": true, "self_evaluated": true, "scoring": true,
		"manager_evaluated": true, "awaiting_review": true, "pending_confirm": true,
		"awaiting_employee_confirmation": true, "completed": true, "rejected": true, "cancelled": true,
	}
	if !validStatuses[evaluation.Status] {
		add("status", "manual_review", "STATUS_INVALID", "考核状态不在受支持的状态集合中", map[string]any{"status": evaluation.Status}, false)
	}

	if strings.TrimSpace(evaluation.TemplateSnapshotJSON) == "" {
		add("snapshot", "manual_review", "SNAPSHOT_MISSING", "考核缺少模板快照，不能自动迁移", nil, false)
		return findings
	}
	snapshot, err := parseEvaluationSnapshot(evaluation.TemplateSnapshotJSON)
	if err != nil {
		add("snapshot", "manual_review", "SNAPSHOT_INVALID", "考核模板快照无法解析", map[string]any{"reason": err.Error()}, false)
		return findings
	}
	if snapshot.Legacy {
		add("snapshot", "warning", "LEGACY_SNAPSHOT", "考核使用旧版模板快照，迁移前需要生成新评分任务", nil, true)
	}

	if isNewWorkflowEvaluation(evaluation) {
		var latestResult models.EvaluationCalculationResult
		resultErr := db.Where("evaluation_id = ?", evaluation.ID).Order("version DESC").First(&latestResult).Error
		if resultErr != nil {
			if errors.Is(resultErr, gorm.ErrRecordNotFound) && (evaluation.Status == workflowStateAwaitingEmployeeConfirmation || evaluation.Status == workflowStateCompleted) {
				add("result", "manual_review", "CANONICAL_RESULT_MISSING", "考核已进入最终确认阶段但缺少最终计算结果", nil, false)
			} else if !errors.Is(resultErr, gorm.ErrRecordNotFound) {
				add("result", "error", "RESULT_READ_FAILED", "读取最终计算结果失败", map[string]any{"reason": resultErr.Error()}, false)
			}
		} else {
			if latestResult.SnapshotChecksum != "" && latestResult.SnapshotChecksum != snapshot.Checksum {
				add("result", "manual_review", "RESULT_SNAPSHOT_MISMATCH", "最终计算结果引用的模板快照与考核快照不一致", map[string]any{"result_checksum": latestResult.SnapshotChecksum, "snapshot_checksum": snapshot.Checksum}, false)
			}
			if (evaluation.Status == workflowStateAwaitingEmployeeConfirmation || evaluation.Status == workflowStateCompleted) && absFloat(evaluation.TotalScore-latestResult.TotalScore) > 0.0001 {
				add("result", "manual_review", "TOTAL_SCORE_MISMATCH", "考核总分与最终计算结果不一致", map[string]any{"evaluation_total": evaluation.TotalScore, "result_total": latestResult.TotalScore}, false)
			}
		}
		if evaluation.Status == workflowStateCompleted {
			var confirmations int64
			if err := db.Model(&models.EvaluationConfirmation{}).Where("evaluation_id = ?", evaluation.ID).Count(&confirmations).Error; err != nil {
				add("confirmation", "error", "CONFIRMATION_READ_FAILED", "读取最终确认记录失败", map[string]any{"reason": err.Error()}, false)
			} else if confirmations == 0 {
				add("confirmation", "manual_review", "CONFIRMATION_MISSING", "考核已完成但缺少最终确认记录", nil, false)
			}
		}
	}

	var scoreItemIDs []uint
	if err := db.Model(&models.KPIScore{}).Where("evaluation_id = ?", evaluation.ID).Distinct("item_id").Pluck("item_id", &scoreItemIDs).Error; err != nil {
		add("score", "error", "SCORE_READ_FAILED", "读取旧评分记录失败", map[string]any{"reason": err.Error()}, false)
		return findings
	}
	var legacyScores []models.KPIScore
	if err := db.Where("evaluation_id = ?", evaluation.ID).Find(&legacyScores).Error; err != nil {
		add("score", "error", "SCORE_DETAIL_READ_FAILED", "读取旧评分明细失败", map[string]any{"reason": err.Error()}, false)
		return findings
	}
	if evaluation.Status == workflowStateCompleted && !isNewWorkflowEvaluation(evaluation) {
		if len(legacyScores) == 0 {
			add("result", "manual_review", "LEGACY_FINAL_SCORES_MISSING", "旧流程考核已完成但没有最终评分明细", nil, false)
		} else {
			expectedTotal := 0.0
			for _, score := range legacyScores {
				expectedTotal += effectiveFinalScore(score)
			}
			if absFloat(evaluation.TotalScore-expectedTotal) > 0.0001 {
				add("result", "manual_review", "TOTAL_SCORE_MISMATCH", "考核总分与旧流程最终评分明细不一致", map[string]any{"evaluation_total": evaluation.TotalScore, "score_total": expectedTotal, "source": "legacy_final_scores"}, false)
			}
		}
	}
	if len(scoreItemIDs) > 0 && len(scoreItemIDs) != len(snapshot.Items) {
		add("score", "manual_review", "ITEM_COUNT_MISMATCH", "模板快照指标数量与旧评分记录不一致", map[string]any{"snapshot_items": len(snapshot.Items), "score_items": len(scoreItemIDs)}, false)
	}
	if len(scoreItemIDs) == 0 && evaluation.Status != "pending" && evaluation.Status != "draft" {
		add("score", "manual_review", "LEGACY_SCORES_MISSING", "考核已经进入评分流程但没有旧评分记录", nil, false)
	}
	var taskCount int64
	if err := db.Model(&models.EvaluationScoringTask{}).Where("evaluation_id = ?", evaluation.ID).Count(&taskCount).Error; err != nil {
		add("task", "error", "TASK_READ_FAILED", "读取评分任务失败", map[string]any{"reason": err.Error()}, false)
	} else if !snapshot.Legacy && taskCount == 0 && evaluation.Status != "pending" && evaluation.Status != workflowStateDraft {
		add("task", "manual_review", "TASKS_MISSING", "新版快照缺少评分任务", nil, false)
	}
	var duplicateItemIDs []uint
	if err := db.Model(&models.KPIScore{}).Select("item_id").Where("evaluation_id = ?", evaluation.ID).Group("item_id").Having("COUNT(*) > 1").Pluck("item_id", &duplicateItemIDs).Error; err != nil {
		add("duplicate", "error", "DUPLICATE_SCORE_READ_FAILED", "读取重复旧评分记录失败", map[string]any{"reason": err.Error()}, false)
	} else if len(duplicateItemIDs) > 0 {
		add("duplicate", "manual_review", "DUPLICATE_LEGACY_SCORES", "同一考核指标存在重复旧评分记录", map[string]any{"item_ids": duplicateItemIDs}, false)
	}
	var duplicateTaskItems []uint
	if err := db.Model(&models.EvaluationScoringTask{}).Select("item_id").Where("evaluation_id = ?", evaluation.ID).Group("item_id, scorer_user_id").Having("COUNT(*) > 1").Pluck("item_id", &duplicateTaskItems).Error; err != nil {
		add("duplicate", "error", "DUPLICATE_TASK_READ_FAILED", "读取重复评分任务失败", map[string]any{"reason": err.Error()}, false)
	} else if len(duplicateTaskItems) > 0 {
		add("duplicate", "manual_review", "DUPLICATE_SCORING_TASKS", "同一考核指标与评分人存在重复评分任务", map[string]any{"item_ids": duplicateTaskItems}, false)
	}
	if !snapshot.Legacy {
		for _, item := range snapshot.Items {
			if item.Required && len(item.ScorerTypes) == 0 {
				add("snapshot", "manual_review", "SCORER_RULE_MISSING", "必填指标没有评分职责规则", map[string]any{"item_id": item.ID}, false)
			}
		}
	}

	if len(findings) == 0 || onlyWarnings(findings) {
		add("assessment", "info", "AUDIT_ELIGIBLE", "考核记录通过基础一致性检查，可进入迁移演练", map[string]any{"snapshot_checksum": snapshot.Checksum}, true)
	}
	return findings
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func auditOrphanWorkflowRecords(db *gorm.DB, userID uint) []assessmentAuditFindingResponse {
	findings := make([]assessmentAuditFindingResponse, 0)
	if !canAuditGlobalRecords(userID) {
		return findings
	}
	addReadFailure := func(code, message string, err error) {
		findings = append(findings, assessmentAuditFindingResponse{
			Category: "orphan", Severity: "error", Code: code, Message: message,
			Details: map[string]any{"reason": err.Error()}, EligibleForMigration: false,
		})
	}
	addOrphans := func(model any, query, code, message, readCode, readMessage string) {
		var count int64
		if err := db.Model(model).Where(query).Count(&count).Error; err != nil {
			addReadFailure(readCode, readMessage, err)
			return
		}
		if count == 0 {
			return
		}
		// Keep the audit read-only while making the finding actionable. A
		// count alone cannot identify which stale rows need a controlled
		// backup/cleanup. Limit the sample to avoid returning an unbounded
		// payload when a damaged database contains many rows.
		var sampleIDs []uint
		if err := db.Model(model).Where(query).Order("id ASC").Limit(20).Pluck("id", &sampleIDs).Error; err != nil {
			addReadFailure(readCode, readMessage, err)
			return
		}
		findings = append(findings, assessmentAuditFindingResponse{
			Category: "orphan", Severity: "manual_review", Code: code, Message: message,
			Details: map[string]any{"count": count, "sample_ids": sampleIDs}, EligibleForMigration: false,
		})
	}
	addOrphans(&models.EvaluationScoringTask{}, "evaluation_id NOT IN (SELECT id FROM kpi_evaluations)", "ORPHAN_SCORING_TASKS", "存在找不到所属考核的评分任务", "ORPHAN_SCORING_TASKS_READ_FAILED", "读取孤立评分任务失败")
	addOrphans(&models.EvaluationCalculationResult{}, "evaluation_id NOT IN (SELECT id FROM kpi_evaluations)", "ORPHAN_CALCULATION_RESULTS", "存在找不到所属考核的最终计算结果", "ORPHAN_CALCULATION_RESULTS_READ_FAILED", "读取孤立最终计算结果失败")
	addOrphans(&models.EvaluationResultSnapshot{}, "evaluation_id NOT IN (SELECT id FROM kpi_evaluations)", "ORPHAN_RESULT_SNAPSHOTS", "存在找不到所属考核的结果快照", "ORPHAN_RESULT_SNAPSHOTS_READ_FAILED", "读取孤立结果快照失败")
	addOrphans(&models.EvaluationConfirmation{}, "evaluation_id NOT IN (SELECT id FROM kpi_evaluations)", "ORPHAN_CONFIRMATIONS", "存在找不到所属考核的确认记录", "ORPHAN_CONFIRMATIONS_READ_FAILED", "读取孤立确认记录失败")
	addOrphans(&models.EvaluationInvitation{}, "evaluation_id NOT IN (SELECT id FROM kpi_evaluations)", "ORPHAN_INVITATIONS", "存在找不到所属考核的评分邀请", "ORPHAN_INVITATIONS_READ_FAILED", "读取孤立评分邀请失败")
	addOrphans(&models.EvaluationComment{}, "evaluation_id NOT IN (SELECT id FROM kpi_evaluations)", "ORPHAN_COMMENTS", "存在找不到所属考核的评论", "ORPHAN_COMMENTS_READ_FAILED", "读取孤立评论失败")
	addOrphans(&models.InvitedScore{}, "invitation_id NOT IN (SELECT id FROM evaluation_invitations)", "ORPHAN_INVITED_SCORES", "存在找不到所属邀请的邀请评分", "ORPHAN_INVITED_SCORES_READ_FAILED", "读取孤立邀请评分失败")
	return findings
}

func canAuditGlobalRecords(userID uint) bool {
	return DataScopeForUser(userID) == "ALL" || UserHasPermission(userID, "system:edit")
}

func onlyWarnings(findings []assessmentAuditFindingResponse) bool {
	for _, finding := range findings {
		if finding.Severity != "warning" && finding.Severity != "info" {
			return false
		}
	}
	return true
}

func auditFindingJSON(finding assessmentAuditFindingResponse) string {
	raw, _ := json.Marshal(finding.Details)
	return string(raw)
}

func auditFindingID(runID string, evaluationID uint, code string) string {
	return fmt.Sprintf("%s:%d:%s", runID, evaluationID, code)
}
