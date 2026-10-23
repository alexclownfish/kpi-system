package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	workflowStateDraft                        = "draft"
	workflowStateSelfEvaluating               = "self_evaluating"
	workflowStateScoring                      = "scoring"
	workflowStateAwaitingReview               = "awaiting_review"
	workflowStateAwaitingEmployeeConfirmation = "awaiting_employee_confirmation"
	workflowStateCompleted                    = "completed"
	workflowStateRejected                     = "rejected"
	workflowStateCancelled                    = "cancelled"
)

var (
	errWorkflowStateConflict    = errors.New("考核状态已变化，请刷新后重试")
	errRequiredTasksIncomplete  = errors.New("仍有必填评分任务未完成")
	errSelfEvaluationIncomplete = errors.New("仍有必填自评指标未完成")
	errWorkflowTerminal         = errors.New("当前考核状态不能继续推进")
)

type workflowGuardFailure struct {
	Status  int
	Code    string
	Message string
	Details any
}

type workflowAdvanceRequest struct {
	ExpectedState string `json:"expected_state"`
}

type workflowAdvanceResult struct {
	PreviousState string `json:"previous_state"`
	CurrentState  string `json:"current_state"`
	BlockingTasks []any  `json:"blocking_tasks"`
	ResultVersion int    `json:"result_version"`
}

// nextWorkflowState is the state-machine core. It never accepts a client
// target state or client-calculated total; the next state is derived from
// server-side completion facts.
func nextWorkflowState(current string, selfComplete, tasksComplete bool) (string, error) {
	switch current {
	case workflowStateDraft:
		return workflowStateSelfEvaluating, nil
	case workflowStateSelfEvaluating:
		if !selfComplete {
			return current, errSelfEvaluationIncomplete
		}
		return workflowStateScoring, nil
	case workflowStateScoring, workflowStateAwaitingReview:
		if !tasksComplete {
			return current, errRequiredTasksIncomplete
		}
		return workflowStateAwaitingEmployeeConfirmation, nil
	case workflowStateAwaitingEmployeeConfirmation, workflowStateCompleted, workflowStateRejected, workflowStateCancelled:
		return current, errWorkflowTerminal
	default:
		return current, errWorkflowStateConflict
	}
}

func canConfirmEvaluationState(status string) bool {
	return status == "pending_confirm" || status == workflowStateAwaitingEmployeeConfirmation
}

// confirmationStateForEvaluation keeps the legacy and weighted-v2 state
// vocabularies compatible while preventing a new-workflow assessment from
// entering confirmation through the legacy pending_confirm shortcut.
func confirmationStateForEvaluation(evaluation models.KPIEvaluation) bool {
	if isNewWorkflowEvaluation(evaluation) {
		return evaluation.Status == workflowStateAwaitingEmployeeConfirmation
	}
	return evaluation.Status == "pending_confirm"
}

// guardEvaluationConfirmation is the single backend guard used by both
// online confirmation and paper sign-off. It rechecks completion facts at the
// transition boundary so a caller cannot bypass the workflow by invoking a
// confirmation endpoint directly.
func guardEvaluationConfirmation(db *gorm.DB, evaluation models.KPIEvaluation) *workflowGuardFailure {
	if !confirmationStateForEvaluation(evaluation) {
		return &workflowGuardFailure{Status: http.StatusConflict, Code: "WORKFLOW_CONFIRMATION_STATE", Message: "当前考核状态不能确认最终得分"}
	}
	if evaluation.HasObjection {
		return &workflowGuardFailure{Status: http.StatusBadRequest, Code: "WORKFLOW_OBJECTION_PENDING", Message: "异议尚未处理，暂不能确认"}
	}
	if !isNewWorkflowEvaluation(evaluation) {
		return nil
	}
	snapshot, err := parseWorkflowSnapshotForEvaluation(evaluation)
	if err != nil {
		return &workflowGuardFailure{Status: http.StatusUnprocessableEntity, Code: "SNAPSHOT_INVALID", Message: err.Error()}
	}
	selfComplete, blockingSelf := selfEvaluationCompletion(db, evaluation, snapshot)
	tasksComplete, blockingTasks := requiredScoringTasksCompletion(db, evaluation.ID, snapshot)
	if !selfComplete || !tasksComplete {
		blocking := append(blockingSelf, blockingTasks...)
		return &workflowGuardFailure{
			Status: http.StatusConflict, Code: "REQUIRED_SCORES_INCOMPLETE",
			Message: "仍有必填自评或评分任务未完成", Details: map[string]any{"blocking_tasks": blocking},
		}
	}
	return nil
}

func objectionSubmissionAllowed(evaluation models.KPIEvaluation) bool {
	if isNewWorkflowEvaluation(evaluation) {
		return evaluation.Status == workflowStateAwaitingEmployeeConfirmation
	}
	return evaluation.Status == "pending_confirm"
}

func objectionHandlingAllowed(evaluation models.KPIEvaluation) bool {
	if isNewWorkflowEvaluation(evaluation) {
		return evaluation.Status == workflowStateRejected
	}
	return evaluation.Status == "pending_confirm"
}

func isNewWorkflowEvaluation(evaluation models.KPIEvaluation) bool {
	if evaluation.CalculationVersion == canonicalCalculationVersion {
		return true
	}
	snapshot, err := parseEvaluationSnapshot(evaluation.TemplateSnapshotJSON)
	return err == nil && !snapshot.Legacy && snapshot.CalculationVersion == canonicalCalculationVersion
}

func AdvanceEvaluationWorkflow(c *gin.Context) {
	evaluationID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || evaluationID == 0 {
		respondError(c, http.StatusBadRequest, "INVALID_EVALUATION_ID", "考核ID无效", nil)
		return
	}
	var request workflowAdvanceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_WORKFLOW_REQUEST", "流程推进参数无效", nil)
		return
	}
	userID := c.GetUint("user_id")
	var evaluation models.KPIEvaluation
	if err := models.DB.Preload("Employee").First(&evaluation, uint(evaluationID)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(c, http.StatusNotFound, "EVALUATION_NOT_FOUND", "考核不存在", nil)
			return
		}
		respondError(c, http.StatusInternalServerError, "WORKFLOW_READ_FAILED", "读取考核失败", nil)
		return
	}
	if !canViewWorkflowEvaluation(models.DB, userID, evaluation.ID) {
		respondError(c, http.StatusForbidden, "WORKFLOW_FORBIDDEN", "无权推进该考核流程", nil)
		return
	}
	if request.ExpectedState != "" && request.ExpectedState != evaluation.Status {
		respondError(c, http.StatusConflict, "WORKFLOW_STATE_CONFLICT", errWorkflowStateConflict.Error(), map[string]any{"current_state": evaluation.Status})
		return
	}
	snapshot, err := parseWorkflowSnapshotForEvaluation(evaluation)
	if err != nil {
		respondError(c, http.StatusConflict, "SNAPSHOT_INVALID", err.Error(), nil)
		return
	}
	selfComplete, blockingSelf := selfEvaluationCompletion(models.DB, evaluation, snapshot)
	tasksComplete, blockingTasks := requiredScoringTasksCompletion(models.DB, evaluation.ID, snapshot)
	next, transitionErr := nextWorkflowState(evaluation.Status, selfComplete, tasksComplete)
	if transitionErr != nil {
		code := "WORKFLOW_STATE_CONFLICT"
		status := http.StatusConflict
		if errors.Is(transitionErr, errRequiredTasksIncomplete) || errors.Is(transitionErr, errSelfEvaluationIncomplete) {
			code, status = "REQUIRED_SCORES_INCOMPLETE", http.StatusConflict
		}
		blocking := append(blockingSelf, blockingTasks...)
		if errors.Is(transitionErr, errRequiredTasksIncomplete) || errors.Is(transitionErr, errSelfEvaluationIncomplete) {
			RecordAuditDetails(c, "confirmation_gate_rejected", "evaluation", strconv.FormatUint(uint64(evaluation.ID), 10), "REJECTED", fmt.Sprintf("blocking_tasks=%d", len(blocking)))
		}
		respondError(c, status, code, transitionErr.Error(), map[string]any{"blocking_tasks": blocking})
		return
	}
	if next != evaluation.Status {
		tx := models.DB.Begin()
		if tx.Error != nil {
			respondError(c, http.StatusInternalServerError, "WORKFLOW_WRITE_FAILED", "推进考核流程失败", nil)
			return
		}
		calculated := false
		if next == workflowStateAwaitingEmployeeConfirmation {
			if _, _, calcErr := persistCanonicalEvaluationResult(tx, evaluation.ID, userID); calcErr != nil {
				tx.Rollback()
				respondError(c, http.StatusUnprocessableEntity, "RESULT_CALCULATION_FAILED", calcErr.Error(), nil)
				return
			}
			calculated = true
		}
		if err := tx.Model(&models.KPIEvaluation{}).Where("id = ? AND status = ?", evaluation.ID, evaluation.Status).Update("status", next).Error; err != nil || tx.Commit().Error != nil {
			tx.Rollback()
			respondError(c, http.StatusInternalServerError, "WORKFLOW_WRITE_FAILED", "推进考核流程失败", nil)
			return
		}
		// Write audit records only after the calculation and state transition are
		// committed. This keeps the audit row from racing the SQLite transaction
		// and avoids claiming a calculation that was rolled back.
		if calculated {
			RecordAuditDetails(c, "evaluation_calculated", "evaluation", strconv.FormatUint(uint64(evaluation.ID), 10), "SUCCESS", "workflow_advance")
		}
		RecordAuditDetails(c, "workflow_advance", "evaluation", strconv.FormatUint(uint64(evaluation.ID), 10), "SUCCESS", fmt.Sprintf("from=%s;to=%s", evaluation.Status, next))
	}
	respondData(c, http.StatusOK, workflowAdvanceResult{PreviousState: evaluation.Status, CurrentState: next, BlockingTasks: []any{}, ResultVersion: 0}, "考核流程已推进")
}

func selfEvaluationCompletion(db *gorm.DB, evaluation models.KPIEvaluation, snapshot parsedEvaluationSnapshot) (bool, []any) {
	blocking := make([]any, 0)
	if complete, summaryBlocking := workSummaryCompletion(evaluation); !complete {
		blocking = append(blocking, summaryBlocking...)
	}
	for _, item := range snapshot.Items {
		if !item.Required {
			continue
		}
		var score models.KPIScore
		err := db.Where("evaluation_id = ? AND item_id = ?", evaluation.ID, item.ID).First(&score).Error
		if err != nil || score.SelfScore == nil {
			blocking = append(blocking, map[string]any{"item_id": item.ID, "item_name": item.Name, "stage": "self"})
		}
	}
	return len(blocking) == 0, blocking
}

func requiredScoringTasksCompletion(db *gorm.DB, evaluationID uint, snapshot parsedEvaluationSnapshot) (bool, []any) {
	var tasks []models.EvaluationScoringTask
	if err := db.Where("evaluation_id = ? AND required = ?", evaluationID, true).Order("item_id ASC, id ASC").Find(&tasks).Error; err != nil {
		return false, []any{map[string]any{"stage": "scoring", "error": err.Error()}}
	}
	blocking := make([]any, 0)
	itemTaskCounts := make(map[uint]int)
	for _, task := range tasks {
		itemTaskCounts[task.ItemID]++
		if task.Status == "submitted" {
			continue
		}
		// Required tasks cannot be bypassed by manually setting
		// not_applicable; the write endpoint rejects this already, and this
		// second check protects migrated/corrupt rows at the confirmation gate.
		blocking = append(blocking, map[string]any{"task_id": task.ID, "item_id": task.ItemID, "scorer_user_id": task.ScorerUserID, "status": task.Status})
	}
	for _, item := range snapshot.Items {
		if item.Required && itemTaskCounts[item.ID] == 0 {
			blocking = append(blocking, map[string]any{"item_id": item.ID, "item_name": item.Name, "stage": "scoring", "reason": "评分任务未生成"})
		}
	}
	return len(blocking) == 0, blocking
}
