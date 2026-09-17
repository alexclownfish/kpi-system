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
	ItemID       uint    `json:"item_id"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	MaxScore     float64 `json:"max_score"`
	FinalScore   float64 `json:"final_score"`
	FinalComment string  `json:"final_comment"`
}

type resultSnapshotPayload struct {
	EvaluationID   uint                 `json:"evaluation_id"`
	EmployeeID     uint                 `json:"employee_id"`
	EmployeeName   string               `json:"employee_name"`
	EmployeeEmail  string               `json:"employee_email"`
	Position       string               `json:"position"`
	Department     string               `json:"department"`
	TemplateID     uint                 `json:"template_id"`
	TemplateName   string               `json:"template_name"`
	Period         string               `json:"period"`
	Year           int                  `json:"year"`
	Month          *int                 `json:"month,omitempty"`
	Quarter        *int                 `json:"quarter,omitempty"`
	TotalScore     float64              `json:"total_score"`
	FinalComment   string               `json:"final_comment"`
	Objection      string               `json:"objection_reason"`
	HasObjection   bool                 `json:"has_objection"`
	Items          []resultSnapshotItem `json:"items"`
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
			MaxScore: score.Item.MaxScore, FinalScore: effectiveFinalScore(score), FinalComment: comment,
		})
	}
	payload := resultSnapshotPayload{
		EvaluationID: evaluation.ID, EmployeeID: evaluation.EmployeeID, EmployeeName: evaluation.Employee.Name,
		EmployeeEmail: evaluation.Employee.Email, Position: evaluation.Employee.Position,
		Department: evaluation.Employee.Department.Name, TemplateID: evaluation.TemplateID,
		TemplateName: evaluation.Template.Name, Period: evaluation.Period, Year: evaluation.Year,
		Month: evaluation.Month, Quarter: evaluation.Quarter, TotalScore: evaluation.TotalScore,
		FinalComment: evaluation.FinalComment, Objection: evaluation.ObjectionReason,
		HasObjection: evaluation.HasObjection, Items: items,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return payload, "", "", err
	}
	sum := sha256.Sum256(raw)
	return payload, string(raw), hex.EncodeToString(sum[:]), nil
}

func GetOrCreateResultSnapshot(db *gorm.DB, evaluationID, createdBy uint) (models.EvaluationResultSnapshot, resultSnapshotPayload, error) {
	evaluation, err := loadEvaluationForSnapshot(db, evaluationID)
	if err != nil {
		return models.EvaluationResultSnapshot{}, resultSnapshotPayload{}, err
	}
	payload, raw, checksum, err := buildResultSnapshot(evaluation)
	if err != nil {
		return models.EvaluationResultSnapshot{}, payload, err
	}
	var existing models.EvaluationResultSnapshot
	if err := db.Where("evaluation_id = ? AND checksum = ?", evaluationID, checksum).First(&existing).Error; err == nil {
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
	return tx.Model(evaluation).Update("status", "completed").Error
}

func loadConfirmation(db *gorm.DB, evaluationID uint) (models.EvaluationConfirmation, error) {
	var confirmation models.EvaluationConfirmation
	err := db.Preload("Snapshot").Preload("Confirmer").Preload("Handler").Where("evaluation_id = ?", evaluationID).First(&confirmation).Error
	return confirmation, err
}

func GetEvaluationConfirmation(c *gin.Context) {
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的评估ID"})
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
	if evaluation.Status != "pending_confirm" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "当前状态不能确认最终得分"})
		return
	}
	if evaluation.HasObjection {
		c.JSON(http.StatusBadRequest, gin.H{"error": "异议尚未处理，暂不能确认"})
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
	if evaluation.Status != "pending_confirm" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "只有待确认考核可以登记纸质签字"})
		return
	}
	if evaluation.HasObjection {
		c.JSON(http.StatusBadRequest, gin.H{"error": "异议尚未处理，暂不能确认"})
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
	var latest models.EvaluationResultSnapshot
	if err := models.DB.Where("evaluation_id = ?", evaluation.ID).Order("version DESC").First(&latest).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先导出当前最终评分表再登记纸质签字"})
		return
	}
	currentEvaluation, err := loadEvaluationForSnapshot(models.DB, evaluation.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取当前结果失败"})
		return
	}
	_, _, checksum, err := buildResultSnapshot(currentEvaluation)
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
	tx := models.DB.Begin()
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
