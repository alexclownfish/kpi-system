package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"dootask-kpi-server/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const maxSignoffFileSize int64 = 10 << 20

type resultSnapshotItem struct {
	ItemID               uint     `json:"item_id"`
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	DimensionName        string   `json:"dimension_name,omitempty"`
	DimensionWeight      float64  `json:"dimension_weight,omitempty"`
	ItemWeight           float64  `json:"item_weight,omitempty"`
	CompositeWeight      float64  `json:"composite_weight,omitempty"`
	ScorerType           string   `json:"scorer_type,omitempty"`
	ScorerName           string   `json:"scorer_name,omitempty"`
	WeightedContribution float64  `json:"weighted_contribution,omitempty"`
	HasWeightedFields    bool     `json:"has_weighted_fields,omitempty"`
	MaxScore             float64  `json:"max_score"`
	SelfScore            *float64 `json:"self_score,omitempty"`
	SelfComment          string   `json:"self_comment,omitempty"`
	ManagerScore         *float64 `json:"manager_score,omitempty"`
	ManagerComment       string   `json:"manager_comment,omitempty"`
	HRScore              *float64 `json:"hr_score,omitempty"`
	HRComment            string   `json:"hr_comment,omitempty"`
	FinalScore           float64  `json:"final_score"`
	FinalComment         string   `json:"final_comment"`
}

type resultSnapshotPayload struct {
	EvaluationID  uint                      `json:"evaluation_id"`
	EmployeeID    uint                      `json:"employee_id"`
	EmployeeName  string                    `json:"employee_name"`
	EmployeeEmail string                    `json:"employee_email"`
	Position      string                    `json:"position"`
	Department    string                    `json:"department"`
	TemplateID    uint                      `json:"template_id"`
	TemplateName  string                    `json:"template_name"`
	Period        string                    `json:"period"`
	Year          int                       `json:"year"`
	Month         *int                      `json:"month,omitempty"`
	Quarter       *int                      `json:"quarter,omitempty"`
	TotalScore    float64                   `json:"total_score"`
	Grade         string                    `json:"grade"`
	FinalComment  string                    `json:"final_comment"`
	Objection     string                    `json:"objection_reason"`
	HasObjection  bool                      `json:"has_objection"`
	Items         []resultSnapshotItem      `json:"items"`
	ExportLayout  models.ResultExportLayout `json:"export_layout"`
}

func effectiveFinalScore(score models.KPIScore) float64 {
	if score.FinalScore != nil {
		return *score.FinalScore
	}
	if score.HRScore != nil {
		return *score.HRScore
	}
	if score.ManagerScore != nil {
		return *score.ManagerScore
	}
	if score.SelfScore != nil {
		return *score.SelfScore
	}
	return 0
}

func loadEvaluationForSnapshot(db *gorm.DB, evaluationID uint) (models.KPIEvaluation, error) {
	var evaluation models.KPIEvaluation
	err := db.Preload("Employee.Department").Preload("Template").Preload("Scores.Item").First(&evaluation, evaluationID).Error
	return evaluation, err
}

func buildResultSnapshot(evaluation models.KPIEvaluation) (resultSnapshotPayload, string, string, error) {
	if isNewWorkflowEvaluation(evaluation) {
		return buildCanonicalResultSnapshot(evaluation)
	}
	sort.SliceStable(evaluation.Scores, func(i, j int) bool {
		if evaluation.Scores[i].Item.Order == evaluation.Scores[j].Item.Order {
			return evaluation.Scores[i].ItemID < evaluation.Scores[j].ItemID
		}
		return evaluation.Scores[i].Item.Order < evaluation.Scores[j].Item.Order
	})
	items := make([]resultSnapshotItem, 0, len(evaluation.Scores))
	for _, score := range evaluation.Scores {
		comment := score.FinalComment
		if comment == "" {
			comment = score.HRComment
		}
		items = append(items, resultSnapshotItem{
			ItemID: score.ItemID, Name: score.Item.Name, Description: score.Item.Description,
			MaxScore: score.Item.MaxScore, SelfScore: score.SelfScore, SelfComment: score.SelfComment,
			ManagerScore: score.ManagerScore, ManagerComment: score.ManagerComment,
			HRScore: score.HRScore, HRComment: score.HRComment,
			FinalScore: effectiveFinalScore(score), FinalComment: comment,
		})
	}
	layout, _ := normalizeStoredExportLayout(evaluation.Template.ExportLayoutJSON)
	payload := resultSnapshotPayload{
		EvaluationID: evaluation.ID, EmployeeID: evaluation.EmployeeID, EmployeeName: evaluation.Employee.Name,
		EmployeeEmail: evaluation.Employee.Email, Position: evaluation.Employee.Position,
		Department: evaluation.Employee.Department.Name, TemplateID: evaluation.TemplateID,
		TemplateName: evaluation.Template.Name, Period: evaluation.Period, Year: evaluation.Year,
		Month: evaluation.Month, Quarter: evaluation.Quarter, TotalScore: evaluation.TotalScore,
		Grade:        GradeForScore(evaluation.TotalScore),
		FinalComment: evaluation.FinalComment, Objection: evaluation.ObjectionReason,
		HasObjection: evaluation.HasObjection, Items: items, ExportLayout: layout,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return payload, "", "", err
	}
	sum := sha256.Sum256(raw)
	return payload, string(raw), hex.EncodeToString(sum[:]), nil
}

func buildCanonicalResultSnapshot(evaluation models.KPIEvaluation) (resultSnapshotPayload, string, string, error) {
	return buildCanonicalResultSnapshotWithDB(models.DB, evaluation)
}

func buildCanonicalResultSnapshotWithDB(db *gorm.DB, evaluation models.KPIEvaluation) (resultSnapshotPayload, string, string, error) {
	var stored models.EvaluationCalculationResult
	if err := db.Where("evaluation_id = ?", evaluation.ID).Order("version DESC").First(&stored).Error; err != nil {
		return resultSnapshotPayload{}, "", "", fmt.Errorf("统一计算结果不存在: %w", err)
	}
	var calculated canonicalEvaluationResult
	if err := json.Unmarshal([]byte(stored.ResultJSON), &calculated); err != nil {
		return resultSnapshotPayload{}, "", "", fmt.Errorf("统一计算结果无效: %w", err)
	}
	snapshot, err := parseWorkflowSnapshotForEvaluation(evaluation)
	if err != nil {
		return resultSnapshotPayload{}, "", "", err
	}
	definitions := make(map[uint]struct {
		dimensionName   string
		dimensionWeight float64
		itemWeight      float64
	}, len(snapshot.Items))
	for _, dimension := range snapshot.Dimensions {
		for _, item := range dimension.Items {
			definitions[item.ID] = struct {
				dimensionName   string
				dimensionWeight float64
				itemWeight      float64
			}{dimension.Name, dimension.Weight, item.ItemWeight}
		}
	}
	scorerNames := make(map[uint][]string)
	scorerTypes := make(map[uint][]string)
	for _, scorer := range snapshot.ResolvedScorers {
		if scorer.ItemID == 0 {
			continue
		}
		name := strings.TrimSpace(scorer.UserName)
		if name == "" {
			name = scorerTypeLabel(scorer.ScorerType)
		}
		scorerNames[scorer.ItemID] = appendUniqueString(scorerNames[scorer.ItemID], name)
		scorerTypes[scorer.ItemID] = appendUniqueString(scorerTypes[scorer.ItemID], scorerTypeLabel(scorer.ScorerType))
	}
	items := make([]resultSnapshotItem, 0, len(calculated.Items))
	for _, item := range calculated.Items {
		definition := definitions[item.ItemID]
		items = append(items, resultSnapshotItem{
			ItemID: item.ItemID, Name: item.Name, DimensionName: definition.dimensionName,
			DimensionWeight: definition.dimensionWeight, ItemWeight: definition.itemWeight,
			CompositeWeight: item.CompositeWeight, ScorerType: strings.Join(scorerTypes[item.ItemID], ", "),
			ScorerName: strings.Join(scorerNames[item.ItemID], ", "), WeightedContribution: item.Contribution,
			HasWeightedFields: true,
			MaxScore:          item.MaxScore, FinalScore: valueOrZero(item.AggregatedScore), FinalComment: "",
		})
	}
	layout := normalizeExportLayout(snapshot.ExportLayout)
	if layout.Title == "" {
		layout, _ = normalizeStoredExportLayout(evaluation.Template.ExportLayoutJSON)
	}
	payload := resultSnapshotPayload{
		EvaluationID: evaluation.ID, EmployeeID: evaluation.EmployeeID, EmployeeName: evaluation.Employee.Name,
		EmployeeEmail: evaluation.Employee.Email, Position: evaluation.Employee.Position, Department: evaluation.Employee.Department.Name,
		TemplateID: evaluation.TemplateID, TemplateName: snapshot.TemplateName, Period: evaluation.Period, Year: evaluation.Year,
		Month: evaluation.Month, Quarter: evaluation.Quarter, TotalScore: calculated.TotalScore, FinalComment: evaluation.FinalComment,
		Grade:        calculated.Grade,
		HasObjection: evaluation.HasObjection, Objection: evaluation.ObjectionReason, Items: items, ExportLayout: layout,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return payload, "", "", err
	}
	checksum := sha256.Sum256(raw)
	return payload, string(raw), hex.EncodeToString(checksum[:]), nil
}

func valueOrZero(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

// enrichStoredCanonicalPayload fills fields introduced after an older result
// snapshot was persisted. It deliberately returns the original layout and
// snapshot identity so historical checksums stay immutable while exports can
// still render the canonical weighted result and grade.
func enrichStoredCanonicalPayload(db *gorm.DB, evaluation models.KPIEvaluation, payload resultSnapshotPayload) resultSnapshotPayload {
	if !isNewWorkflowEvaluation(evaluation) {
		return payload
	}
	needsWeightedFields := strings.TrimSpace(payload.Grade) == ""
	for _, item := range payload.Items {
		if !item.HasWeightedFields {
			needsWeightedFields = true
			break
		}
	}
	if !needsWeightedFields {
		return payload
	}
	canonical, _, _, err := buildCanonicalResultSnapshotWithDB(db, evaluation)
	if err != nil {
		return payload
	}
	canonicalByID := make(map[uint]resultSnapshotItem, len(canonical.Items))
	for _, item := range canonical.Items {
		canonicalByID[item.ItemID] = item
	}
	for index := range payload.Items {
		if item, ok := canonicalByID[payload.Items[index].ItemID]; ok {
			payload.Items[index].DimensionName = item.DimensionName
			payload.Items[index].DimensionWeight = item.DimensionWeight
			payload.Items[index].ItemWeight = item.ItemWeight
			payload.Items[index].CompositeWeight = item.CompositeWeight
			payload.Items[index].ScorerType = item.ScorerType
			payload.Items[index].ScorerName = item.ScorerName
			payload.Items[index].WeightedContribution = item.WeightedContribution
			payload.Items[index].HasWeightedFields = item.HasWeightedFields
			payload.Items[index].FinalScore = item.FinalScore
		}
	}
	if payload.Grade == "" {
		payload.Grade = canonical.Grade
	}
	return payload
}

func GetOrCreateResultSnapshot(db *gorm.DB, evaluationID, createdBy uint) (models.EvaluationResultSnapshot, resultSnapshotPayload, error) {
	evaluation, err := loadEvaluationForSnapshot(db, evaluationID)
	if err != nil {
		return models.EvaluationResultSnapshot{}, resultSnapshotPayload{}, err
	}
	var payload resultSnapshotPayload
	if isNewWorkflowEvaluation(evaluation) {
		payload, _, _, err = buildCanonicalResultSnapshotWithDB(db, evaluation)
	} else {
		payload, _, _, err = buildResultSnapshot(evaluation)
	}
	if err != nil {
		return models.EvaluationResultSnapshot{}, payload, err
	}
	var latest models.EvaluationResultSnapshot
	if err := db.Where("evaluation_id = ?", evaluationID).Order("version DESC").First(&latest).Error; err == nil {
		var previous resultSnapshotPayload
		if json.Unmarshal([]byte(latest.SnapshotJSON), &previous) == nil {
			var rawFields map[string]json.RawMessage
			_ = json.Unmarshal([]byte(latest.SnapshotJSON), &rawFields)
			previous.ExportLayout = normalizeExportLayout(previous.ExportLayout)
			if evaluation.Status == "completed" || rawFields["export_layout"] == nil {
				previous = enrichStoredCanonicalPayload(db, evaluation, previous)
				return latest, previous, nil
			}
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.EvaluationResultSnapshot{}, payload, err
	}
	rawBytes, err := json.Marshal(payload)
	if err != nil {
		return models.EvaluationResultSnapshot{}, payload, err
	}
	raw := string(rawBytes)
	sum := sha256.Sum256(rawBytes)
	checksum := hex.EncodeToString(sum[:])
	var existing models.EvaluationResultSnapshot
	if err := db.Where("evaluation_id = ? AND checksum = ?", evaluationID, checksum).First(&existing).Error; err == nil {
		var stored resultSnapshotPayload
		if json.Unmarshal([]byte(existing.SnapshotJSON), &stored) == nil {
			stored.ExportLayout = normalizeExportLayout(stored.ExportLayout)
			return existing, stored, nil
		}
		return existing, payload, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.EvaluationResultSnapshot{}, payload, err
	}
	var maxVersion int
	if err := db.Model(&models.EvaluationResultSnapshot{}).Where("evaluation_id = ?", evaluationID).Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error; err != nil {
		return models.EvaluationResultSnapshot{}, payload, err
	}
	snapshot := models.EvaluationResultSnapshot{
		EvaluationID: evaluationID, Version: maxVersion + 1, SnapshotJSON: raw,
		Checksum: checksum, CreatedBy: createdBy,
	}
	if err := db.Create(&snapshot).Error; err != nil {
		return models.EvaluationResultSnapshot{}, payload, err
	}
	return snapshot, payload, nil
}

func finalizeEvaluation(tx *gorm.DB, evaluation *models.KPIEvaluation) error {
	update := tx.Model(&models.KPIEvaluation{}).
		Where("id = ? AND status = ? AND has_objection = ?", evaluation.ID, evaluation.Status, false).
		Update("status", workflowStateCompleted)
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return errWorkflowStateConflict
	}
	// The confirmation snapshot is the authoritative final evidence. Lock every
	// materialized scoring task in the same transaction so a direct task API
	// call cannot change the result after confirmation.
	now := time.Now()
	if err := tx.Model(&models.EvaluationScoringTask{}).
		Where("evaluation_id = ? AND locked_at IS NULL", evaluation.ID).
		Updates(map[string]any{"locked_at": now, "version": gorm.Expr("version + 1")}).Error; err != nil {
		return err
	}
	if isNewWorkflowEvaluation(*evaluation) {
		return nil
	}
	var scores []models.KPIScore
	if err := tx.Where("evaluation_id = ?", evaluation.ID).Find(&scores).Error; err != nil {
		return err
	}
	for _, score := range scores {
		final := effectiveFinalScore(score)
		if err := tx.Model(&score).Update("final_score", final).Error; err != nil {
			return err
		}
	}
	return nil
}

func loadConfirmation(db *gorm.DB, evaluationID uint) (models.EvaluationConfirmation, error) {
	var confirmation models.EvaluationConfirmation
	err := db.Preload("Snapshot").Preload("Confirmer").Preload("Handler").Where("evaluation_id = ?", evaluationID).First(&confirmation).Error
	return confirmation, err
}

func GetEvaluationConfirmation(c *gin.Context) {
	evaluationID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_EVALUATION_ID", "无效的评估ID", nil)
		return
	}
	if !CanAccessEvaluation(c.GetUint("user_id"), uint(evaluationID)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "超出数据范围"})
		return
	}
	confirmation, err := loadConfirmation(models.DB, uint(evaluationID))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusOK, gin.H{"data": nil})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取确认记录失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": confirmation})
}

func ConfirmEvaluationOnline(c *gin.Context) {
	evaluationID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_EVALUATION_ID", "无效的评估ID", nil)
		return
	}
	userID := c.GetUint("user_id")
	var evaluation models.KPIEvaluation
	if err := models.DB.First(&evaluation, evaluationID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "评估不存在"})
		return
	}
	if evaluation.EmployeeID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "只能确认本人的最终得分"})
		return
	}
	if guard := guardEvaluationConfirmation(models.DB, evaluation); guard != nil {
		respondError(c, guard.Status, guard.Code, guard.Message, guard.Details)
		return
	}
	tx := models.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "确认失败"})
		return
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()
	if isNewWorkflowEvaluation(evaluation) {
		if _, _, err := persistCanonicalEvaluationResult(tx, evaluation.ID, userID); err != nil {
			tx.Rollback()
			respondError(c, http.StatusUnprocessableEntity, "RESULT_CALCULATION_FAILED", err.Error(), nil)
			return
		}
	}
	var count int64
	if err := tx.Model(&models.EvaluationConfirmation{}).Where("evaluation_id = ?", evaluation.ID).Count(&count).Error; err != nil || count > 0 {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"error": "该考核已经确认"})
		return
	}
	snapshot, _, err := GetOrCreateResultSnapshot(tx, evaluation.ID, userID)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成确认快照失败"})
		return
	}
	now := time.Now()
	confirmation := models.EvaluationConfirmation{
		EvaluationID: evaluation.ID, SnapshotID: snapshot.ID, Method: "online",
		ConfirmedBy: userID, ConfirmedAt: now,
	}
	if err := tx.Create(&confirmation).Error; err != nil || finalizeEvaluation(tx, &evaluation) != nil || tx.Commit().Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "确认最终得分失败"})
		return
	}
	RecordAudit(c, "confirm_online", "evaluation", strconv.FormatUint(evaluationID, 10), "SUCCESS")
	confirmation, _ = loadConfirmation(models.DB, evaluation.ID)
	models.DB.Preload("Employee.Department").Preload("Template").First(&evaluation, evaluation.ID)
	GetNotificationService().SendNotification(userID, EventEvaluationStatusChange, &evaluation)
	c.JSON(http.StatusOK, gin.H{"message": "最终得分确认成功", "data": confirmation, "evaluation": evaluation})
}

func signoffUploadDir() string {
	if value := strings.TrimSpace(os.Getenv("SIGNOFF_UPLOAD_DIR")); value != "" {
		return value
	}
	return "./uploads/evaluation-confirmations"
}

func acceptedSignoffType(name, detected string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(name))
	switch detected {
	case "application/pdf":
		return ".pdf", ext == ".pdf"
	case "image/jpeg":
		return ".jpg", ext == ".jpg" || ext == ".jpeg"
	case "image/png":
		return ".png", ext == ".png"
	default:
		return "", false
	}
}

func ConfirmEvaluationPaper(c *gin.Context) {
	evaluationID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的评估ID"})
		return
	}
	userID := c.GetUint("user_id")
	var evaluation models.KPIEvaluation
	if err := ApplyEvaluationScope(models.DB, userID).First(&evaluation, evaluationID).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "评估不存在或超出数据范围"})
		return
	}
	if guard := guardEvaluationConfirmation(models.DB, evaluation); guard != nil {
		respondError(c, guard.Status, guard.Code, guard.Message, guard.Details)
		return
	}
	signedAt, err := time.Parse("2006-01-02", c.PostForm("signed_at"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "签字日期格式应为 YYYY-MM-DD"})
		return
	}
	remark := strings.TrimSpace(c.PostForm("remark"))
	if len([]rune(remark)) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "备注不能超过500字"})
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil || fileHeader.Size <= 0 || fileHeader.Size > maxSignoffFileSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请上传不超过10MB的签字材料"})
		return
	}
	source, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法读取签字材料"})
		return
	}
	defer source.Close()
	header := make([]byte, 512)
	n, readErr := io.ReadFull(source, header)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法校验签字材料"})
		return
	}
	detected := http.DetectContentType(header[:n])
	ext, ok := acceptedSignoffType(fileHeader.Filename, detected)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "仅支持 PDF、JPG、JPEG、PNG 文件"})
		return
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法读取签字材料"})
		return
	}
	tx := models.DB.Begin()
	if tx.Error != nil {
		respondError(c, http.StatusInternalServerError, "CONFIRMATION_WRITE_FAILED", "登记纸质签字失败", nil)
		return
	}
	defer tx.Rollback()
	if isNewWorkflowEvaluation(evaluation) {
		if _, _, err := persistCanonicalEvaluationResult(tx, evaluation.ID, userID); err != nil {
			respondError(c, http.StatusUnprocessableEntity, "RESULT_CALCULATION_FAILED", err.Error(), nil)
			return
		}
	}
	var latest models.EvaluationResultSnapshot
	if err := tx.Where("evaluation_id = ?", evaluation.ID).Order("version DESC").First(&latest).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先导出当前最终评分表再登记纸质签字"})
		return
	}
	currentEvaluation, err := loadEvaluationForSnapshot(tx, evaluation.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取当前结果失败"})
		return
	}
	var checksum string
	if isNewWorkflowEvaluation(currentEvaluation) {
		_, _, checksum, err = buildCanonicalResultSnapshotWithDB(tx, currentEvaluation)
	} else {
		_, _, checksum, err = buildResultSnapshot(currentEvaluation)
	}
	if err != nil || latest.Checksum != checksum {
		c.JSON(http.StatusBadRequest, gin.H{"error": "最终结果已变化，请重新导出后再登记签字"})
		return
	}
	uploadDir := signoffUploadDir()
	if err := os.MkdirAll(uploadDir, 0750); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建签字材料目录失败"})
		return
	}
	storedName := uuid.New().String() + ext
	storedPath := filepath.Join(uploadDir, storedName)
	destination, err := os.OpenFile(storedPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存签字材料失败"})
		return
	}
	_, copyErr := io.Copy(destination, io.LimitReader(source, maxSignoffFileSize+1))
	closeErr := destination.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(storedPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存签字材料失败"})
		return
	}
	var count int64
	if err := tx.Model(&models.EvaluationConfirmation{}).Where("evaluation_id = ?", evaluation.ID).Count(&count).Error; err != nil || count > 0 {
		tx.Rollback()
		_ = os.Remove(storedPath)
		c.JSON(http.StatusConflict, gin.H{"error": "该考核已经确认"})
		return
	}
	now := time.Now()
	handledBy := userID
	confirmation := models.EvaluationConfirmation{
		EvaluationID: evaluation.ID, SnapshotID: latest.ID, Method: "paper",
		ConfirmedBy: evaluation.EmployeeID, ConfirmedAt: now, SignedAt: &signedAt, HandledBy: &handledBy,
		AttachmentStoredName: storedName, AttachmentOriginalName: filepath.Base(fileHeader.Filename),
		AttachmentContentType: detected, AttachmentSize: fileHeader.Size, Remark: remark,
	}
	if err := tx.Create(&confirmation).Error; err != nil || finalizeEvaluation(tx, &evaluation) != nil || tx.Commit().Error != nil {
		tx.Rollback()
		_ = os.Remove(storedPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "登记纸质签字失败"})
		return
	}
	RecordAudit(c, "confirm_paper", "evaluation", strconv.FormatUint(evaluationID, 10), "SUCCESS")
	confirmation, _ = loadConfirmation(models.DB, evaluation.ID)
	models.DB.Preload("Employee.Department").Preload("Template").First(&evaluation, evaluation.ID)
	GetNotificationService().SendNotification(userID, EventEvaluationStatusChange, &evaluation)
	c.JSON(http.StatusOK, gin.H{"message": "纸质签字登记成功", "data": confirmation, "evaluation": evaluation})
}

func DownloadConfirmationAttachment(c *gin.Context) {
	evaluationID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的评估ID"})
		return
	}
	if !CanAccessEvaluation(c.GetUint("user_id"), uint(evaluationID)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "超出数据范围"})
		return
	}
	confirmation, err := loadConfirmation(models.DB, uint(evaluationID))
	if err != nil || confirmation.Method != "paper" || confirmation.AttachmentStoredName == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "签字材料不存在"})
		return
	}
	path := filepath.Join(signoffUploadDir(), filepath.Base(confirmation.AttachmentStoredName))
	if _, err := os.Stat(path); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "签字材料不存在"})
		return
	}
	c.Header("Content-Type", confirmation.AttachmentContentType)
	c.FileAttachment(path, confirmation.AttachmentOriginalName)
}

// UploadConfirmationSupplement archives a paper-signature copy after an
// evaluation has already been finalized. The immutable EvaluationConfirmation
// row is never updated and the evaluation remains completed.
func UploadConfirmationSupplement(c *gin.Context) {
	evaluationID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_EVALUATION_ID", "无效的评估ID", nil)
		return
	}
	userID := c.GetUint("user_id")
	var evaluation models.KPIEvaluation
	if err := ApplyEvaluationScope(models.DB, userID).First(&evaluation, evaluationID).Error; err != nil {
		respondError(c, http.StatusForbidden, "ASSESSMENT_SCOPE_FORBIDDEN", "评估不存在或超出数据范围", nil)
		return
	}
	if evaluation.Status != workflowStateCompleted {
		respondError(c, http.StatusConflict, "SIGNOFF_SUPPLEMENT_STATE_CONFLICT", "仅已完成的考核可以补充纸质签字材料", map[string]any{"current_state": evaluation.Status})
		return
	}
	var confirmation models.EvaluationConfirmation
	if err := models.DB.Where("evaluation_id = ?", evaluation.ID).First(&confirmation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusConflict, "SIGNOFF_CONFIRMATION_REQUIRED", "该考核缺少最终确认记录，无法补充签字材料", nil)
			return
		}
		respondError(c, http.StatusInternalServerError, "SIGNOFF_CONFIRMATION_READ_FAILED", "读取最终确认记录失败", nil)
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil || fileHeader.Size <= 0 || fileHeader.Size > maxSignoffFileSize {
		respondError(c, http.StatusBadRequest, "SIGNOFF_FILE_INVALID", "请上传不超过10MB的签字材料", nil)
		return
	}
	source, err := fileHeader.Open()
	if err != nil {
		respondError(c, http.StatusBadRequest, "SIGNOFF_FILE_READ_FAILED", "无法读取签字材料", nil)
		return
	}
	defer source.Close()
	header := make([]byte, 512)
	n, readErr := io.ReadFull(source, header)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		respondError(c, http.StatusBadRequest, "SIGNOFF_FILE_READ_FAILED", "无法校验签字材料", nil)
		return
	}
	detected := http.DetectContentType(header[:n])
	ext, ok := acceptedSignoffType(fileHeader.Filename, detected)
	if !ok {
		respondError(c, http.StatusBadRequest, "SIGNOFF_FILE_TYPE_UNSUPPORTED", "仅支持 PDF、JPG、JPEG、PNG 文件", nil)
		return
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		respondError(c, http.StatusBadRequest, "SIGNOFF_FILE_READ_FAILED", "无法读取签字材料", nil)
		return
	}
	remark := strings.TrimSpace(c.PostForm("remark"))
	if len([]rune(remark)) > 500 {
		respondError(c, http.StatusBadRequest, "SIGNOFF_REMARK_TOO_LONG", "备注不能超过500字", nil)
		return
	}
	var signedAt *time.Time
	if raw := strings.TrimSpace(c.PostForm("signed_at")); raw != "" {
		parsed, parseErr := time.Parse("2006-01-02", raw)
		if parseErr != nil {
			respondError(c, http.StatusBadRequest, "SIGNOFF_SIGNED_AT_INVALID", "签字日期格式应为 YYYY-MM-DD", nil)
			return
		}
		signedAt = &parsed
	}
	if err := os.MkdirAll(signoffUploadDir(), 0750); err != nil {
		respondError(c, http.StatusInternalServerError, "SIGNOFF_STORAGE_FAILED", "创建签字材料目录失败", nil)
		return
	}
	storedName := uuid.New().String() + ext
	storedPath := filepath.Join(signoffUploadDir(), storedName)
	destination, err := os.OpenFile(storedPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "SIGNOFF_STORAGE_FAILED", "保存签字材料失败", nil)
		return
	}
	written, copyErr := io.Copy(destination, io.LimitReader(source, maxSignoffFileSize+1))
	closeErr := destination.Close()
	if copyErr != nil || closeErr != nil || written <= 0 || written > maxSignoffFileSize {
		_ = os.Remove(storedPath)
		respondError(c, http.StatusBadRequest, "SIGNOFF_FILE_INVALID", "签字材料大小超过10MB或保存失败", nil)
		return
	}
	supplement := models.EvaluationSignoffSupplement{
		EvaluationID: evaluation.ID, UploadedBy: userID, UploadedAt: time.Now(), SignedAt: signedAt,
		AttachmentStoredName: storedName, AttachmentOriginalName: filepath.Base(fileHeader.Filename),
		AttachmentContentType: detected, AttachmentSize: written, Remark: remark,
	}
	if err := models.DB.Create(&supplement).Error; err != nil {
		_ = os.Remove(storedPath)
		respondError(c, http.StatusInternalServerError, "SIGNOFF_SUPPLEMENT_WRITE_FAILED", "保存补充签字记录失败", nil)
		return
	}
	signedAtAudit := ""
	if signedAt != nil {
		signedAtAudit = signedAt.Format("2006-01-02")
	}
	auditOriginalName := strings.NewReplacer(";", "_", "\r", "_", "\n", "_").Replace(supplement.AttachmentOriginalName)
	RecordAuditDetails(c, "upload_signoff_supplement", "evaluation", strconv.FormatUint(uint64(evaluation.ID), 10), "SUCCESS", fmt.Sprintf("supplement_id=%d;stored_name=%s;original_name=%s;content_type=%s;size=%d;signed_at=%s;remark_recorded=%t;remark_length=%d", supplement.ID, storedName, auditOriginalName, detected, written, signedAtAudit, remark != "", len([]rune(remark))))
	c.JSON(http.StatusOK, gin.H{"message": "补充签字材料上传成功", "data": supplement, "evaluation": gin.H{"id": evaluation.ID, "status": evaluation.Status}, "request_id": requestID(c)})
}

func ListConfirmationSupplements(c *gin.Context) {
	evaluationID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_EVALUATION_ID", "无效的评估ID", nil)
		return
	}
	userID := c.GetUint("user_id")
	if !CanAccessEvaluation(userID, uint(evaluationID)) {
		respondError(c, http.StatusForbidden, "ASSESSMENT_SCOPE_FORBIDDEN", "超出数据范围", nil)
		return
	}
	var rows []models.EvaluationSignoffSupplement
	if err := models.DB.Preload("Uploader").Where("evaluation_id = ?", evaluationID).Order("uploaded_at DESC").Find(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "SIGNOFF_SUPPLEMENT_READ_FAILED", "获取补充签字材料失败", nil)
		return
	}
	respondData(c, http.StatusOK, rows, "补充签字材料读取成功")
}

func DownloadConfirmationSupplement(c *gin.Context) {
	evaluationID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_EVALUATION_ID", "无效的评估ID", nil)
		return
	}
	supplementID, err := strconv.ParseUint(c.Param("supplement_id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_SIGNOFF_SUPPLEMENT_ID", "无效的补充材料ID", nil)
		return
	}
	if !CanAccessEvaluation(c.GetUint("user_id"), uint(evaluationID)) {
		respondError(c, http.StatusForbidden, "ASSESSMENT_SCOPE_FORBIDDEN", "超出数据范围", nil)
		return
	}
	var supplement models.EvaluationSignoffSupplement
	if err := models.DB.Where("id = ? AND evaluation_id = ?", supplementID, evaluationID).First(&supplement).Error; err != nil {
		respondError(c, http.StatusNotFound, "SIGNOFF_SUPPLEMENT_NOT_FOUND", "补充签字材料不存在", nil)
		return
	}
	path := filepath.Join(signoffUploadDir(), filepath.Base(supplement.AttachmentStoredName))
	if _, err := os.Stat(path); err != nil {
		respondError(c, http.StatusNotFound, "SIGNOFF_ATTACHMENT_NOT_FOUND", "补充签字材料不存在", nil)
		return
	}
	c.Header("X-Request-ID", requestID(c))
	c.Header("Content-Type", supplement.AttachmentContentType)
	c.FileAttachment(path, supplement.AttachmentOriginalName)
}

func ensureEvaluationNotCompleted(evaluationID uint) error {
	var status string
	if err := models.DB.Model(&models.KPIEvaluation{}).Where("id = ?", evaluationID).Pluck("status", &status).Error; err != nil {
		return err
	}
	if status == "completed" {
		return fmt.Errorf("已完成的考核不能修改")
	}
	return nil
}
