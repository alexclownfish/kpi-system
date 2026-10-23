package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const maxSourceWorkbookSize int64 = 10 << 20

var sourceWorkbookMu sync.Mutex

var errSourceMappingInvalid = errors.New("source department mapping invalid")

type sourceWorkbookSummary struct {
	ProcessSheets     int `json:"process_sheets"`
	PositionTemplates int `json:"position_templates"`
	HRSummarySheets   int `json:"hr_summary_sheets"`
	SourceDepartments int `json:"source_departments"`
	Indicators        int `json:"indicators"`
}

type sourceWorkbookResponse struct {
	WorkbookID       string                    `json:"workbook_id"`
	SourceSHA256     string                    `json:"source_sha256"`
	ParserVersion    string                    `json:"parser_version"`
	SheetCount       int                       `json:"sheet_count"`
	Summary          sourceWorkbookSummary     `json:"summary"`
	Diagnostics      []models.ImportDiagnostic `json:"diagnostics"`
	CanImportDraft   bool                      `json:"can_import_draft"`
	CanPublish       bool                      `json:"can_publish"`
	SourceSheets     []sourceSheetPreview      `json:"source_sheets,omitempty"`
	TemplatePreviews []templateSourcePreview   `json:"template_previews,omitempty"`
}

type sourceSheetPreview struct {
	Name             string `json:"name"`
	Kind             string `json:"kind"`
	Order            int    `json:"order"`
	DiagnosticStatus string `json:"diagnostic_status"`
}

type templateSourcePreview struct {
	SourceSheet      string `json:"source_sheet"`
	Department       string `json:"department"`
	PositionType     string `json:"position_type"`
	Period           string `json:"period"`
	IndicatorCount   int    `json:"indicator_count"`
	DiagnosticStatus string `json:"diagnostic_status"`
}

func sourceWorkbookID(id uint) string { return fmt.Sprintf("workbook-%d", id) }

func parseSourceWorkbookID(raw string) (uint, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "workbook-") {
		raw = strings.TrimPrefix(raw, "workbook-")
	}
	id, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || id == 0 {
		return 0, errors.New("无效的工作簿ID")
	}
	return uint(id), nil
}

func sourceWorkbookFile(file *multipart.FileHeader) ([]byte, string, error) {
	if file == nil || file.Size <= 0 || file.Size > maxSourceWorkbookSize || strings.ToLower(filepathExt(file.Filename)) != ".xlsx" {
		return nil, "", errors.New("请上传不超过10MB的 .xlsx 文件")
	}
	reader, err := file.Open()
	if err != nil {
		return nil, "", errors.New("无法读取工作簿")
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxSourceWorkbookSize+1))
	if err != nil || int64(len(data)) > maxSourceWorkbookSize {
		return nil, "", errors.New("工作簿大小超过10MB")
	}
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:]), nil
}

func filepathExt(name string) string {
	index := strings.LastIndex(name, ".")
	if index < 0 {
		return ""
	}
	return name[index:]
}

func classifySourceSheet(name string) string {
	switch {
	case strings.Contains(name, "流程"):
		return "process"
	case strings.Contains(name, "HR汇总（月度）"):
		return "hr_monthly"
	case strings.Contains(name, "HR汇总（季度）"):
		return "hr_quarterly"
	default:
		return "position_template"
	}
}

func buildSourceDiagnostics(catalog hongxingCatalog) []models.ImportDiagnostic {
	rows := make([]models.ImportDiagnostic, 0)
	for _, profile := range catalog.Profiles {
		issues, _ := profileSourceIssues(profile)
		for _, issue := range issues {
			code, _ := issue["code"].(string)
			severity, _ := issue["severity"].(string)
			dimension, _ := issue["dimension"].(string)
			weight, _ := issue["weight_sum"].(float64)
			cells := ""
			if values, ok := issue["cells"].([]any); ok {
				parts := make([]string, 0, len(values))
				for _, value := range values {
					if text, ok := value.(string); ok {
						parts = append(parts, text)
					}
				}
				cells = strings.Join(parts, ",")
			} else if values, ok := issue["cells"].([]string); ok {
				cells = strings.Join(values, ",")
			}
			message, _ := issue["message"].(string)
			hint := "由 HR 修正后重新验证"
			if cells == "" {
				cells, _ = issue["source_cell"].(string)
			}
			raw, normalized := fmt.Sprintf("%.1f", weight), fmt.Sprintf("%.1f", weight)
			if strings.EqualFold(code, "dimension_item_weight_sum") {
				message = fmt.Sprintf("维度“%s”内指标权重合计为 %.1f%%；按原表权重计算，综合权重合计校验通过时不阻断发布", dimension, weight)
				hint = "保留原表权重，无需将每个维度内权重归一化为100%"
			} else if strings.EqualFold(code, "unknown_scorer") {
				raw, _ = issue["value"].(string)
				normalized = ""
				message = "无法识别原表评分人：" + raw
			} else if strings.EqualFold(code, "composite_formula_mismatch") {
				raw, _ = issue["actual"].(string)
				normalized, _ = issue["expected"].(string)
				message = "综合权重公式与原表规则不一致，期望 " + normalized
			}
			if message == "" {
				message = "工作表存在来源问题：" + code
			}
			rows = append(rows, models.ImportDiagnostic{Severity: severity, Code: strings.ToUpper(code), Sheet: profile.Sheet, Coordinate: cells, Cell: cells, RawValue: raw, NormalizedValue: normalized, Message: message, RepairHint: hint})
		}
	}
	for _, summary := range catalog.HRSummaryDiagnostics {
		for _, gap := range summary.FormulaGaps {
			severity, message, hint := "warning", "空白预留行缺少公式："+gap.Reason, "使用该预留行录入实际业务数据前请补齐公式"
			if gap.IsRealDataRow {
				severity, message, hint = "blocking", "真实数据行存在公式断档："+gap.Reason, "补齐公式后重新校验工作簿"
			}
			rows = append(rows, models.ImportDiagnostic{Severity: severity, Code: "HR_FORMULA_GAP", Sheet: summary.Sheet, Cell: gap.Cell, Coordinate: gap.Cell, PeriodType: summary.Kind, Message: message, RepairHint: hint, IsRealDataRow: gap.IsRealDataRow})
		}
		for _, issue := range summary.AnnualFormulaIssues {
			rows = append(rows, models.ImportDiagnostic{Severity: "warning", Code: "HR_ANNUAL_FORMULA_SCOPE", Sheet: summary.Sheet, Cell: issue.Cell, Coordinate: issue.Cell, PeriodType: summary.Kind, Formula: issue.Formula, ExpandedFormula: issue.Formula, Message: "年度公式引用范围与周期列不一致", RepairHint: "确认年度汇总是否应只引用 D:G"})
		}
	}
	return rows
}

func validateSourceWorkbookRecord(db *gorm.DB, userID uint, data []byte, sha, name string) (models.SourceWorkbook, sourceWorkbookResponse, error) {
	parsed, err := parseSourceWorkbook(data, sha, name)
	if err != nil {
		return models.SourceWorkbook{}, sourceWorkbookResponse{}, err
	}
	meta, err := json.Marshal(parsed.Metadata)
	if err != nil {
		return models.SourceWorkbook{}, sourceWorkbookResponse{}, err
	}
	workbook := models.SourceWorkbook{SourceName: name, SourceSHA256: sha, ParserVersion: sourceWorkbookParserVersion, UploadedBy: userID, Status: "validated", SheetCount: len(parsed.Sheets), SourceMetadataJSON: string(meta)}
	if err := db.Transaction(func(tx *gorm.DB) error {
		var existing models.SourceWorkbook
		if err := tx.Where("source_sha256 = ?", sha).First(&existing).Error; err == nil {
			// Revalidation updates parser artifacts, including stale diagnostic
			// severities, while preserving the import marker and all templates.
			workbook.ID, workbook.Status, workbook.UploadedBy = existing.ID, existing.Status, existing.UploadedBy
			workbook.SourceName = existing.SourceName
			if _, err := sourceCatalogFromMetadata(existing.SourceMetadataJSON); err == nil {
				// A valid parsed snapshot is immutable. Reuploading identical
				// bytes under another filename only refreshes diagnostics.
				workbook.SourceMetadataJSON = existing.SourceMetadataJSON
			} else {
				parsed.Catalog.SourceFilename = existing.SourceName
				parsed.Metadata.SourceFilename = existing.SourceName
				parsed.Metadata.Catalog = parsed.Catalog
				upgraded, err := json.Marshal(parsed.Metadata)
				if err != nil {
					return err
				}
				workbook.SourceMetadataJSON = string(upgraded)
			}
			if err := tx.Model(&existing).Updates(map[string]any{"parser_version": workbook.ParserVersion, "sheet_count": workbook.SheetCount, "source_metadata_json": workbook.SourceMetadataJSON}).Error; err != nil {
				return err
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&workbook).Error; err != nil {
				return err
			}
		} else {
			return err
		}
		for _, entity := range []any{&models.ImportDiagnostic{}, &models.FormulaCell{}, &models.HRAggregationRule{}, &models.SourceSheet{}} {
			if err := tx.Where("workbook_id = ?", workbook.ID).Delete(entity).Error; err != nil {
				return err
			}
		}
		sheetIDs := make(map[string]uint, len(parsed.Sheets))
		for _, sheet := range parsed.Sheets {
			sourceSheet := models.SourceSheet{WorkbookID: workbook.ID, Name: sheet.Name, Kind: sheet.Kind, SheetOrder: sheet.Order, DiagnosticStatus: "ok"}
			if err := tx.Create(&sourceSheet).Error; err != nil {
				return err
			}
			sheetIDs[sheet.Name] = sourceSheet.ID
		}
		diagnostics := buildSourceDiagnostics(parsed.Catalog)
		for index := range diagnostics {
			diagnostics[index].WorkbookID = workbook.ID
			if diagnostics[index].Sheet != "" {
				if sheetID := sheetIDs[diagnostics[index].Sheet]; sheetID != 0 {
					diagnostics[index].SheetID = &sheetID
				}
			}
			if err := tx.Create(&diagnostics[index]).Error; err != nil {
				return err
			}
		}
		for sheetName, formulas := range parsed.FormulaCells {
			for _, formula := range formulas {
				formula.ID = 0
				formula.WorkbookID = workbook.ID
				formula.SheetID = uintPointer(sheetIDs[sheetName])
				for _, diagnostic := range diagnostics {
					if diagnostic.Sheet == sheetName && diagnostic.Coordinate == formula.Coordinate {
						formula.DiagnosticStatus = diagnostic.Severity
					}
				}
				if err := tx.Create(&formula).Error; err != nil {
					return err
				}
			}
		}
		for _, summary := range parsed.Catalog.HRSummaryDiagnostics {
			sheetID := sheetIDs[summary.Sheet]
			for _, gap := range summary.FormulaGaps {
				severity := "warning"
				if gap.IsRealDataRow {
					severity = "blocking"
				}
				if err := tx.Create(&models.FormulaCell{WorkbookID: workbook.ID, SheetID: uintPointer(sheetID), Coordinate: gap.Cell, FormulaType: "missing", DiagnosticStatus: severity}).Error; err != nil {
					return err
				}
			}
			for _, issue := range summary.AnnualFormulaIssues {
				if err := tx.Create(&models.HRAggregationRule{WorkbookID: workbook.ID, SheetID: uintPointer(sheetID), PeriodType: summary.Kind, FieldName: "annual_total", FormulaCell: issue.Cell, ExpandedFormula: issue.Formula, AllocationThreshold: 0.20, DiagnosticStatus: "warning"}).Error; err != nil {
					return err
				}
			}
		}
		for sheetName, sheetID := range sheetIDs {
			status := "ok"
			for _, diagnostic := range diagnostics {
				if diagnostic.Sheet != sheetName {
					continue
				}
				if diagnostic.Severity == "blocking" || diagnostic.Severity == "error" {
					status = "blocking"
					break
				}
				if diagnostic.Severity == "warning" {
					status = "warning"
				}
			}
			if err := tx.Model(&models.SourceSheet{}).Where("id = ?", sheetID).Update("diagnostic_status", status).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return models.SourceWorkbook{}, sourceWorkbookResponse{}, err
	}
	var diagnostics []models.ImportDiagnostic
	db.Where("workbook_id = ?", workbook.ID).Order("severity DESC, id").Find(&diagnostics)
	hydrateSourceDiagnosticPresentation(db, workbook.ID, diagnostics)
	return workbook, sourceWorkbookResponse{WorkbookID: sourceWorkbookID(workbook.ID), SourceSHA256: sha, ParserVersion: sourceWorkbookParserVersion, SheetCount: len(parsed.Sheets), Summary: parsed.Summary, Diagnostics: diagnostics, CanImportDraft: workbook.Status != "imported", CanPublish: countBlockingDiagnostics(diagnostics) == 0, SourceSheets: sourceSheetPreviews(db, workbook.ID), TemplatePreviews: catalogTemplatePreviews(parsed.Catalog)}, nil
}

func uintPointer(value uint) *uint {
	if value == 0 {
		return nil
	}
	return &value
}

func countBlockingDiagnostics(diagnostics []models.ImportDiagnostic) int {
	count := 0
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "blocking" || diagnostic.Severity == "error" {
			count++
		}
	}
	return count
}

func sourceSheetPreviews(db *gorm.DB, workbookID uint) []sourceSheetPreview {
	var sheets []models.SourceSheet
	db.Where("workbook_id = ?", workbookID).Order("sheet_order").Find(&sheets)
	result := make([]sourceSheetPreview, 0, len(sheets))
	for _, sheet := range sheets {
		result = append(result, sourceSheetPreview{Name: sheet.Name, Kind: sheet.Kind, Order: sheet.SheetOrder, DiagnosticStatus: sheet.DiagnosticStatus})
	}
	return result
}

func catalogTemplatePreviews(catalog hongxingCatalog) []templateSourcePreview {
	result := make([]templateSourcePreview, 0, len(catalog.Profiles))
	for _, profile := range catalog.Profiles {
		count := 0
		for _, dimension := range profile.Dimensions {
			count += len(dimension.Items)
		}
		status := "ok"
		issues, blocked := profileSourceIssues(profile)
		if blocked {
			status = "blocking"
		} else if len(issues) > 0 {
			status = "warning"
		}
		result = append(result, templateSourcePreview{SourceSheet: profile.Sheet, Department: profile.Department, PositionType: profile.Position, Period: profile.Period, IndicatorCount: count, DiagnosticStatus: status})
	}
	return result
}

func hydrateSourceDiagnosticPresentation(db *gorm.DB, workbookID uint, diagnostics []models.ImportDiagnostic) {
	var sheets []models.SourceSheet
	db.Where("workbook_id = ?", workbookID).Find(&sheets)
	names := make(map[uint]string, len(sheets))
	for _, sheet := range sheets {
		names[sheet.ID] = sheet.Name
	}
	for index := range diagnostics {
		if diagnostics[index].SheetID != nil {
			diagnostics[index].Sheet = names[*diagnostics[index].SheetID]
		}
		diagnostics[index].Cell = diagnostics[index].Coordinate
	}
	var workbook models.SourceWorkbook
	if db.First(&workbook, workbookID).Error != nil {
		return
	}
	catalog, err := sourceCatalogForWorkbook(workbook)
	if err != nil {
		return
	}
	byKey := make(map[string]models.ImportDiagnostic)
	for _, diagnostic := range buildSourceDiagnostics(catalog) {
		byKey[diagnostic.Sheet+"|"+diagnostic.Coordinate+"|"+diagnostic.Code] = diagnostic
	}
	for index := range diagnostics {
		diagnostic := &diagnostics[index]
		if source, ok := byKey[diagnostic.Sheet+"|"+diagnostic.Coordinate+"|"+diagnostic.Code]; ok {
			diagnostic.PeriodType = source.PeriodType
			diagnostic.Formula = source.Formula
			diagnostic.ExpandedFormula = source.ExpandedFormula
			diagnostic.IsRealDataRow = source.IsRealDataRow
		}
	}
}

func sourceCatalogForWorkbook(workbook models.SourceWorkbook) (hongxingCatalog, error) {
	catalog, err := sourceCatalogFromMetadata(workbook.SourceMetadataJSON)
	if err == nil {
		if catalog.SourceSHA256 != workbook.SourceSHA256 {
			return hongxingCatalog{}, errors.New("来源快照 SHA-256 不匹配，请重新验证")
		}
		return catalog, nil
	}
	// Only pre-v3 validation records of the exact embedded source can use the
	// legacy catalog. A changed workbook must supply its own parsed snapshot.
	if workbook.ParserVersion == sourceWorkbookParserVersion {
		return hongxingCatalog{}, err
	}
	if json.Unmarshal(hongxingCatalogJSON, &catalog) == nil && catalog.SourceSHA256 == workbook.SourceSHA256 {
		return catalog, nil
	}
	return hongxingCatalog{}, err
}

func ValidateSourceWorkbook(c *gin.Context) {
	file, err := c.FormFile("workbook")
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_WORKBOOK", "请上传工作簿", nil)
		return
	}
	data, sha, err := sourceWorkbookFile(file)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_WORKBOOK", err.Error(), nil)
		return
	}
	sourceWorkbookMu.Lock()
	defer sourceWorkbookMu.Unlock()
	_, response, err := validateSourceWorkbookRecord(models.DB, actorID(c), data, sha, file.Filename)
	if err != nil {
		respondError(c, http.StatusBadRequest, "WORKBOOK_PARSE_FAILED", err.Error(), nil)
		return
	}
	RecordAuditDetails(c, "source_workbook_validate", "source_workbook", response.WorkbookID, "SUCCESS", fmt.Sprintf("sha256=%s;diagnostics=%d", sha, len(response.Diagnostics)))
	respondData(c, http.StatusOK, response, "工作簿验证完成")
}

func GetSourceWorkbookDiagnostics(c *gin.Context) {
	id, err := parseSourceWorkbookID(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_SOURCE_WORKBOOK_ID", "无效的工作簿ID", nil)
		return
	}
	var workbook models.SourceWorkbook
	if err := models.DB.First(&workbook, id).Error; err != nil {
		respondError(c, http.StatusNotFound, "SOURCE_WORKBOOK_NOT_FOUND", "来源工作簿不存在", nil)
		return
	}
	var diagnostics []models.ImportDiagnostic
	models.DB.Where("workbook_id = ?", id).Order("severity DESC, id").Find(&diagnostics)
	hydrateSourceDiagnosticPresentation(models.DB, id, diagnostics)
	respondData(c, http.StatusOK, gin.H{"workbook": workbook, "diagnostics": diagnostics}, "诊断读取成功")
}

func GetSourceWorkbookHRDiagnostics(c *gin.Context) {
	id, err := parseSourceWorkbookID(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_SOURCE_WORKBOOK_ID", "无效的工作簿ID", nil)
		return
	}
	var workbook models.SourceWorkbook
	if err := models.DB.First(&workbook, id).Error; err != nil {
		respondError(c, http.StatusNotFound, "SOURCE_WORKBOOK_NOT_FOUND", "来源工作簿不存在", nil)
		return
	}
	var diagnostics []models.ImportDiagnostic
	models.DB.Where("workbook_id = ? AND code LIKE ?", id, "HR_%").Order("severity DESC, id").Find(&diagnostics)
	hydrateSourceDiagnosticPresentation(models.DB, id, diagnostics)
	respondData(c, http.StatusOK, diagnostics, "HR 汇总诊断读取成功")
}

type importSourceWorkbookRequest struct {
	SourceSHA256              string `json:"source_sha256"`
	Mode                      string `json:"mode"`
	AllowWarningDiagnostics   bool   `json:"allow_warning_diagnostics"`
	ExplicitDepartmentMapping []struct {
		RawValue       string `json:"raw_value"`
		DepartmentID   *uint  `json:"department_id,omitempty"`
		DepartmentName string `json:"department_name,omitempty"`
	} `json:"explicit_department_mappings"`
}

type sourceWorkbookImportResponse struct {
	WorkbookID       string `json:"workbook_id"`
	SourceSHA256     string `json:"source_sha256"`
	Status           string `json:"status"`
	Mode             string `json:"mode"`
	CreatedTemplates int    `json:"created_templates"`
	ReusedTemplates  int    `json:"reused_templates"`
	TemplateIDs      []uint `json:"template_ids"`
}

func ImportSourceWorkbook(c *gin.Context) {
	id, err := parseSourceWorkbookID(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_SOURCE_WORKBOOK_ID", "无效的工作簿ID", nil)
		return
	}
	var request importSourceWorkbookRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_IMPORT_REQUEST", "导入参数无效", nil)
		return
	}
	request.Mode = strings.ToLower(strings.TrimSpace(request.Mode))
	if request.Mode == "" {
		request.Mode = "draft"
	}
	if request.Mode != "draft" && request.Mode != "publish" {
		respondError(c, http.StatusBadRequest, "INVALID_IMPORT_MODE", "导入模式必须为 draft 或 publish", nil)
		return
	}
	sourceWorkbookMu.Lock()
	defer sourceWorkbookMu.Unlock()
	var workbook models.SourceWorkbook
	if err := models.DB.First(&workbook, id).Error; err != nil {
		respondError(c, http.StatusNotFound, "SOURCE_WORKBOOK_NOT_FOUND", "来源工作簿不存在", nil)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(request.SourceSHA256), workbook.SourceSHA256) {
		respondError(c, http.StatusConflict, "SOURCE_SHA_MISMATCH", "来源 SHA-256 不匹配", nil)
		return
	}
	if workbook.Status == "imported" {
		var existing []models.KPITemplate
		if err := models.DB.Where("source_sha256 = ?", workbook.SourceSHA256).Order("id").Find(&existing).Error; err != nil {
			respondError(c, http.StatusInternalServerError, "IMPORT_CATALOG_FAILED", "读取已导入模板失败", nil)
			return
		}
		if len(existing) == 0 {
			respondError(c, http.StatusConflict, "SOURCE_VERSION_ALREADY_IMPORTED", "该来源版本已经导入，但模板目录不存在，请联系管理员核查", nil)
			return
		}
		if request.Mode == "publish" {
			notPublished := make([]uint, 0)
			for _, template := range existing {
				if template.Status != "published" || !template.IsActive {
					notPublished = append(notPublished, template.ID)
				}
			}
			if len(notPublished) > 0 {
				respondError(c, http.StatusConflict, "SOURCE_VERSION_ALREADY_IMPORTED", "该来源已导入，但仍有草稿或停用模板，请在模板列表显式发布", gin.H{"template_ids": notPublished})
				return
			}
		}
		templateIDs := make([]uint, 0, len(existing))
		for _, template := range existing {
			templateIDs = append(templateIDs, template.ID)
		}
		respondData(c, http.StatusOK, sourceWorkbookImportResponse{
			WorkbookID:       sourceWorkbookID(workbook.ID),
			SourceSHA256:     workbook.SourceSHA256,
			Status:           workbook.Status,
			Mode:             request.Mode,
			CreatedTemplates: 0,
			ReusedTemplates:  len(existing),
			TemplateIDs:      templateIDs,
		}, "来源工作簿已存在，已复用当前版本")
		return
	}
	var diagnostics int64
	if err := models.DB.Model(&models.ImportDiagnostic{}).Where("workbook_id = ? AND severity IN ?", id, []string{"blocking", "error"}).Count(&diagnostics).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "IMPORT_DIAGNOSTICS_FAILED", "无法读取来源诊断", nil)
		return
	}
	if request.Mode == "publish" && diagnostics > 0 {
		respondError(c, http.StatusConflict, "IMPORT_PUBLISH_BLOCKED", "来源包含阻断诊断，只能导入草稿", gin.H{"blocking": diagnostics})
		return
	}
	if request.Mode == "publish" && !request.AllowWarningDiagnostics {
		var warnings int64
		if err := models.DB.Model(&models.ImportDiagnostic{}).Where("workbook_id = ? AND severity = ?", id, "warning").Count(&warnings).Error; err != nil {
			respondError(c, http.StatusInternalServerError, "IMPORT_DIAGNOSTICS_FAILED", "无法读取来源诊断", nil)
			return
		}
		if warnings > 0 {
			respondError(c, http.StatusConflict, "IMPORT_WARNINGS_NOT_ACKNOWLEDGED", "请确认保留来源告警后再发布", gin.H{"warnings": warnings})
			return
		}
	}
	catalog, err := sourceCatalogForWorkbook(workbook)
	if err != nil {
		respondError(c, http.StatusConflict, "IMPORT_CATALOG_FAILED", err.Error(), nil)
		return
	}
	var templatesBefore int64
	models.DB.Model(&models.KPITemplate{}).Where("source_sha256 = ?", workbook.SourceSHA256).Count(&templatesBefore)
	if err := models.DB.Transaction(func(tx *gorm.DB) error {
		// The row predicate makes retries safe when two browser tabs submit the
		// same validated version at once. No template or business record is
		// written until this state transition succeeds.
		result := tx.Model(&models.SourceWorkbook{}).Where("id = ? AND source_sha256 = ? AND status = ?", workbook.ID, workbook.SourceSHA256, "validated").Update("status", "imported")
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrDuplicatedKey
		}
		mappedDepartments := make(map[string]uint)
		var metadata sourceWorkbookMetadata
		_ = json.Unmarshal([]byte(workbook.SourceMetadataJSON), &metadata)
		for _, mapping := range request.ExplicitDepartmentMapping {
			raw := strings.TrimSpace(mapping.RawValue)
			if raw == "" {
				return fmt.Errorf("%w: 显式部门映射的原始名称不能为空", errSourceMappingInvalid)
			}
			departmentID := uint(0)
			if mapping.DepartmentID != nil {
				departmentID = *mapping.DepartmentID
			}
			if departmentID == 0 && strings.TrimSpace(mapping.DepartmentName) != "" {
				var departments []models.Department
				if err := tx.Where("name = ?", strings.TrimSpace(mapping.DepartmentName)).Find(&departments).Error; err != nil || len(departments) != 1 {
					return fmt.Errorf("%w: 部门映射目标不存在：%s", errSourceMappingInvalid, mapping.DepartmentName)
				}
				departmentID = departments[0].ID
			}
			if departmentID == 0 {
				return fmt.Errorf("%w: 部门映射缺少目标部门：%s", errSourceMappingInvalid, raw)
			}
			var department models.Department
			if err := tx.First(&department, departmentID).Error; err != nil {
				return fmt.Errorf("%w: 部门映射目标不存在：%d", errSourceMappingInvalid, departmentID)
			}
			if strings.TrimSpace(mapping.DepartmentName) != "" && department.Name != strings.TrimSpace(mapping.DepartmentName) {
				return fmt.Errorf("%w: 部门映射目标ID和名称不一致", errSourceMappingInvalid)
			}
			sourceDepartment := raw
			for _, origin := range metadata.DepartmentOrigins {
				if strings.TrimSpace(origin.RawValue) == raw {
					sourceDepartment = origin.Value
					break
				}
			}
			known := false
			for _, profile := range catalog.Profiles {
				known = known || profile.Department == sourceDepartment
			}
			if !known {
				return fmt.Errorf("%w: 工作簿没有来源部门：%s", errSourceMappingInvalid, raw)
			}
			if _, exists := mappedDepartments[sourceDepartment]; exists {
				return fmt.Errorf("%w: 来源部门不能重复映射：%s", errSourceMappingInvalid, raw)
			}
			mappedDepartments[sourceDepartment] = departmentID
			mappingRecord := models.DepartmentMapping{WorkbookID: workbook.ID, SourceLabel: raw, NormalizedLabel: strings.ToLower(raw), DepartmentID: &departmentID, MappingReason: "explicit_hr_mapping", RawValue: mapping.RawValue}
			if err := tx.Create(&mappingRecord).Error; err != nil {
				return err
			}
		}
		// Resolve mappings before materializing templates. The validated raw
		// catalog remains immutable; only new templates use the chosen target.
		if err := importHongxingCatalogWithDepartments(tx, catalog, request.Mode, false, mappedDepartments); err != nil {
			return fmt.Errorf("模板目录导入失败: %w", err)
		}
		return nil
	}); err != nil {
		if errors.Is(err, errSourcePublishConflict) {
			respondError(c, http.StatusConflict, "IMPORT_PUBLISH_CONFLICT", err.Error(), nil)
			return
		}
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			respondError(c, http.StatusConflict, "SOURCE_VERSION_ALREADY_IMPORTED", "该来源版本已经导入", nil)
			return
		}
		if errors.Is(err, errSourceMappingInvalid) {
			respondError(c, http.StatusBadRequest, "INVALID_DEPARTMENT_MAPPING", err.Error(), nil)
			return
		}
		respondError(c, http.StatusInternalServerError, "IMPORT_ATOMICITY_FAILED", "来源工作簿导入失败", nil)
		return
	}
	workbook.Status = "imported"
	var templates []models.KPITemplate
	models.DB.Where("source_sha256 = ?", workbook.SourceSHA256).Order("id").Find(&templates)
	templateIDs := make([]uint, 0, len(templates))
	for _, template := range templates {
		templateIDs = append(templateIDs, template.ID)
	}
	createdTemplates := len(templateIDs) - int(templatesBefore)
	if createdTemplates < 0 {
		createdTemplates = 0
	}
	RecordAuditDetails(c, "source_workbook_import", "source_workbook", sourceWorkbookID(workbook.ID), "SUCCESS", fmt.Sprintf("sha256=%s;mode=%s", workbook.SourceSHA256, request.Mode))
	respondData(c, http.StatusCreated, sourceWorkbookImportResponse{WorkbookID: sourceWorkbookID(workbook.ID), SourceSHA256: workbook.SourceSHA256, Status: workbook.Status, Mode: request.Mode, CreatedTemplates: createdTemplates, ReusedTemplates: int(templatesBefore), TemplateIDs: templateIDs}, "来源工作簿已导入")
}
