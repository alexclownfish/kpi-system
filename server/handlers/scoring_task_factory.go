package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"dootask-kpi-server/models"
	"gorm.io/gorm"
)

// generateEvaluationScoringTasks materializes the item-level assignments
// captured in an assessment snapshot. It is transactional and idempotent so
// a retry cannot create duplicate work for the same assessment/item/user/duty.
func generateEvaluationScoringTasks(tx *gorm.DB, evaluation models.KPIEvaluation, snapshot parsedEvaluationSnapshot) ([]models.EvaluationScoringTask, error) {
	assignments := snapshot.ResolvedScorers
	if len(assignments) == 0 {
		return nil, fmt.Errorf("考核快照没有评分任务分配")
	}
	items := make(map[uint]workflowSnapshotItem, len(snapshot.Items))
	for _, item := range snapshot.Items {
		items[item.ID] = item
	}
	created := make([]models.EvaluationScoringTask, 0, len(assignments))
	for _, assignment := range assignments {
		if assignment.ItemID == 0 {
			// Legacy snapshots store one scorer per type without item IDs. New
			// records must be item-specific; callers should use the workflow
			// snapshot builder instead of trying to materialize legacy data.
			continue
		}
		item, ok := items[assignment.ItemID]
		if !ok {
			return nil, fmt.Errorf("评分任务引用了不存在的指标: %d", assignment.ItemID)
		}
		var existing models.EvaluationScoringTask
		err := tx.Where("evaluation_id = ? AND item_id = ? AND scorer_user_id = ? AND scorer_type = ?", evaluation.ID, assignment.ItemID, assignment.UserID, assignment.ScorerType).First(&existing).Error
		if err == nil {
			continue
		}
		if err != gorm.ErrRecordNotFound {
			return nil, err
		}
		task := models.EvaluationScoringTask{
			EvaluationID: evaluation.ID, ItemID: assignment.ItemID, ScorerUserID: assignment.UserID,
			ScorerType: assignment.ScorerType, Required: item.Required, Status: "pending", Version: 1,
			Sequence: scorerSequence(assignment.ScorerType), SourceRule: assignment.SourceRule,
		}
		if err := tx.Create(&task).Error; err != nil {
			return nil, err
		}
		created = append(created, task)
	}
	return created, nil
}

func scorerSequence(scorerType string) int {
	switch strings.ToUpper(strings.TrimSpace(scorerType)) {
	case "PM":
		return 1
	case "DEPARTMENT_MANAGER", "DIRECT_MANAGER", "CTO":
		return 2
	case "PRODUCT_MANAGER":
		return 3
	default:
		return 2
	}
}

func parseWorkflowSnapshotForEvaluation(evaluation models.KPIEvaluation) (parsedEvaluationSnapshot, error) {
	if evaluation.TemplateSnapshotJSON == "" {
		return parsedEvaluationSnapshot{}, fmt.Errorf("考核缺少模板快照")
	}
	return parseEvaluationSnapshot(evaluation.TemplateSnapshotJSON)
}

func taskItemFromSnapshot(snapshot parsedEvaluationSnapshot, itemID uint) (workflowSnapshotItem, error) {
	for _, item := range snapshot.Items {
		if item.ID == itemID {
			return item, nil
		}
	}
	return workflowSnapshotItem{}, fmt.Errorf("模板快照中不存在指标%d", itemID)
}

func encodeTaskDetails(task models.EvaluationScoringTask, item workflowSnapshotItem) map[string]any {
	label := map[string]string{
		"PM": "项目经理", "DEPARTMENT_MANAGER": "部门负责人", "PRODUCT_MANAGER": "产品经理",
		"CTO": "CTO", "DIRECT_MANAGER": "直属负责人",
	}[task.ScorerType]
	return map[string]any{
		"id": task.ID, "evaluation_id": task.EvaluationID, "item_id": task.ItemID,
		"item_name": item.Name, "scorer_user_id": task.ScorerUserID, "scorer_type": task.ScorerType, "scorer_label": label,
		"sequence": task.Sequence,
		"required": task.Required, "status": task.Status, "score": task.Score,
		"comment": task.Comment, "not_applicable": task.NotApplicable, "max_score": item.MaxScore,
		"version": task.Version, "source_rule": task.SourceRule,
	}
}

func snapshotAssignmentsJSON(assignments []workflowScoringAssignment) string {
	raw, _ := json.Marshal(assignments)
	return string(raw)
}
