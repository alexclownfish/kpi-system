package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type assessmentMigrationRequest struct {
	EvaluationIDs []uint `json:"evaluation_ids"`
	TargetVersion string `json:"target_version"`
}

type migrationPlanRecord struct {
	EvaluationID   uint                     `json:"evaluation_id"`
	SourceChecksum string                   `json:"source_checksum"`
	Snapshot       parsedEvaluationSnapshot `json:"snapshot"`
}

type migrationDryRunResponse struct {
	BatchID               string           `json:"batch_id"`
	Mode                  string           `json:"mode"`
	EligibleEvaluationIDs []uint           `json:"eligible_evaluation_ids"`
	ManualReview          []map[string]any `json:"manual_review"`
	PlannedTaskCount      int              `json:"planned_task_count"`
}

func buildMigrationPlan(db *gorm.DB, ids []uint, actorID uint) ([]migrationPlanRecord, []map[string]any, error) {
	query := ApplyEvaluationScope(db.Model(&models.KPIEvaluation{}), actorID)
	if len(ids) > 0 {
		query = query.Where("kpi_evaluations.id IN ?", ids)
	}
	var evaluations []models.KPIEvaluation
	if err := query.Order("kpi_evaluations.id ASC").Find(&evaluations).Error; err != nil {
		return nil, nil, err
	}
	plans := make([]migrationPlanRecord, 0, len(evaluations))
	manual := make([]map[string]any, 0)
	for _, evaluation := range evaluations {
		snapshot, err := parseEvaluationSnapshot(evaluation.TemplateSnapshotJSON)
		if err != nil || snapshot.Legacy {
			reason := "快照缺失或不可解析"
			if err == nil && snapshot.Legacy {
				reason = "旧版快照需要人工确认评分人映射"
			}
			manual = append(manual, map[string]any{"evaluation_id": evaluation.ID, "code": "MANUAL_REVIEW_REQUIRED", "reason": reason})
			continue
		}
		findings := auditEvaluation(db, evaluation)
		blocked := false
		for _, finding := range findings {
			// A missing task set is exactly what this migration operation repairs.
			// Other manual-review/error findings still block automatic apply.
			if finding.Code == "TASKS_MISSING" {
				continue
			}
			if !finding.EligibleForMigration || finding.Severity == "error" || finding.Severity == "manual_review" {
				blocked = true
				break
			}
		}
		if blocked {
			manual = append(manual, map[string]any{"evaluation_id": evaluation.ID, "code": "AUDIT_FINDING", "reason": "审计发现需要人工复核的问题"})
			continue
		}
		plans = append(plans, migrationPlanRecord{EvaluationID: evaluation.ID, SourceChecksum: snapshot.Checksum, Snapshot: snapshot})
	}
	return plans, manual, nil
}

func DryRunAssessmentMigration(c *gin.Context) {
	var request assessmentMigrationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_MIGRATION_FILTER", "迁移筛选条件无效", nil)
		return
	}
	if request.TargetVersion == "" {
		request.TargetVersion = canonicalCalculationVersion
	}
	if request.TargetVersion != canonicalCalculationVersion {
		respondError(c, http.StatusBadRequest, "INVALID_MIGRATION_VERSION", "目标版本不受支持", nil)
		return
	}
	actorID := c.GetUint("user_id")
	plans, manual, err := buildMigrationPlan(models.DB, request.EvaluationIDs, actorID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "MIGRATION_DRY_RUN_FAILED", "迁移预演失败", nil)
		return
	}
	batchID := uuid.NewString()
	payload, _ := json.Marshal(plans)
	summary := map[string]any{"eligible": len(plans), "manual_review": len(manual), "planned_task_count": 0}
	for _, plan := range plans {
		summary["planned_task_count"] = summary["planned_task_count"].(int) + len(plan.Snapshot.ResolvedScorers)
	}
	summaryJSON, _ := json.Marshal(summary)
	filtersJSON, _ := json.Marshal(request)
	batch := models.MigrationBatch{ID: batchID, Mode: "dry_run", Status: "completed", SourceVersion: "legacy-or-weighted-v2", TargetVersion: request.TargetVersion, FiltersJSON: string(filtersJSON), PayloadJSON: string(payload), SummaryJSON: string(summaryJSON), OperatorID: actorID, CreatedAt: time.Now(), CompletedAt: ptrTime(time.Now())}
	if err := models.DB.Create(&batch).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "MIGRATION_DRY_RUN_FAILED", "保存迁移预演失败", nil)
		return
	}
	eligible := make([]uint, 0, len(plans))
	taskCount := 0
	for _, plan := range plans {
		eligible = append(eligible, plan.EvaluationID)
		taskCount += len(plan.Snapshot.ResolvedScorers)
	}
	RecordAuditDetails(c, "assessment_migration_dry_run", "migration_batch", batchID, "SUCCESS", fmt.Sprintf("eligible=%d manual_review=%d", len(eligible), len(manual)))
	respondData(c, http.StatusOK, migrationDryRunResponse{BatchID: batchID, Mode: "dry_run", EligibleEvaluationIDs: eligible, ManualReview: manual, PlannedTaskCount: taskCount}, "迁移预演完成；未修改业务数据")
}

func ApplyAssessmentMigration(c *gin.Context) {
	batchID := c.Param("batchId")
	var batch models.MigrationBatch
	if err := models.DB.Where("id = ?", batchID).First(&batch).Error; err != nil {
		respondError(c, http.StatusNotFound, "MIGRATION_NOT_FOUND", "迁移批次不存在", nil)
		return
	}
	if batch.Status == "applied" {
		respondError(c, http.StatusConflict, "MIGRATION_ALREADY_APPLIED", "迁移批次已经执行", nil)
		return
	}
	if batch.Status != "completed" || batch.Mode != "dry_run" {
		respondError(c, http.StatusConflict, "MIGRATION_NOT_REVIEWED", "只有已完成的预演批次可以执行", nil)
		return
	}
	var plans []migrationPlanRecord
	if err := json.Unmarshal([]byte(batch.PayloadJSON), &plans); err != nil {
		respondError(c, http.StatusConflict, "MIGRATION_PAYLOAD_INVALID", "迁移批次内容无效", nil)
		return
	}
	actorID := c.GetUint("user_id")
	created := 0
	err := models.DB.Transaction(func(tx *gorm.DB) error {
		for _, plan := range plans {
			var evaluation models.KPIEvaluation
			if err := ApplyEvaluationScope(tx, actorID).First(&evaluation, plan.EvaluationID).Error; err != nil {
				return fmt.Errorf("考核%d不在管理范围内", plan.EvaluationID)
			}
			current, err := parseEvaluationSnapshot(evaluation.TemplateSnapshotJSON)
			if err != nil || current.Checksum != plan.SourceChecksum {
				return fmt.Errorf("考核%d来源已变化，请重新预演", plan.EvaluationID)
			}
			tasks, err := generateEvaluationScoringTasks(tx, evaluation, current)
			if err != nil {
				return err
			}
			created += len(tasks)
		}
		return tx.Model(&models.MigrationBatch{}).Where("id = ? AND status = ?", batch.ID, "completed").Updates(map[string]any{"status": "applied", "mode": "apply", "operator_id": actorID, "completed_at": time.Now()}).Error
	})
	if err != nil {
		code := "MIGRATION_APPLY_FAILED"
		if strings.Contains(err.Error(), "来源已变化") {
			code = "MIGRATION_SOURCE_CHANGED"
		}
		respondError(c, http.StatusConflict, code, err.Error(), nil)
		return
	}
	RecordAuditDetails(c, "assessment_migration_apply", "migration_batch", batchID, "SUCCESS", fmt.Sprintf("created_tasks=%d", created))
	respondData(c, http.StatusOK, gin.H{"batch_id": batchID, "status": "applied", "created_task_count": created}, "迁移批次执行完成；旧评分记录已保留")
}

func GetAssessmentMigration(c *gin.Context) {
	var batch models.MigrationBatch
	if err := models.DB.Where("id = ?", c.Param("batchId")).First(&batch).Error; err != nil {
		respondError(c, http.StatusNotFound, "MIGRATION_NOT_FOUND", "迁移批次不存在", nil)
		return
	}
	var summary any
	_ = json.Unmarshal([]byte(batch.SummaryJSON), &summary)
	respondData(c, http.StatusOK, gin.H{"batch_id": batch.ID, "mode": batch.Mode, "status": batch.Status, "summary": summary, "created_at": batch.CreatedAt, "completed_at": batch.CompletedAt}, "迁移批次读取成功")
}

func ptrTime(value time.Time) *time.Time { return &value }
