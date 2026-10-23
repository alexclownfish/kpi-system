package handlers

import (
	"net/http"

	"dootask-kpi-server/models"
	"gorm.io/gorm"
)

type workflowAccessError struct {
	Status  int
	Code    string
	Message string
	Details any
}

func (err *workflowAccessError) Error() string { return err.Message }

func canViewWorkflowEvaluation(db *gorm.DB, userID, evaluationID uint) bool {
	var visible int64
	if err := ApplyEvaluationScope(db.Model(&models.KPIEvaluation{}), userID).
		Where("kpi_evaluations.id = ?", evaluationID).Count(&visible).Error; err == nil && visible > 0 {
		return true
	}
	var assigned int64
	return db.Model(&models.EvaluationScoringTask{}).
		Where("evaluation_id = ? AND scorer_user_id = ?", evaluationID, userID).
		Count(&assigned).Error == nil && assigned > 0
}

func loadScoringTaskForWrite(db *gorm.DB, userID, taskID uint, expectedVersion int) (models.EvaluationScoringTask, error) {
	var task models.EvaluationScoringTask
	if err := db.First(&task, taskID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return task, &workflowAccessError{Status: http.StatusNotFound, Code: "SCORING_TASK_NOT_FOUND", Message: "评分任务不存在"}
		}
		return task, err
	}
	allowOverride := task.ScorerUserID != userID && UserHasPermission(userID, "assessment:approve")
	if task.ScorerUserID != userID && !allowOverride {
		return task, &workflowAccessError{Status: http.StatusForbidden, Code: "SCORING_TASK_FORBIDDEN", Message: "您不是该指标的评分人"}
	}
	if !canViewWorkflowEvaluation(db, userID, task.EvaluationID) {
		return task, &workflowAccessError{Status: http.StatusForbidden, Code: "ASSESSMENT_SCOPE_FORBIDDEN", Message: "无权访问该考核"}
	}
	if expectedVersion > 0 && task.Version != expectedVersion {
		return task, &workflowAccessError{
			Status: http.StatusConflict, Code: "TASK_VERSION_CONFLICT", Message: "评分任务已被其他操作更新，请刷新后重试",
			Details: map[string]any{"expected_version": expectedVersion, "current_version": task.Version},
		}
	}
	if task.Status == "locked" || task.LockedAt != nil {
		return task, &workflowAccessError{Status: http.StatusConflict, Code: "TASK_LOCKED", Message: "评分任务已锁定，不能继续修改"}
	}
	return task, nil
}

// guardScoringTaskWorkflowState prevents direct scoring-task writes from
// bypassing the weighted workflow state machine. Once all required work has
// been completed and the result is awaiting employee confirmation (or has
// already been finalized), the submitted result and sign-off snapshot must
// remain immutable.
func guardScoringTaskWorkflowState(evaluation models.KPIEvaluation) *workflowAccessError {
	// Terminal/confirmation states are immutable for both legacy and weighted
	// evaluations. Legacy records normally do not materialize scoring tasks,
	// but migrated tasks must obey the same boundary.
	switch evaluation.Status {
	case "pending_confirm", workflowStateAwaitingEmployeeConfirmation, workflowStateCompleted, workflowStateRejected, workflowStateCancelled:
		return &workflowAccessError{
			Status:  http.StatusConflict,
			Code:    "SCORING_WORKFLOW_STATE",
			Message: "当前考核阶段不能修改评分任务",
			Details: map[string]any{"state": evaluation.Status},
		}
	}
	if !isNewWorkflowEvaluation(evaluation) {
		return nil
	}
	if evaluation.Status == workflowStateScoring || evaluation.Status == workflowStateAwaitingReview {
		return nil
	}
	return &workflowAccessError{
		Status:  http.StatusConflict,
		Code:    "SCORING_WORKFLOW_STATE",
		Message: "当前考核阶段不能修改评分任务",
		Details: map[string]any{"state": evaluation.Status},
	}
}
