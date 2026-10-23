package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"dootask-kpi-server/global"
	"dootask-kpi-server/models"
	"dootask-kpi-server/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

const maxFinalScoreImportSize int64 = 10 << 20

type resultFilter struct {
	DepartmentID uint
	Year         int
	Period       string
	Month        int
	Quarter      int
	Status       string
}

func parseResultFilter(c *gin.Context) (resultFilter, error) {
	f := resultFilter{Period: strings.TrimSpace(c.Query("period")), Status: strings.TrimSpace(c.Query("status"))}
	var err error
	if value := c.Query("department_id"); value != "" {
		id, parseErr := strconv.ParseUint(value, 10, 32)
		if parseErr != nil {
			return f, errors.New("部门参数无效")
		}
		f.DepartmentID = uint(id)
	}
	if value := c.Query("year"); value != "" {
		f.Year, err = strconv.Atoi(value)
		if err != nil || f.Year < 2000 || f.Year > 2100 {
			return f, errors.New("年度参数无效")
		}
	}
	if value := c.Query("month"); value != "" {
		f.Month, err = strconv.Atoi(value)
		if err != nil || f.Month < 1 || f.Month > 12 {
			return f, errors.New("月份参数无效")
		}
	}
	if value := c.Query("quarter"); value != "" {
		f.Quarter, err = strconv.Atoi(value)
		if err != nil || f.Quarter < 1 || f.Quarter > 4 {
			return f, errors.New("季度参数无效")
		}
	}
	if f.Period != "" && f.Period != "monthly" && f.Period != "quarterly" && f.Period != "yearly" {
		return f, errors.New("周期类型无效")
	}
	return f, nil
}

func applyResultFilter(query *gorm.DB, f resultFilter) *gorm.DB {
	if f.DepartmentID > 0 {
		query = query.Where("kpi_evaluations.employee_id IN (SELECT id FROM employees WHERE department_id = ?)", f.DepartmentID)
	}
	if f.Year > 0 {
		query = query.Where("kpi_evaluations.year = ?", f.Year)
	}
	if f.Period != "" {
		if f.Period == "yearly" {
			query = query.Where("(kpi_evaluations.period = ? OR kpi_evaluations.period = ?)", "yearly", strconv.Itoa(f.Year))
		} else {
			query = query.Where("kpi_evaluations.period = ?", f.Period)
		}
	}
	if f.Period == "monthly" && f.Month > 0 {
		query = query.Where("kpi_evaluations.month = ?", f.Month)
	}
	if f.Period == "quarterly" && f.Quarter > 0 {
		query = query.Where("kpi_evaluations.quarter = ?", f.Quarter)
	}
	if f.Status != "" {
		query = query.Where("kpi_evaluations.status = ?", f.Status)
	}
	return query
}

func loadFilteredEvaluations(c *gin.Context, statuses []string) ([]models.KPIEvaluation, resultFilter, error) {
	f, err := parseResultFilter(c)
	if err != nil {
		return nil, f, err
	}
	query := ApplyEvaluationScope(models.DB.Preload("Employee.Department").Preload("Template").Preload("Scores.Item"), c.GetUint("user_id"))
	query = applyResultFilter(query, f)
	if len(statuses) > 0 {
		query = query.Where("kpi_evaluations.status IN ?", statuses)
	}
	var evaluations []models.KPIEvaluation
	err = query.Order("kpi_evaluations.id").Find(&evaluations).Error
	return evaluations, f, err
}

func publishExport(c *gin.Context, filePath, fileName, message string, results ...ExportResultVersion) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	randomKey := "export_" + uuid.NewString()
	global.Cache.Set(randomKey, fileName, 5*time.Minute)
	url := utils.GetFileURL(c.GetString("base_url"), fmt.Sprintf("/api/download/exports/%s", randomKey))
	c.JSON(http.StatusOK, ExportResponse{FileURL: url, FileName: fileName, FileSize: info.Size(), Message: message, RequestID: requestID(c), Results: results})
	go func() {
		time.Sleep(30 * time.Minute)
		_ = os.Remove(filePath)
	}()
	return nil
}

func ExportFinalScoreImportTemplate(c *gin.Context) {
	evaluations, _, err := loadFilteredEvaluations(c, []string{"pending_confirm"})
	if err != nil {
		respondExportError(c, http.StatusBadRequest, "INVALID_EXPORT_FILTER", err.Error(), false, "请调整筛选条件后重试", nil)
		return
	}
	if len(evaluations) == 0 {
		respondExportError(c, http.StatusNotFound, "NO_PENDING_FINAL_SCORES", "没有可导入最终评分的待确认考核", false, "请调整部门、周期或状态筛选条件", nil)
		return
	}
	f := excelize.NewFile()
	defer f.Close()
	sheet := "最终评分导入"
	f.SetSheetName("Sheet1", sheet)
	headers := []string{"考核ID", "评分项ID", "员工", "部门", "周期", "指标", "满分", "当前最终分", "导入最终分", "最终评价"}
	for index, value := range headers {
		cell, _ := excelize.CoordinatesToCellName(index+1, 1)
		f.SetCellValue(sheet, cell, value)
	}
	row := 2
	for _, evaluation := range evaluations {
		sort.SliceStable(evaluation.Scores, func(i, j int) bool { return evaluation.Scores[i].Item.Order < evaluation.Scores[j].Item.Order })
		for _, score := range evaluation.Scores {
			values := []any{evaluation.ID, score.ItemID, evaluation.Employee.Name, evaluation.Employee.Department.Name,
				formatPeriodDisplay(evaluation.Period, evaluation.Year, evaluation.Month, evaluation.Quarter), score.Item.Name,
				score.Item.MaxScore, effectiveFinalScore(score), "", score.FinalComment}
			for col, value := range values {
				cell, _ := excelize.CoordinatesToCellName(col+1, row)
				f.SetCellValue(sheet, cell, value)
			}
			row++
		}
	}
	headerStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1}, Alignment: &excelize.Alignment{Horizontal: "center"}})
	f.SetCellStyle(sheet, "A1", "J1", headerStyle)
	f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1})
	widths := []float64{12, 12, 14, 16, 20, 24, 10, 14, 14, 30}
	for i, width := range widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth(sheet, col, col, width)
	}
	if err := os.MkdirAll(ExportDir, 0755); err != nil {
		respondExportError(c, http.StatusInternalServerError, "EXPORT_DIRECTORY_FAILED", "创建导出目录失败", true, "请稍后重试；若仍失败，请检查服务存储空间并提供 request_id", nil)
		return
	}
	fileName := fmt.Sprintf("最终评分导入模板-%s.xlsx", time.Now().Format("20060102-150405"))
	filePath := filepath.Join(ExportDir, fileName)
	if err := f.SaveAs(filePath); err != nil {
		respondExportError(c, http.StatusInternalServerError, "EXPORT_RENDER_FAILED", "生成导入模板失败", true, "请稍后重试；若连续失败，请提供 request_id 给管理员", nil)
		return
	}
	if err := publishExport(c, filePath, fileName, "导入模板生成成功"); err != nil {
		respondExportError(c, http.StatusInternalServerError, "EXPORT_PUBLISH_FAILED", "生成下载链接失败", true, "请稍后重试；若仍失败，请提供 request_id 给管理员", nil)
	}
}

type importRowResult struct {
	Row          int     `json:"row"`
	EvaluationID uint    `json:"evaluation_id,omitempty"`
	ScoreID      uint    `json:"score_id,omitempty"`
	Employee     string  `json:"employee,omitempty"`
	Item         string  `json:"item,omitempty"`
	Score        float64 `json:"score,omitempty"`
	Comment      string  `json:"comment,omitempty"`
	Status       string  `json:"status"`
	Message      string  `json:"message"`
}

type importScoreChange struct {
	ScoreID uint    `json:"score_id"`
	Score   float64 `json:"score"`
	Comment string  `json:"comment"`
}

type importEvaluationChange struct {
	EvaluationID uint                `json:"evaluation_id"`
	Employee     string              `json:"employee"`
	UpdatedAt    time.Time           `json:"updated_at"`
	Valid        bool                `json:"valid"`
	Errors       []string            `json:"errors"`
	Changes      []importScoreChange `json:"changes"`
}

type importBatchPayload struct {
	Evaluations []importEvaluationChange `json:"evaluations"`
	Rows        []importRowResult        `json:"rows"`
}

type importSummary struct {
	TotalRows          int `json:"total_rows"`
	ValidRows          int `json:"valid_rows"`
	InvalidRows        int `json:"invalid_rows"`
	ValidEvaluations   int `json:"valid_evaluations"`
	InvalidEvaluations int `json:"invalid_evaluations"`
}

func parseUintCell(value string) (uint, error) {
	n, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
	return uint(n), err
}

func PreviewFinalScoreImport(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil || fileHeader.Size <= 0 || fileHeader.Size > maxFinalScoreImportSize || strings.ToLower(filepath.Ext(fileHeader.Filename)) != ".xlsx" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请上传不超过10MB的 .xlsx 文件"})
		return
	}
	source, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法读取导入文件"})
		return
	}
	defer source.Close()
	data, err := io.ReadAll(io.LimitReader(source, maxFinalScoreImportSize+1))
	if err != nil || int64(len(data)) > maxFinalScoreImportSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "导入文件读取失败"})
		return
	}
	book, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Excel 文件损坏或格式不正确"})
		return
	}
	defer book.Close()
	if len(book.GetSheetList()) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Excel 中没有工作表"})
		return
	}
	rows, err := book.GetRows(book.GetSheetList()[0])
	if err != nil || len(rows) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Excel 中没有可导入的数据"})
		return
	}
	required := []string{"考核ID", "评分项ID", "员工", "指标", "满分", "导入最终分", "最终评价"}
	columns := map[string]int{}
	for i, value := range rows[0] {
		columns[strings.TrimSpace(value)] = i
	}
	for _, name := range required {
		if _, ok := columns[name]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少必需列：" + name})
			return
		}
	}
	cell := func(row []string, name string) string {
		if index := columns[name]; index < len(row) {
			return strings.TrimSpace(row[index])
		}
		return ""
	}
	userID := c.GetUint("user_id")
	rowResults := make([]importRowResult, 0, len(rows)-1)
	changes := map[uint]*importEvaluationChange{}
	seenItems := map[string]int{}
	for index, row := range rows[1:] {
		rowNumber := index + 2
		if strings.Join(row, "") == "" {
			continue
		}
		result := importRowResult{Row: rowNumber, Employee: cell(row, "员工"), Item: cell(row, "指标"), Comment: cell(row, "最终评价"), Status: "invalid"}
		evaluationID, evalErr := parseUintCell(cell(row, "考核ID"))
		itemID, scoreErr := parseUintCell(cell(row, "评分项ID"))
		scoreValue, valueErr := strconv.ParseFloat(cell(row, "导入最终分"), 64)
		result.EvaluationID, result.ScoreID, result.Score = evaluationID, itemID, scoreValue
		itemKey := fmt.Sprintf("%d:%d", evaluationID, itemID)
		message := ""
		var evaluation models.KPIEvaluation
		var score models.KPIScore
		if evalErr != nil || scoreErr != nil || valueErr != nil {
			message = "考核ID、评分项ID或导入最终分格式错误"
		} else if previous, exists := seenItems[itemKey]; exists {
			message = fmt.Sprintf("评分项重复，已在第%d行出现", previous)
		} else if !CanAccessEvaluation(userID, evaluationID) {
			seenItems[itemKey] = rowNumber
			message = "考核不存在或超出数据范围"
		} else if err := models.DB.First(&evaluation, evaluationID).Error; err != nil {
			seenItems[itemKey] = rowNumber
			message = "考核不存在"
		} else if evaluation.Status != "pending_confirm" {
			seenItems[itemKey] = rowNumber
			message = "仅待确认考核允许导入"
		} else if err := models.DB.Preload("Item").Where("item_id = ? AND evaluation_id = ?", itemID, evaluationID).First(&score).Error; err != nil {
			seenItems[itemKey] = rowNumber
			message = "评分项不属于该考核"
		} else if scoreValue < 0 || scoreValue > score.Item.MaxScore {
			seenItems[itemKey] = rowNumber
			message = fmt.Sprintf("分数必须在 0 到 %.2f 之间", score.Item.MaxScore)
		} else {
			seenItems[itemKey] = rowNumber
			result.Status, result.Message = "valid", "校验通过"
			change := changes[evaluationID]
			if change == nil {
				var employee models.Employee
				models.DB.Select("name").First(&employee, evaluation.EmployeeID)
				change = &importEvaluationChange{EvaluationID: evaluationID, Employee: employee.Name, UpdatedAt: evaluation.UpdatedAt, Valid: true}
				changes[evaluationID] = change
			}
			change.Changes = append(change.Changes, importScoreChange{ScoreID: score.ID, Score: scoreValue, Comment: result.Comment})
			rowResults = append(rowResults, result)
			continue
		}
		result.Message = message
		change := changes[evaluationID]
		if change == nil && evaluationID > 0 {
			change = &importEvaluationChange{EvaluationID: evaluationID, Employee: result.Employee, UpdatedAt: evaluation.UpdatedAt, Valid: false}
			changes[evaluationID] = change
		}
		if change != nil {
			change.Valid = false
			change.Errors = append(change.Errors, fmt.Sprintf("第%d行：%s", rowNumber, message))
		}
		rowResults = append(rowResults, result)
	}
	if len(rowResults) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Excel 中没有可导入的数据"})
		return
	}
	evaluationChanges := make([]importEvaluationChange, 0, len(changes))
	summary := importSummary{TotalRows: len(rowResults)}
	for _, row := range rowResults {
		if row.Status == "valid" {
			summary.ValidRows++
		} else {
			summary.InvalidRows++
		}
	}
	for _, change := range changes {
		if len(change.Errors) > 0 {
			change.Valid = false
		}
		if change.Valid && len(change.Changes) > 0 {
			summary.ValidEvaluations++
		} else {
			summary.InvalidEvaluations++
		}
		evaluationChanges = append(evaluationChanges, *change)
	}
	sort.Slice(evaluationChanges, func(i, j int) bool { return evaluationChanges[i].EvaluationID < evaluationChanges[j].EvaluationID })
	payload := importBatchPayload{Evaluations: evaluationChanges, Rows: rowResults}
	payloadJSON, _ := json.Marshal(payload)
	summaryJSON, _ := json.Marshal(summary)
	batch := models.FinalScoreImportBatch{ID: uuid.NewString(), CreatedBy: userID, FileName: filepath.Base(fileHeader.Filename), Status: "previewed", PayloadJSON: string(payloadJSON), SummaryJSON: string(summaryJSON), ExpiresAt: time.Now().Add(30 * time.Minute)}
	if err := models.DB.Create(&batch).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存预检批次失败"})
		return
	}
	RecordAudit(c, "preview_final_scores", "final_score_import", batch.ID, "SUCCESS")
	c.JSON(http.StatusOK, gin.H{"message": "预检完成", "batch_id": batch.ID, "expires_at": batch.ExpiresAt, "summary": summary, "rows": rowResults, "evaluations": evaluationChanges})
}

type importCommitResult struct {
	EvaluationID uint   `json:"evaluation_id"`
	Employee     string `json:"employee"`
	Status       string `json:"status"`
	Message      string `json:"message"`
}

func CommitFinalScoreImport(c *gin.Context) {
	userID := c.GetUint("user_id")
	var batch models.FinalScoreImportBatch
	if err := models.DB.Where("id = ? AND created_by = ?", c.Param("batchId"), userID).First(&batch).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "预检批次不存在"})
		return
	}
	if batch.Status != "previewed" {
		c.JSON(http.StatusConflict, gin.H{"error": "该预检批次已提交或不可用"})
		return
	}
	if time.Now().After(batch.ExpiresAt) {
		models.DB.Model(&batch).Update("status", "expired")
		c.JSON(http.StatusGone, gin.H{"error": "预检批次已过期，请重新上传"})
		return
	}
	var payload importBatchPayload
	if err := json.Unmarshal([]byte(batch.PayloadJSON), &payload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "预检批次内容损坏"})
		return
	}
	results := make([]importCommitResult, 0, len(payload.Evaluations))
	succeeded, failed := 0, 0
	for _, change := range payload.Evaluations {
		result := importCommitResult{EvaluationID: change.EvaluationID, Employee: change.Employee, Status: "failed"}
		if !change.Valid || len(change.Changes) == 0 {
			result.Message = "预检未通过"
			failed++
			results = append(results, result)
			continue
		}
		if !CanAccessEvaluation(userID, change.EvaluationID) {
			result.Message = "考核已超出数据范围"
			failed++
			results = append(results, result)
			continue
		}
		txErr := models.DB.Transaction(func(tx *gorm.DB) error {
			var evaluation models.KPIEvaluation
			if err := tx.First(&evaluation, change.EvaluationID).Error; err != nil {
				return errors.New("考核不存在")
			}
			if evaluation.Status != "pending_confirm" {
				return errors.New("考核状态已变化")
			}
			if !evaluation.UpdatedAt.Equal(change.UpdatedAt) {
				return errors.New("考核已在预检后被修改")
			}
			for _, item := range change.Changes {
				var score models.KPIScore
				if err := tx.Preload("Item").Where("id = ? AND evaluation_id = ?", item.ScoreID, evaluation.ID).First(&score).Error; err != nil {
					return errors.New("评分项已变化")
				}
				if item.Score < 0 || item.Score > score.Item.MaxScore {
					return errors.New("评分超出范围")
				}
				if err := tx.Model(&score).Updates(map[string]any{"final_score": item.Score, "final_comment": item.Comment}).Error; err != nil {
					return err
				}
			}
			var total float64
			if err := tx.Model(&models.KPIScore{}).Where("evaluation_id = ?", evaluation.ID).Select("COALESCE(SUM(COALESCE(final_score, hr_score, manager_score, self_score, 0)), 0)").Scan(&total).Error; err != nil {
				return err
			}
			return tx.Model(&evaluation).Update("total_score", total).Error
		})
		if txErr != nil {
			result.Message = txErr.Error()
			failed++
		} else {
			result.Status, result.Message = "success", "导入成功"
			succeeded++
		}
		results = append(results, result)
	}
	now := time.Now()
	models.DB.Model(&batch).Updates(map[string]any{"status": "committed", "committed_at": &now})
	resultText := "SUCCESS"
	if failed > 0 {
		resultText = "PARTIAL"
	}
	RecordAudit(c, "commit_final_scores", "final_score_import", batch.ID, resultText)
	c.JSON(http.StatusOK, gin.H{"message": "导入提交完成", "summary": gin.H{"succeeded": succeeded, "failed": failed}, "results": results})
}

var unsafeFileName = regexp.MustCompile(`[^\p{Han}a-zA-Z0-9._-]+`)

func safeFileName(value string) string {
	value = unsafeFileName.ReplaceAllString(strings.TrimSpace(value), "-")
	if value == "" {
		return "未命名"
	}
	return value
}

func createSignoffSummary(evaluations []models.KPIEvaluation, path string) error {
	f := excelize.NewFile()
	defer f.Close()
	sheet := "签字回收汇总"
	f.SetSheetName("Sheet1", sheet)
	headers := []string{"考核ID", "员工", "部门", "周期", "总分", "状态", "确认方式", "确认时间"}
	for i, value := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, value)
	}
	for i, evaluation := range evaluations {
		method, confirmedAt := "未回收", ""
		var confirmation models.EvaluationConfirmation
		if models.DB.Where("evaluation_id = ?", evaluation.ID).First(&confirmation).Error == nil {
			if confirmation.Method == "online" {
				method = "在线确认"
			} else {
				method = "纸质确认"
			}
			confirmedAt = confirmation.ConfirmedAt.Format("2006-01-02 15:04")
		}
		values := []any{evaluation.ID, evaluation.Employee.Name, evaluation.Employee.Department.Name, formatPeriodDisplay(evaluation.Period, evaluation.Year, evaluation.Month, evaluation.Quarter), evaluation.TotalScore, getStatusText(evaluation.Status), method, confirmedAt}
		for col, value := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, i+2)
			f.SetCellValue(sheet, cell, value)
		}
	}
	f.SetColWidth(sheet, "A", "H", 18)
	return f.SaveAs(path)
}

func ExportSignoffBatch(c *gin.Context) {
	evaluations, _, err := loadFilteredEvaluations(c, []string{"pending_confirm", "awaiting_employee_confirmation", "completed"})
	if err != nil {
		respondExportError(c, http.StatusBadRequest, "INVALID_EXPORT_FILTER", err.Error(), false, "请调整筛选条件后重试", nil)
		return
	}
	if len(evaluations) == 0 {
		respondExportError(c, http.StatusNotFound, "NO_FINALIZED_EVALUATIONS", "没有符合条件的已定稿考核", false, "请调整部门、周期或状态筛选条件", nil)
		return
	}
	tempDir, err := os.MkdirTemp("", "kpi-signoff-batch-")
	if err != nil {
		respondExportError(c, http.StatusInternalServerError, "EXPORT_TEMP_DIRECTORY_FAILED", "创建临时目录失败", true, "请稍后重试；若仍失败，请提供 request_id 给管理员", nil)
		return
	}
	defer os.RemoveAll(tempDir)
	pdfPaths := make([]string, 0, len(evaluations))
	results := make([]ExportResultVersion, 0, len(evaluations))
	for _, evaluation := range evaluations {
		snapshot, payload, snapshotErr := GetOrCreateResultSnapshot(models.DB, evaluation.ID, c.GetUint("user_id"))
		if snapshotErr != nil {
			respondExportError(c, http.StatusInternalServerError, "EXPORT_RESULT_SNAPSHOT_FAILED", fmt.Sprintf("生成 %s 的结果版本失败", evaluation.Employee.Name), true, "请稍后重试；若仍失败，请提供 request_id 给管理员", nil)
			return
		}
		results = append(results, ExportResultVersion{EvaluationID: evaluation.ID, ResultVersion: snapshot.Version, Checksum: snapshot.Checksum})
		base := fmt.Sprintf("%06d-%s-%s", evaluation.ID, safeFileName(payload.EmployeeName), safeFileName(formatPeriodDisplay(payload.Period, payload.Year, payload.Month, payload.Quarter)))
		pdfPath := filepath.Join(tempDir, base+".pdf")
		pdfErr := renderResultPDFFile(buildExportDocument(snapshot, payload), pdfPath)
		if pdfErr != nil {
			respondExportError(c, http.StatusInternalServerError, "EXPORT_RENDER_FAILED", fmt.Sprintf("生成 %s 的签字表失败", evaluation.Employee.Name), true, "请稍后重试；若连续失败，请提供 request_id 和结果版本给管理员", &snapshot.Version)
			return
		}
		pdfPaths = append(pdfPaths, pdfPath)
	}
	summaryPath := filepath.Join(tempDir, "签字回收汇总.xlsx")
	if err := createSignoffSummary(evaluations, summaryPath); err != nil {
		respondExportError(c, http.StatusInternalServerError, "EXPORT_SUMMARY_FAILED", "生成汇总表失败", true, "请稍后重试；若仍失败，请提供 request_id 给管理员", nil)
		return
	}
	if err := os.MkdirAll(ExportDir, 0755); err != nil {
		respondExportError(c, http.StatusInternalServerError, "EXPORT_DIRECTORY_FAILED", "创建导出目录失败", true, "请稍后重试；若仍失败，请检查服务存储空间并提供 request_id", nil)
		return
	}
	fileName := fmt.Sprintf("绩效签字表批量导出-%s.zip", time.Now().Format("20060102-150405"))
	zipPath := filepath.Join(ExportDir, fileName)
	archive, err := os.Create(zipPath)
	if err != nil {
		respondExportError(c, http.StatusInternalServerError, "EXPORT_ARCHIVE_FAILED", "创建ZIP失败", true, "请稍后重试；若仍失败，请提供 request_id 给管理员", nil)
		return
	}
	writer := zip.NewWriter(archive)
	paths := append(pdfPaths, summaryPath)
	for _, path := range paths {
		entry, createErr := writer.Create(filepath.Base(path))
		if createErr != nil {
			writer.Close()
			archive.Close()
			os.Remove(zipPath)
			respondExportError(c, http.StatusInternalServerError, "EXPORT_ARCHIVE_FAILED", "打包ZIP失败", true, "请稍后重试；若仍失败，请提供 request_id 给管理员", nil)
			return
		}
		source, openErr := os.Open(path)
		if openErr != nil {
			writer.Close()
			archive.Close()
			os.Remove(zipPath)
			respondExportError(c, http.StatusInternalServerError, "EXPORT_FILE_READ_FAILED", "读取导出文件失败", true, "请稍后重试；若仍失败，请提供 request_id 给管理员", nil)
			return
		}
		_, copyErr := io.Copy(entry, source)
		source.Close()
		if copyErr != nil {
			writer.Close()
			archive.Close()
			os.Remove(zipPath)
			respondExportError(c, http.StatusInternalServerError, "EXPORT_ARCHIVE_FAILED", "写入ZIP失败", true, "请稍后重试；若仍失败，请提供 request_id 给管理员", nil)
			return
		}
	}
	if err := writer.Close(); err != nil {
		archive.Close()
		os.Remove(zipPath)
		respondExportError(c, http.StatusInternalServerError, "EXPORT_ARCHIVE_FAILED", "完成ZIP失败", true, "请稍后重试；若仍失败，请提供 request_id 给管理员", nil)
		return
	}
	if err := archive.Close(); err != nil {
		os.Remove(zipPath)
		respondExportError(c, http.StatusInternalServerError, "EXPORT_ARCHIVE_FAILED", "保存ZIP失败", true, "请稍后重试；若仍失败，请提供 request_id 给管理员", nil)
		return
	}
	RecordAudit(c, "export_signoff_batch", "evaluation", fmt.Sprintf("count:%d", len(evaluations)), "SUCCESS")
	if err := publishExport(c, zipPath, fileName, fmt.Sprintf("已生成 %d 份签字表", len(evaluations)), results...); err != nil {
		respondExportError(c, http.StatusInternalServerError, "EXPORT_PUBLISH_FAILED", "生成下载链接失败", true, "请稍后重试；若仍失败，请提供 request_id 给管理员", nil)
	}
}

type signoffStatisticsItem struct {
	EvaluationID uint       `json:"evaluation_id"`
	Employee     string     `json:"employee"`
	Department   string     `json:"department"`
	Period       string     `json:"period"`
	TotalScore   float64    `json:"total_score"`
	Status       string     `json:"status"`
	Method       string     `json:"method"`
	ConfirmedAt  *time.Time `json:"confirmed_at,omitempty"`
	Handler      string     `json:"handler,omitempty"`
}

func GetSignoffStatistics(c *gin.Context) {
	evaluations, _, err := loadFilteredEvaluations(c, []string{"pending_confirm", "awaiting_employee_confirmation", "completed"})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	items := make([]signoffStatisticsItem, 0, len(evaluations))
	online, paper := 0, 0
	for _, evaluation := range evaluations {
		item := signoffStatisticsItem{EvaluationID: evaluation.ID, Employee: evaluation.Employee.Name, Department: evaluation.Employee.Department.Name, Period: formatPeriodDisplay(evaluation.Period, evaluation.Year, evaluation.Month, evaluation.Quarter), TotalScore: authoritativeEvaluationTotal(models.DB, evaluation), Status: evaluation.Status, Method: "pending"}
		var confirmation models.EvaluationConfirmation
		if err := models.DB.Preload("Handler").Where("evaluation_id = ?", evaluation.ID).First(&confirmation).Error; err == nil {
			item.Method, item.ConfirmedAt = confirmation.Method, &confirmation.ConfirmedAt
			if confirmation.Handler != nil {
				item.Handler = confirmation.Handler.Name
			}
			if confirmation.Method == "online" {
				online++
			} else if confirmation.Method == "paper" {
				paper++
			}
		}
		items = append(items, item)
	}
	total := len(items)
	received := online + paper
	rate := float64(0)
	if total > 0 {
		rate = float64(received) * 100 / float64(total)
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"summary": gin.H{"total": total, "online": online, "paper": paper, "pending": total - received, "received": received, "recovery_rate": rate}, "items": items}})
}
