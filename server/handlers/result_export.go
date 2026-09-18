package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"dootask-kpi-server/global"
	"dootask-kpi-server/models"
	"dootask-kpi-server/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

const (
	exportPresetFinalSignoff = "final_signoff"
	exportPresetFullProcess  = "full_process"
)

var exportColumnLabels = map[string]string{
	"item_name":        "指标",
	"item_description": "指标说明",
	"max_score":        "满分",
	"self_score":       "自评分",
	"self_comment":     "自评说明",
	"manager_score":    "主管评分",
	"manager_comment":  "主管说明",
	"hr_score":         "HR评分",
	"hr_comment":       "HR说明",
	"final_score":      "最终得分",
	"final_comment":    "最终评价",
}

func defaultExportLayout(preset string) models.ResultExportLayout {
	if preset == exportPresetFullProcess {
		return models.ResultExportLayout{
			Version: 1, Preset: exportPresetFullProcess, Title: "绩效考核结果确认表",
			Columns:     []string{"item_name", "item_description", "max_score", "self_score", "self_comment", "manager_score", "manager_comment", "hr_score", "hr_comment", "final_score", "final_comment"},
			ShowSummary: true, ShowEmployeeOpinion: true,
			SignatureLabels: []string{"员工签字", "直属主管签字", "HR签字", "签字日期"},
		}
	}
	return models.ResultExportLayout{
		Version: 1, Preset: exportPresetFinalSignoff, Title: "绩效考核结果确认表",
		Columns:     []string{"item_name", "max_score", "final_score", "final_comment"},
		ShowSummary: true, ShowEmployeeOpinion: true,
		SignatureLabels: []string{"员工签字", "直属主管签字", "HR签字", "签字日期"},
	}
}

func normalizeExportLayout(layout models.ResultExportLayout) models.ResultExportLayout {
	preset := strings.TrimSpace(layout.Preset)
	if preset != exportPresetFullProcess {
		preset = exportPresetFinalSignoff
	}
	base := defaultExportLayout(preset)
	if layout.Version == 0 && layout.Title == "" && len(layout.Columns) == 0 && len(layout.SignatureLabels) == 0 {
		return base
	}
	base.Version = 1
	base.Title = strings.TrimSpace(layout.Title)
	if base.Title == "" {
		base.Title = "绩效考核结果确认表"
	}
	if len(layout.Columns) > 0 {
		base.Columns = append([]string(nil), layout.Columns...)
	}
	base.ShowSummary = layout.ShowSummary
	base.ShowEmployeeOpinion = layout.ShowEmployeeOpinion
	if len(layout.SignatureLabels) > 0 {
		base.SignatureLabels = append([]string(nil), layout.SignatureLabels...)
	}
	return base
}

func validateExportLayout(layout models.ResultExportLayout) (models.ResultExportLayout, error) {
	layout = normalizeExportLayout(layout)
	if len([]rune(layout.Title)) < 1 || len([]rune(layout.Title)) > 60 {
		return layout, errors.New("标题长度必须为1到60个字符")
	}
	if len(layout.Columns) < 2 || len(layout.Columns) > len(exportColumnLabels) {
		return layout, errors.New("导出列数量必须为2到11个")
	}
	seen := map[string]bool{}
	for _, key := range layout.Columns {
		if _, ok := exportColumnLabels[key]; !ok {
			return layout, fmt.Errorf("不支持的导出字段: %s", key)
		}
		if seen[key] {
			return layout, fmt.Errorf("导出字段不能重复: %s", key)
		}
		seen[key] = true
	}
	if !seen["item_name"] || !seen["final_score"] {
		return layout, errors.New("导出列必须包含指标和最终得分")
	}
	if len(layout.SignatureLabels) < 1 || len(layout.SignatureLabels) > 6 {
		return layout, errors.New("签字栏数量必须为1到6个")
	}
	signatures := map[string]bool{}
	for i, label := range layout.SignatureLabels {
		label = strings.TrimSpace(label)
		if label == "" || len([]rune(label)) > 20 {
			return layout, errors.New("签字栏名称必须为1到20个字符")
		}
		if signatures[label] {
			return layout, fmt.Errorf("签字栏名称不能重复: %s", label)
		}
		signatures[label] = true
		layout.SignatureLabels[i] = label
	}
	return layout, nil
}

func normalizeStoredExportLayout(raw string) (models.ResultExportLayout, error) {
	if strings.TrimSpace(raw) == "" {
		return defaultExportLayout(exportPresetFinalSignoff), nil
	}
	var layout models.ResultExportLayout
	if err := json.Unmarshal([]byte(raw), &layout); err != nil {
		return defaultExportLayout(exportPresetFinalSignoff), err
	}
	return validateExportLayout(layout)
}

func marshalExportLayout(layout models.ResultExportLayout) (string, models.ResultExportLayout, error) {
	normalized, err := validateExportLayout(layout)
	if err != nil {
		return "", normalized, err
	}
	raw, err := json.Marshal(normalized)
	return string(raw), normalized, err
}

type exportDocument struct {
	Title               string
	EmployeeName        string
	Department          string
	Position            string
	Period              string
	TemplateName        string
	TotalScore          string
	Columns             []string
	Headers             []string
	Rows                [][]string
	ShowSummary         bool
	Summary             string
	ShowEmployeeOpinion bool
	SignatureLabels     []string
	EvaluationCode      string
	Version             int
	Checksum            string
}

func scoreText(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', 2, 64)
}

func finalScoreText(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func itemExportValue(item resultSnapshotItem, key string) string {
	switch key {
	case "item_name":
		return item.Name
	case "item_description":
		return item.Description
	case "max_score":
		return finalScoreText(item.MaxScore)
	case "self_score":
		return scoreText(item.SelfScore)
	case "self_comment":
		return item.SelfComment
	case "manager_score":
		return scoreText(item.ManagerScore)
	case "manager_comment":
		return item.ManagerComment
	case "hr_score":
		return scoreText(item.HRScore)
	case "hr_comment":
		return item.HRComment
	case "final_score":
		return finalScoreText(item.FinalScore)
	case "final_comment":
		return item.FinalComment
	default:
		return ""
	}
}

func buildExportDocument(snapshot models.EvaluationResultSnapshot, payload resultSnapshotPayload) exportDocument {
	layout := normalizeExportLayout(payload.ExportLayout)
	doc := exportDocument{
		Title: layout.Title, EmployeeName: payload.EmployeeName, Department: payload.Department,
		Position: payload.Position, Period: formatPeriodDisplay(payload.Period, payload.Year, payload.Month, payload.Quarter),
		TemplateName: payload.TemplateName, TotalScore: finalScoreText(payload.TotalScore),
		Columns: append([]string(nil), layout.Columns...), ShowSummary: layout.ShowSummary,
		Summary: payload.FinalComment, ShowEmployeeOpinion: layout.ShowEmployeeOpinion,
		SignatureLabels: append([]string(nil), layout.SignatureLabels...),
		EvaluationCode:  fmt.Sprintf("KPI-%06d", payload.EvaluationID), Version: snapshot.Version, Checksum: snapshot.Checksum,
	}
	for _, key := range doc.Columns {
		doc.Headers = append(doc.Headers, exportColumnLabels[key])
	}
	for _, item := range payload.Items {
		row := make([]string, 0, len(doc.Columns))
		for _, key := range doc.Columns {
			row = append(row, itemExportValue(item, key))
		}
		doc.Rows = append(doc.Rows, row)
	}
	return doc
}

func renderResultExcel(doc exportDocument, path string) error {
	f := excelize.NewFile()
	defer f.Close()
	sheet := "绩效结果"
	f.SetSheetName("Sheet1", sheet)
	lastCol, _ := excelize.ColumnNumberToName(len(doc.Headers))
	f.SetCellValue(sheet, "A1", doc.Title)
	f.MergeCell(sheet, "A1", lastCol+"1")
	titleStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 16}, Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"}})
	f.SetCellStyle(sheet, "A1", lastCol+"1", titleStyle)
	meta := [][]string{{"员工", doc.EmployeeName, "部门", doc.Department}, {"岗位", doc.Position, "周期", doc.Period}, {"模板", doc.TemplateName, "总分", doc.TotalScore}, {"结果版本", fmt.Sprintf("V%d", doc.Version), "校验码", shortChecksum(doc.Checksum)}}
	row := 3
	for _, values := range meta {
		for index, value := range values {
			col, _ := excelize.ColumnNumberToName(index + 1)
			f.SetCellValue(sheet, col+strconv.Itoa(row), value)
		}
		row++
	}
	row++
	headerRow := row
	headerStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Color: []string{"#E6E6FA"}, Pattern: 1}, Border: tableBorders(), Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true}})
	dataStyle, _ := f.NewStyle(&excelize.Style{Border: tableBorders(), Alignment: &excelize.Alignment{Vertical: "top", WrapText: true}})
	for index, header := range doc.Headers {
		cell, _ := excelize.CoordinatesToCellName(index+1, row)
		f.SetCellValue(sheet, cell, header)
		f.SetCellStyle(sheet, cell, cell, headerStyle)
	}
	for _, values := range doc.Rows {
		row++
		for index, value := range values {
			cell, _ := excelize.CoordinatesToCellName(index+1, row)
			f.SetCellValue(sheet, cell, value)
			f.SetCellStyle(sheet, cell, cell, dataStyle)
		}
	}
	if doc.ShowSummary {
		row += 2
		f.SetCellValue(sheet, "A"+strconv.Itoa(row), "总结评价："+doc.Summary)
		f.MergeCell(sheet, "A"+strconv.Itoa(row), lastCol+strconv.Itoa(row+2))
	}
	if doc.ShowEmployeeOpinion {
		row += 4
		f.SetCellValue(sheet, "A"+strconv.Itoa(row), "员工确认意见：")
		f.MergeCell(sheet, "A"+strconv.Itoa(row), lastCol+strconv.Itoa(row+1))
		row++
	}
	row += 3
	for index, label := range doc.SignatureLabels {
		if index > 0 && index%2 == 0 {
			row += 2
		}
		col := 1
		if index%2 == 1 {
			col = maxInt(2, (len(doc.Headers)+1)/2)
		}
		cell, _ := excelize.CoordinatesToCellName(col, row)
		if label == "签字日期" {
			f.SetCellValue(sheet, cell, label+"：______年____月____日")
		} else {
			f.SetCellValue(sheet, cell, label+"：________________")
		}
	}
	row += 3
	f.SetCellValue(sheet, "A"+strconv.Itoa(row), "考核编号："+doc.EvaluationCode)
	f.SetCellValue(sheet, "A"+strconv.Itoa(row+1), "完整校验码："+doc.Checksum)
	f.MergeCell(sheet, "A"+strconv.Itoa(row+1), lastCol+strconv.Itoa(row+1))
	for index, key := range doc.Columns {
		col, _ := excelize.ColumnNumberToName(index + 1)
		width := 14.0
		if strings.Contains(key, "comment") || key == "item_description" || key == "item_name" {
			width = 24
		}
		f.SetColWidth(sheet, col, col, width)
	}
	landscape := len(doc.Headers) > 6
	orientation := "portrait"
	if landscape {
		orientation = "landscape"
	}
	fitWidth, fitHeight, paperSize, fitToPage := 1, 0, 9, true
	_ = f.SetSheetProps(sheet, &excelize.SheetPropsOptions{FitToPage: &fitToPage})
	_ = f.SetPageLayout(sheet, &excelize.PageLayoutOptions{Orientation: &orientation, Size: &paperSize, FitToWidth: &fitWidth, FitToHeight: &fitHeight})
	_ = f.SetDefinedName(&excelize.DefinedName{Name: "_xlnm.Print_Area", RefersTo: fmt.Sprintf("'%s'!$A$1:$%s$%d", sheet, lastCol, row+1), Scope: sheet})
	_ = f.SetDefinedName(&excelize.DefinedName{Name: "_xlnm.Print_Titles", RefersTo: fmt.Sprintf("'%s'!$%d:$%d", sheet, headerRow, headerRow), Scope: sheet})
	return f.SaveAs(path)
}

func tableBorders() []excelize.Border {
	return []excelize.Border{{Type: "left", Color: "000000", Style: 1}, {Type: "top", Color: "000000", Style: 1}, {Type: "bottom", Color: "000000", Style: 1}, {Type: "right", Color: "000000", Style: 1}}
}

func shortChecksum(checksum string) string {
	if len(checksum) <= 16 {
		return checksum
	}
	return checksum[:16]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

const resultPDFHTML = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><style>
@page{size:A4 landscape;margin:12mm}body{font-family:"Noto Sans CJK SC","Microsoft YaHei",sans-serif;color:#111;font-size:11px}h1{text-align:center;font-size:21px;margin:0 0 14px}.meta,.scores{width:100%;border-collapse:collapse}.meta{margin-bottom:12px}.meta td,.scores th,.scores td{border:1px solid #333;padding:6px;vertical-align:top;word-break:break-word}.scores th{background:#eee}.scores thead{display:table-header-group}.summary,.opinion{border:1px solid #333;min-height:52px;padding:8px;margin-top:10px}.signatures{margin-top:26px;display:grid;grid-template-columns:1fr 1fr;gap:24px 42px}.foot{margin-top:24px;font-size:9px;word-break:break-all;color:#555}tr{break-inside:avoid}</style></head><body>
<h1>{{.Title}}</h1><table class="meta"><tr><td>员工：{{.EmployeeName}}</td><td>部门：{{.Department}}</td></tr><tr><td>岗位：{{.Position}}</td><td>周期：{{.Period}}</td></tr><tr><td>模板：{{.TemplateName}}</td><td>总分：{{.TotalScore}}</td></tr></table>
<table class="scores"><thead><tr>{{range .Headers}}<th>{{.}}</th>{{end}}</tr></thead><tbody>{{range .Rows}}<tr>{{range .}}<td>{{.}}</td>{{end}}</tr>{{end}}</tbody></table>
{{if .ShowSummary}}<div class="summary"><strong>总结评价：</strong><br>{{.Summary}}</div>{{end}}{{if .ShowEmployeeOpinion}}<div class="opinion"><strong>员工确认意见：</strong></div>{{end}}<div class="signatures">{{range .SignatureLabels}}<div>{{.}}：{{if eq . "签字日期"}}______年____月____日{{else}}________________{{end}}</div>{{end}}</div>
<div class="foot">考核编号：{{.EvaluationCode}}　结果版本：V{{.Version}}<br>校验码：{{.Checksum}}</div></body></html>`

func renderResultPDF(doc exportDocument, path string) error {
	tmpl, err := template.New("result").Parse(resultPDFHTML)
	if err != nil {
		return err
	}
	absolutePDFPath, htmlPath, htmlURL, err := resultPDFPaths(path)
	if err != nil {
		return err
	}
	file, err := os.Create(htmlPath)
	if err != nil {
		return err
	}
	if err = tmpl.Execute(file, doc); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	defer os.Remove(htmlPath)
	binary, err := chromiumBinary()
	if err != nil {
		return err
	}
	output, err := exec.Command(binary, "--headless", "--no-sandbox", "--disable-gpu", "--no-pdf-header-footer", "--print-to-pdf="+absolutePDFPath, htmlURL).CombinedOutput()
	if err != nil {
		return fmt.Errorf("PDF生成失败: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func resultPDFPaths(path string) (string, string, string, error) {
	absolutePDFPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", "", err
	}
	htmlPath := strings.TrimSuffix(absolutePDFPath, filepath.Ext(absolutePDFPath)) + ".html"
	htmlURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(htmlPath)}).String()
	return absolutePDFPath, htmlPath, htmlURL, nil
}

var renderResultPDFFile = renderResultPDF

func normalizeExportFormat(value string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(value))
	if format == "" {
		format = "xlsx"
	}
	if format != "xlsx" && format != "pdf" {
		return "", errors.New("仅支持 xlsx 或 pdf 格式")
	}
	return format, nil
}

func parseSnapshotPayload(snapshot models.EvaluationResultSnapshot) (resultSnapshotPayload, error) {
	var payload resultSnapshotPayload
	if err := json.Unmarshal([]byte(snapshot.SnapshotJSON), &payload); err != nil {
		return payload, err
	}
	payload.ExportLayout = normalizeExportLayout(payload.ExportLayout)
	return payload, nil
}

// ExportEvaluationToExcel keeps the historical route name while supporting a
// format query and routing both outputs through the unified document model.
func ExportEvaluationToExcel(c *gin.Context) {
	evaluationID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的评估ID"})
		return
	}
	format, err := normalizeExportFormat(c.Query("format"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var evaluation models.KPIEvaluation
	result := ApplyEvaluationScope(models.DB.Preload("Employee.Department").Preload("Template").Preload("Scores.Item"), c.GetUint("user_id")).First(&evaluation, evaluationID)
	if result.Error != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "评估不存在或超出数据范围"})
		return
	}
	if evaluation.Status != "pending_confirm" && evaluation.Status != "completed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "最终结果尚未定稿，暂不能导出签字表"})
		return
	}
	snapshot, payload, err := GetOrCreateResultSnapshot(models.DB, evaluation.ID, c.GetUint("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成结果版本失败"})
		return
	}
	doc := buildExportDocument(snapshot, payload)
	if err := os.MkdirAll(ExportDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建导出目录失败"})
		return
	}
	base := fmt.Sprintf("绩效结果-%s-%s-%d", safeFileName(payload.EmployeeName), safeFileName(doc.Period), time.Now().Unix())
	fileName := base + "." + format
	filePath := filepath.Join(ExportDir, fileName)
	if format == "pdf" {
		err = renderResultPDFFile(doc, filePath)
	} else {
		err = renderResultExcel(doc, filePath)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成导出文件失败", "message": err.Error()})
		return
	}
	info, err := os.Stat(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取导出文件失败"})
		return
	}
	randomKey := "export_" + uuid.NewString()
	global.Cache.Set(randomKey, fileName, 5*time.Minute)
	downloadURL := utils.GetFileURL(c.GetString("base_url"), fmt.Sprintf("/api/download/exports/%s", randomKey))
	RecordAuditDetails(c, "export_evaluation_result", "evaluation", strconv.FormatUint(uint64(evaluation.ID), 10), "SUCCESS", "format="+format)
	c.JSON(http.StatusOK, ExportResponse{FileURL: downloadURL, FileName: fileName, FileSize: info.Size(), Message: "导出成功", ResultVersion: snapshot.Version, Checksum: snapshot.Checksum})
	go func() {
		time.Sleep(30 * time.Minute)
		_ = os.Remove(filePath)
	}()
}
