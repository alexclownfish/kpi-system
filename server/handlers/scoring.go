package handlers

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type scoringTaskWriteRequest struct {
	Score         *float64 `json:"score"`
	Comment       string   `json:"comment"`
	NotApplicable bool     `json:"not_applicable"`
	Version       int      `json:"version"`
}

type scoringTaskListResponse struct {
	EvaluationID  uint           `json:"evaluation_id"`
	WorkflowState string         `json:"workflow_state"`
	Completion    map[string]int `json:"completion"`
	BlockingTasks []any          `json:"blocking_tasks"`
	Tasks         []any          `json:"tasks"`
}

func GetEvaluationScoringTasks(c *gin.Context) {
	evaluationID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || evaluationID == 0 {
		respondError(c, http.StatusBadRequest, "INVALID_EVALUATION_ID", "考核ID无效", nil)
		return
	}
	userID := c.GetUint("user_id")
	evaluation, snapshot, err := loadWorkflowEvaluation(models.DB, userID, uint(evaluationID))
	if err != nil {
		writeWorkflowError(c, err)
		return
	}

	query := models.DB.Where("evaluation_id = ?", evaluation.ID).Order("item_id ASC, scorer_type ASC, scorer_user_id ASC")
	if !isWorkflowEvaluationOwnerOrOverride(userID, evaluation) {
		query = query.Where("scorer_user_id = ?", userID)
	}
	var tasks []models.EvaluationScoringTask
	if err := query.Find(&tasks).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "SCORING_TASK_READ_FAILED", "读取评分任务失败", nil)
		return
	}
	earliestIncompleteSequence := 0
	for _, task := range tasks {
		if !task.Required || task.Status == "submitted" || task.Status == "not_applicable" {
			continue
		}
		sequence := task.Sequence
		if sequence == 0 {
			sequence = scorerSequence(task.ScorerType)
		}
		if earliestIncompleteSequence == 0 || sequence < earliestIncompleteSequence {
			earliestIncompleteSequence = sequence
		}
	}
	visible := make([]any, 0, len(tasks))
	for _, task := range tasks {
		item, itemErr := taskItemFromSnapshot(snapshot, task.ItemID)
		if itemErr != nil {
			continue
		}
		entry := encodeTaskDetails(task, item)
		sequence := task.Sequence
		if sequence == 0 {
			sequence = scorerSequence(task.ScorerType)
		}
		entry["sequence"] = sequence
		entry["sequence_ready"] = earliestIncompleteSequence == 0 || sequence <= earliestIncompleteSequence
		entry["can_edit"] = (task.ScorerUserID == userID || UserHasPermission(userID, "assessment:approve")) &&
			guardScoringTaskWorkflowState(evaluation) == nil && task.LockedAt == nil &&
			task.Status != "submitted" && task.Status != "not_applicable" && task.Status != "locked" &&
			entry["sequence_ready"].(bool)
		visible = append(visible, entry)
	}
	completion := scoringTaskCompletion(models.DB, evaluation.ID)
	_, blocking := requiredScoringTasksCompletion(models.DB, evaluation.ID, snapshot)
	respondData(c, http.StatusOK, scoringTaskListResponse{EvaluationID: evaluation.ID, WorkflowState: evaluation.Status, Completion: completion, BlockingTasks: blocking, Tasks: visible}, "评分任务读取成功")
}

func UpdateScoringTask(c *gin.Context) {
	writeScoringTask(c, false)
}

func SubmitScoringTask(c *gin.Context) {
	writeScoringTask(c, true)
}

func writeScoringTask(c *gin.Context, submit bool) {
	taskID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || taskID == 0 {
		respondError(c, http.StatusBadRequest, "INVALID_SCORING_TASK_ID", "评分任务ID无效", nil)
		return
	}
	var request scoringTaskWriteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_SCORING_TASK", "评分任务参数无效", map[string]any{"reason": err.Error()})
		return
	}
	userID := c.GetUint("user_id")
	task, accessErr := loadScoringTaskForWrite(models.DB, userID, uint(taskID), request.Version)
	if accessErr != nil {
		writeWorkflowError(c, accessErr)
		return
	}
	evaluation, snapshot, err := loadWorkflowEvaluation(models.DB, userID, task.EvaluationID)
	if err != nil {
		writeWorkflowError(c, err)
		return
	}
	if guard := guardScoringTaskWorkflowState(evaluation); guard != nil {
		writeWorkflowError(c, guard)
		return
	}
	item, err := taskItemFromSnapshot(snapshot, task.ItemID)
	if err != nil {
		respondError(c, http.StatusConflict, "SNAPSHOT_INVALID", err.Error(), nil)
		return
	}
	if task.Status == "submitted" || task.Status == "not_applicable" || task.Status == "locked" {
		respondError(c, http.StatusConflict, "TASK_LOCKED", "评分任务已提交或锁定，不能继续修改", nil)
		return
	}
	if submit && isNewWorkflowEvaluation(evaluation) {
		sequence := task.Sequence
		if sequence == 0 {
			sequence = scorerSequence(task.ScorerType)
		}
		var earlier []models.EvaluationScoringTask
		if err := models.DB.Where("evaluation_id = ? AND required = ? AND sequence > 0 AND sequence < ? AND status <> ?", evaluation.ID, true, sequence, "submitted").Order("sequence, id").Find(&earlier).Error; err != nil {
			respondError(c, http.StatusInternalServerError, "SCORING_SEQUENCE_READ_FAILED", "读取评分顺位失败", nil)
			return
		}
		if len(earlier) > 0 {
			blocking := make([]any, 0, len(earlier))
			for _, pending := range earlier {
				blocking = append(blocking, map[string]any{
					"task_id": pending.ID, "scorer_type": pending.ScorerType,
					"scorer_user_id": pending.ScorerUserID, "sequence": pending.Sequence,
				})
			}
			respondError(c, http.StatusConflict, "SCORING_SEQUENCE_BLOCKED", "请等待前序评分人完成", map[string]any{"blocking_tasks": blocking})
			return
		}
	}
	if err := validateScoringTaskInput(request, item, submit); err != nil {
		code := "INVALID_SCORING_TASK"
		status := http.StatusBadRequest
		if errors.Is(err, errScoreOutOfRange) {
			code, status = "SCORE_OUT_OF_RANGE", http.StatusBadRequest
		} else if errors.Is(err, errScoreRequired) {
			code, status = "SCORE_REQUIRED", http.StatusUnprocessableEntity
		} else if errors.Is(err, errNotApplicableNotAllowed) {
			code, status = "NOT_APPLICABLE_NOT_ALLOWED", http.StatusUnprocessableEntity
		} else if errors.Is(err, errNotApplicableReasonRequired) {
			code, status = "NOT_APPLICABLE_REASON_REQUIRED", http.StatusUnprocessableEntity
		}
		respondError(c, status, code, err.Error(), nil)
		return
	}

	status := "in_progress"
	var submittedAt *time.Time
	if request.Score == nil && strings.TrimSpace(request.Comment) == "" && !request.NotApplicable {
		status = "pending"
	}
	if submit {
		status = "submitted"
		now := time.Now()
		submittedAt = &now
		if request.NotApplicable {
			status = "not_applicable"
		}
	}
	updates := map[string]any{
		"score": request.Score, "comment": strings.TrimSpace(request.Comment),
		"not_applicable": request.NotApplicable, "status": status,
		"version": task.Version + 1, "submitted_at": submittedAt,
	}
	query := models.DB.Model(&models.EvaluationScoringTask{}).
		Where("id = ? AND version = ? AND locked_at IS NULL AND status NOT IN ?", task.ID, task.Version, []string{"submitted", "not_applicable", "locked"})
	if isNewWorkflowEvaluation(evaluation) {
		// Recheck the phase in the actual write to reject a request that loaded
		// its task just before another request advanced or confirmed the result.
		query = query.Where("evaluation_id IN (SELECT id FROM kpi_evaluations WHERE status IN ?)", []string{workflowStateScoring, workflowStateAwaitingReview})
	} else {
		// Legacy records may have been migrated with item-level tasks. Keep
		// confirmation and terminal states immutable there as well.
		query = query.Where("evaluation_id IN (SELECT id FROM kpi_evaluations WHERE status NOT IN ?)", []string{"pending_confirm", workflowStateAwaitingEmployeeConfirmation, workflowStateCompleted, workflowStateRejected, workflowStateCancelled})
	}
	result := query.Updates(updates)
	if result.Error != nil {
		respondError(c, http.StatusInternalServerError, "SCORING_TASK_WRITE_FAILED", "保存评分任务失败", nil)
		return
	}
	if result.RowsAffected == 0 {
		respondError(c, http.StatusConflict, "TASK_VERSION_CONFLICT", "评分任务已被其他操作更新，请刷新后重试", nil)
		return
	}
	var saved models.EvaluationScoringTask
	if err := models.DB.First(&saved, task.ID).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "SCORING_TASK_READ_FAILED", "读取更新后的评分任务失败", nil)
		return
	}
	if submit && isNewWorkflowEvaluation(evaluation) {
		// A partial submission is accepted even when required tasks remain. The
		// canonical result is persisted once all required tasks are complete;
		// incomplete-result errors are intentionally non-fatal to this task write.
		if tx := models.DB.Begin(); tx.Error == nil {
			if _, _, calcErr := persistCanonicalEvaluationResult(tx, evaluation.ID, userID); calcErr == nil {
				_ = tx.Commit().Error
			} else {
				tx.Rollback()
			}
		}
	}
	entry := encodeTaskDetails(saved, item)
	if submit {
		RecordAuditDetails(c, "score_submitted", "scoring_task", strconv.FormatUint(uint64(saved.ID), 10), "SUCCESS", fmt.Sprintf("evaluation_id=%d;item_id=%d;scorer_type=%s", saved.EvaluationID, saved.ItemID, saved.ScorerType))
	}
	if submit {
		respondData(c, http.StatusOK, map[string]any{"task": entry, "workflow": map[string]any{"state": evaluation.Status, "blocking_tasks": scoringTaskCompletion(models.DB, evaluation.ID)["blocking"]}}, "评分任务提交成功")
		return
	}
	respondData(c, http.StatusOK, entry, "评分草稿保存成功")
}

var (
	errScoreOutOfRange             = errors.New("评分超出指标允许范围")
	errScoreRequired               = errors.New("提交评分时必须填写分数或选择不适用")
	errNotApplicableNotAllowed     = errors.New("当前必填指标不允许标记为不适用")
	errNotApplicableReasonRequired = errors.New("标记为不适用时必须填写理由")
)

func validateScoringTaskInput(request scoringTaskWriteRequest, item workflowSnapshotItem, submit bool) error {
	if request.NotApplicable && request.Score != nil {
		return fmt.Errorf("不适用不能与数值评分同时提交")
	}
	if request.Score != nil && (math.IsNaN(*request.Score) || math.IsInf(*request.Score, 0) || *request.Score < 0 || *request.Score > item.MaxScore) {
		return errScoreOutOfRange
	}
	if request.NotApplicable && item.Required {
		return errNotApplicableNotAllowed
	}
	if request.NotApplicable && submit && strings.TrimSpace(request.Comment) == "" {
		return errNotApplicableReasonRequired
	}
	if submit && request.Score == nil && !request.NotApplicable {
		return errScoreRequired
	}
	return nil
}

func loadWorkflowEvaluation(db *gorm.DB, userID, evaluationID uint) (models.KPIEvaluation, parsedEvaluationSnapshot, error) {
	var evaluation models.KPIEvaluation
	if err := db.Preload("Employee").First(&evaluation, evaluationID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return evaluation, parsedEvaluationSnapshot{}, &workflowAccessError{Status: http.StatusNotFound, Code: "EVALUATION_NOT_FOUND", Message: "考核不存在"}
		}
		return evaluation, parsedEvaluationSnapshot{}, err
	}
	if !canViewWorkflowEvaluation(db, userID, evaluationID) {
		return evaluation, parsedEvaluationSnapshot{}, &workflowAccessError{Status: http.StatusForbidden, Code: "ASSESSMENT_SCOPE_FORBIDDEN", Message: "无权访问该考核"}
	}
	snapshot, err := parseWorkflowSnapshotForEvaluation(evaluation)
	if err != nil {
		return evaluation, snapshot, &workflowAccessError{Status: http.StatusConflict, Code: "SNAPSHOT_INVALID", Message: err.Error()}
	}
	return evaluation, snapshot, nil
}

func isWorkflowEvaluationOwnerOrOverride(userID uint, evaluation models.KPIEvaluation) bool {
	return evaluation.EmployeeID == userID || UserHasPermission(userID, "assessment:approve")
}

func scoringTaskCompletion(db *gorm.DB, evaluationID uint) map[string]int {
	var required, submitted, blocking int64
	db.Model(&models.EvaluationScoringTask{}).Where("evaluation_id = ? AND required = ?", evaluationID, true).Count(&required)
	// A required task is complete only after a real score is submitted. N/A is
	// intentionally excluded here because the workflow gate rejects N/A for
	// required indicators, including rows migrated from older data.
	db.Model(&models.EvaluationScoringTask{}).Where("evaluation_id = ? AND required = ? AND status = ?", evaluationID, true, "submitted").Count(&submitted)
	blocking = required - submitted
	if blocking < 0 {
		blocking = 0
	}
	return map[string]int{"required": int(required), "submitted": int(submitted), "blocking": int(blocking)}
}

func writeWorkflowError(c *gin.Context, err error) {
	if accessErr, ok := err.(*workflowAccessError); ok {
		respondError(c, accessErr.Status, accessErr.Code, accessErr.Message, accessErr.Details)
		return
	}
	respondError(c, http.StatusInternalServerError, "WORKFLOW_REQUEST_FAILED", err.Error(), nil)
}
