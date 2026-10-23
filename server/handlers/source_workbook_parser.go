package handlers

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"dootask-kpi-server/models"
	"github.com/xuri/excelize/v2"
)

const sourceWorkbookParserVersion = "source-workbook-v3"

// The persisted parser output is the import input. A later import must never
// substitute the bootstrap catalog for the workbook the user validated.
type sourceWorkbookMetadata struct {
	ParserVersion     string                            `json:"parser_version"`
	SourceFilename    string                            `json:"source_filename"`
	SourceSHA256      string                            `json:"source_sha256"`
	Size              int                               `json:"size"`
	Catalog           hongxingCatalog                   `json:"catalog"`
	Sheets            []sourceSheetPreview              `json:"sheets"`
	DepartmentOrigins map[string]sourceDepartmentOrigin `json:"department_origins"`
	FormulaCells      map[string][]models.FormulaCell   `json:"formula_cells"`
}

type sourceDepartmentOrigin struct {
	RawValue      string `json:"raw_value"`
	Value         string `json:"value"`
	LabelCell     string `json:"label_cell"`
	SourceCell    string `json:"source_cell"`
	MappingReason string `json:"mapping_reason"`
}

type parsedSourceWorkbook struct {
	Catalog      hongxingCatalog
	Metadata     sourceWorkbookMetadata
	Summary      sourceWorkbookSummary
	Sheets       []sourceSheetPreview
	FormulaCells map[string][]models.FormulaCell
}

type sourceXMLFormula struct {
	Type        string `xml:"t,attr"`
	SharedIndex string `xml:"si,attr"`
	Ref         string `xml:"ref,attr"`
	Text        string `xml:",chardata"`
}

type sourceXMLCell struct {
	Coordinate string `xml:"r,attr"`
	Type       string `xml:"t,attr"`
	Value      string `xml:"v"`
	Inline     struct {
		Text string `xml:"t"`
		Runs []struct {
			Text string `xml:"t"`
		} `xml:"r"`
	} `xml:"is"`
	Formula *sourceXMLFormula `xml:"f"`
}

type sourceXMLSheet struct {
	Cells     map[string]sourceXMLCell
	MaxRow    int
	MaxColumn int
}

var sourceScorerAliases = map[string]string{
	"PM": "PM", "项目经理": "PM", "项目经理（PM）": "PM", "CTO": "CTO",
	"部门负责人": "DEPARTMENT_MANAGER", "部门负责人（E层）": "DEPARTMENT_MANAGER", "部门负责人(E层)": "DEPARTMENT_MANAGER",
	"L层Leader": "DEPARTMENT_MANAGER", "L 层 Leader": "DEPARTMENT_MANAGER",
	"产品经理": "PRODUCT_MANAGER", "产品经理（PD）": "PRODUCT_MANAGER", "产品经理(PD)": "PRODUCT_MANAGER",
}

var sourceDepartmentFallbacks = map[string]string{
	"01-ID工业设计": "ID 工业设计部", "02-结构部门": "结构部门", "03-电子部门": "电子部门",
	"04-软件应用开发": "软件应用开发部", "05-算法开发": "算法开发部", "06-运动控制": "运控部",
	"07-测试部门": "测试部门", "08-嵌入式开发": "嵌入式开发部", "09-产品部门": "产品部门",
}

var sourceDataSourcePattern = regexp.MustCompile(`(?s)数据来源[：:]?\s*(.*)$`)

func parseSourceWorkbook(data []byte, sha, name string) (parsedSourceWorkbook, error) {
	var parsed parsedSourceWorkbook
	sum := sha256.Sum256(data)
	actualSHA := hex.EncodeToString(sum[:])
	if sha != "" && !strings.EqualFold(sha, actualSHA) {
		return parsed, errors.New("工作簿内容与来源 SHA-256 不一致")
	}
	if len(data) == 0 || int64(len(data)) > maxSourceWorkbookSize {
		return parsed, errors.New("工作簿大小必须在10MB以内")
	}
	topology, err := sourceWorkbookXML(data)
	if err != nil {
		return parsed, err
	}
	book, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: 50 << 20, UnzipXMLSizeLimit: 10 << 20})
	if err != nil {
		return parsed, errors.New("工作簿解析失败")
	}
	defer book.Close()
	sheets := book.GetSheetList()
	if len(sheets) == 0 || len(sheets) > 100 {
		return parsed, errors.New("工作簿工作表数量无效")
	}
	catalog := hongxingCatalog{Version: "hx-v3-" + actualSHA[:58], SourceSHA256: actualSHA, SourceFilename: path.Base(strings.ReplaceAll(name, `\`, "/")), SourceSheetCount: len(sheets)}
	parsed.Metadata = sourceWorkbookMetadata{ParserVersion: sourceWorkbookParserVersion, SourceFilename: catalog.SourceFilename, SourceSHA256: actualSHA, Size: len(data), DepartmentOrigins: map[string]sourceDepartmentOrigin{}, FormulaCells: map[string][]models.FormulaCell{}}
	departments := map[string]bool{}
	profileKeys := map[string]string{}
	for order, sheet := range sheets {
		xmlSheet, exists := topology[sheet]
		if !exists {
			return parsed, fmt.Errorf("工作表“%s”缺少可读取的来源内容", sheet)
		}
		headers := sourceIndicatorHeaderRows(book, sheet, xmlSheet)
		kind := ""
		switch {
		case len(headers) > 0:
			kind = "position_template"
		case strings.Contains(sheet, "HR汇总") && strings.Contains(sheet, "月度"):
			kind = "hr_monthly"
		case strings.Contains(sheet, "HR汇总") && strings.Contains(sheet, "季度"):
			kind = "hr_quarterly"
		case strings.Contains(sheet, "流程"):
			kind = "process"
		default:
			return parsed, fmt.Errorf("无法识别工作表“%s”，需要流程、岗位考核指标或 HR 汇总工作表", sheet)
		}
		preview := sourceSheetPreview{Name: sheet, Kind: kind, Order: order, DiagnosticStatus: "ok"}
		parsed.Sheets = append(parsed.Sheets, preview)
		formulas, err := sourceFormulaCells(book, sheet, xmlSheet)
		if err != nil {
			return parsed, err
		}
		parsed.Metadata.FormulaCells[sheet] = formulas
		switch kind {
		case "process":
			parsed.Summary.ProcessSheets++
			flow, flowErr := sourceWorkflowFlow(book, sheet)
			if flowErr != nil {
				return parsed, flowErr
			}
			catalog.Flow = &flow
		case "position_template":
			if len(headers) != 1 {
				return parsed, fmt.Errorf("岗位工作表“%s”必须有一个考核指标表头", sheet)
			}
			profile, origin, err := sourcePositionProfile(book, sheet, xmlSheet, headers[0])
			if err != nil {
				return parsed, err
			}
			key := strings.ToLower(profile.Department + "/" + profile.Position + "/" + profile.Period)
			if previous, exists := profileKeys[key]; exists {
				return parsed, fmt.Errorf("工作表“%s”与“%s”包含重复的部门、岗位和周期模板", previous, sheet)
			}
			profileKeys[key] = sheet
			catalog.Profiles = append(catalog.Profiles, profile)
			parsed.Metadata.DepartmentOrigins[sheet] = origin
			departments[profile.Department] = true
			parsed.Summary.PositionTemplates++
			for _, dimension := range profile.Dimensions {
				parsed.Summary.Indicators += len(dimension.Items)
			}
		default:
			catalog.HRSummaryDiagnostics = append(catalog.HRSummaryDiagnostics, sourceHRDiagnostics(book, sheet, xmlSheet, kind == "hr_quarterly"))
			parsed.Summary.HRSummarySheets++
		}
	}
	if parsed.Summary.PositionTemplates == 0 {
		return parsed, errors.New("未识别到岗位模板工作表")
	}
	for department := range departments {
		catalog.SourceDepartments = append(catalog.SourceDepartments, department)
	}
	sort.Strings(catalog.SourceDepartments)
	catalog.SourceDepartmentCount = len(catalog.SourceDepartments)
	catalog.SourceTemplateCount = len(catalog.Profiles)
	parsed.Summary.SourceDepartments = catalog.SourceDepartmentCount
	parsed.Catalog = catalog
	parsed.FormulaCells = parsed.Metadata.FormulaCells
	parsed.Metadata.Catalog = catalog
	parsed.Metadata.Sheets = parsed.Sheets
	return parsed, nil
}

func sourceWorkflowFlow(book *excelize.File, sheet string) (hongxingFlow, error) {
	cellValue := func(coordinate string) string {
		value, _ := book.GetCellValue(sheet, coordinate)
		return strings.TrimSpace(value)
	}
	flow := hongxingFlow{Sheet: sheet, Scope: cellValue("A2")}
	for row := 6; row <= 13; row++ {
		stepValue := cellValue(fmt.Sprintf("A%d", row))
		if stepValue == "" {
			continue
		}
		step, err := strconv.Atoi(stepValue)
		if err != nil {
			continue
		}
		flow.Steps = append(flow.Steps, hongxingFlowStep{
			Step: step, Name: cellValue(fmt.Sprintf("B%d", row)),
			Responsibility: cellValue(fmt.Sprintf("D%d", row)),
			Requirements:   cellValue(fmt.Sprintf("E%d", row)),
			Deadline:       cellValue(fmt.Sprintf("I%d", row)),
			SourceRow:      row,
		})
	}
	return flow, nil
}

func sourceCatalogFromMetadata(raw string) (hongxingCatalog, error) {
	var metadata sourceWorkbookMetadata
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return hongxingCatalog{}, errors.New("工作簿验证快照无法读取，请重新上传验证")
	}
	catalog := metadata.Catalog
	if metadata.ParserVersion != sourceWorkbookParserVersion || len(catalog.SourceSHA256) != 64 || catalog.SourceSHA256 != metadata.SourceSHA256 || len(catalog.Profiles) == 0 || catalog.SourceTemplateCount != len(catalog.Profiles) || catalog.SourceSheetCount != len(metadata.Sheets) || catalog.SourceDepartmentCount != len(catalog.SourceDepartments) {
		return hongxingCatalog{}, errors.New("工作簿缺少完整的已验证来源快照，请重新上传验证")
	}
	return catalog, nil
}

// Read XML topology without evaluating any formula or following external links.
// Explicit stored cells distinguish merged placeholders from actual values.
func sourceWorkbookXML(data []byte) (map[string]sourceXMLSheet, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("工作簿不是有效的 XLSX 文件")
	}
	files := map[string]*zip.File{}
	var expanded uint64
	if len(archive.File) > 1000 {
		return nil, errors.New("工作簿内部文件数量超过1000")
	}
	for _, file := range archive.File {
		if file.UncompressedSize64 > (50<<20)-expanded {
			return nil, errors.New("工作簿解压内容超过50MB")
		}
		expanded += file.UncompressedSize64
		if _, exists := files[file.Name]; exists {
			return nil, errors.New("工作簿包含重复的内部来源文件")
		}
		files[file.Name] = file
	}
	readXML := func(filename string, target any) error {
		file := files[filename]
		if file == nil || file.UncompressedSize64 > 10<<20 {
			return errors.New("工作簿 XML 来源缺失或过大")
		}
		reader, err := file.Open()
		if err != nil {
			return err
		}
		defer reader.Close()
		return xml.NewDecoder(io.LimitReader(reader, 10<<20)).Decode(target)
	}
	var workbook struct {
		Sheets []struct {
			Name         string `xml:"name,attr"`
			Relationship string `xml:"id,attr"`
		} `xml:"sheets>sheet"`
	}
	var relationships struct {
		Items []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := readXML("xl/workbook.xml", &workbook); err != nil {
		return nil, errors.New("工作簿目录无法解析")
	}
	if len(workbook.Sheets) == 0 || len(workbook.Sheets) > 100 {
		return nil, errors.New("工作簿工作表数量无效")
	}
	if err := readXML("xl/_rels/workbook.xml.rels", &relationships); err != nil {
		return nil, errors.New("工作簿关联无法解析")
	}
	targets := map[string]string{}
	for _, relationship := range relationships.Items {
		target := relationship.Target
		if strings.HasPrefix(target, "/") {
			target = strings.TrimPrefix(target, "/")
		} else {
			target = path.Join("xl", target)
		}
		targets[relationship.ID] = path.Clean(target)
	}
	result := map[string]sourceXMLSheet{}
	for _, sheet := range workbook.Sheets {
		var worksheet struct {
			Rows []struct {
				Cells []sourceXMLCell `xml:"c"`
			} `xml:"sheetData>row"`
		}
		if err := readXML(targets[sheet.Relationship], &worksheet); err != nil {
			return nil, fmt.Errorf("工作表“%s”XML 无法解析", sheet.Name)
		}
		parsed := sourceXMLSheet{Cells: map[string]sourceXMLCell{}}
		for _, row := range worksheet.Rows {
			for _, cell := range row.Cells {
				column, rowNumber, err := excelize.CellNameToCoordinates(cell.Coordinate)
				if err != nil || rowNumber > 5000 || column > 64 {
					return nil, fmt.Errorf("工作表“%s”超出支持的5000行、64列范围", sheet.Name)
				}
				if rowNumber > parsed.MaxRow {
					parsed.MaxRow = rowNumber
				}
				if column > parsed.MaxColumn {
					parsed.MaxColumn = column
				}
				if _, exists := parsed.Cells[cell.Coordinate]; exists {
					return nil, fmt.Errorf("工作表“%s”包含重复的来源单元格%s", sheet.Name, cell.Coordinate)
				}
				parsed.Cells[cell.Coordinate] = cell
			}
		}
		result[sheet.Name] = parsed
	}
	return result, nil
}

func sourceStoredValue(book *excelize.File, sheet, coordinate string, topology sourceXMLSheet) string {
	cell, ok := topology.Cells[coordinate]
	if !ok || cell.Formula != nil {
		return ""
	}
	if cell.Value == "" && cell.Inline.Text == "" && len(cell.Inline.Runs) == 0 {
		return ""
	}
	value, _ := book.GetCellValue(sheet, coordinate, excelize.Options{RawCellValue: true})
	return value
}

func sourceIndicatorHeaderRows(book *excelize.File, sheet string, topology sourceXMLSheet) []int {
	rows := []int{}
	for coordinate := range topology.Cells {
		if strings.TrimSpace(sourceStoredValue(book, sheet, coordinate, topology)) == "考核指标" {
			_, row, _ := excelize.CellNameToCoordinates(coordinate)
			rows = append(rows, row)
		}
	}
	sort.Ints(rows)
	return rows
}

func sourcePositionProfile(book *excelize.File, sheet string, topology sourceXMLSheet, header int) (hongxingProfile, sourceDepartmentOrigin, error) {
	profile := hongxingProfile{Sheet: sheet, Period: "monthly"}
	if strings.HasSuffix(sheet, "部门负责人") {
		profile.Position = "部门负责人"
	} else if strings.HasSuffix(sheet, "工程师") {
		profile.Position = "工程师"
	}
	if profile.Position == "" {
		return profile, sourceDepartmentOrigin{}, fmt.Errorf("岗位工作表“%s”缺少可识别的部门负责人或工程师岗位", sheet)
	}
	origin := sourceDepartmentOrigin{}
	for row := 1; row < header; row++ {
		for col := 1; col <= topology.MaxColumn; col++ {
			coordinate, _ := excelize.CoordinatesToCellName(col, row)
			if strings.TrimSpace(sourceStoredValue(book, sheet, coordinate, topology)) != "部门" {
				continue
			}
			origin.LabelCell = coordinate
			for valueCol := col + 1; valueCol <= topology.MaxColumn; valueCol++ {
				valueCell, _ := excelize.CoordinatesToCellName(valueCol, row)
				if value := strings.TrimSpace(sourceStoredValue(book, sheet, valueCell, topology)); value != "" {
					origin.RawValue = value
					origin.Value = value
					origin.SourceCell = valueCell
					break
				}
			}
			break
		}
		if origin.LabelCell != "" {
			break
		}
	}
	if origin.Value == "" {
		base := strings.TrimSuffix(sheet, "-"+profile.Position)
		origin.Value = sourceDepartmentFallbacks[base]
		origin.MappingReason = "controlled_position_group"
	} else {
		origin.MappingReason = "source_label"
	}
	if origin.Value == "" {
		return profile, origin, fmt.Errorf("岗位工作表“%s”部门为空且没有受控映射", sheet)
	}
	profile.Department = origin.Value
	dimensionRows := []int{}
	itemOrder := 0
	for row := header + 1; row <= topology.MaxRow; row++ {
		label := sourceStoredValue(book, sheet, fmt.Sprintf("A%d", row), topology)
		if strings.HasPrefix(label, "综合权重合计") || strings.HasPrefix(label, "总分") || strings.HasPrefix(label, "绩效等级") {
			break
		}
		name := strings.TrimSpace(sourceStoredValue(book, sheet, fmt.Sprintf("D%d", row), topology))
		weightText := strings.TrimSpace(sourceStoredValue(book, sheet, fmt.Sprintf("E%d", row), topology))
		if name == "" && weightText == "" {
			continue
		}
		weight, err := strconv.ParseFloat(weightText, 64)
		if name == "" || err != nil || !validWeight(weight) {
			return profile, origin, fmt.Errorf("工作表“%s”D%d/E%d 指标名称或权重无效", sheet, row, row)
		}
		dimensionName := strings.TrimSpace(sourceStoredValue(book, sheet, fmt.Sprintf("B%d", row), topology))
		if dimensionName != "" {
			dimensionWeight, err := strconv.ParseFloat(sourceStoredValue(book, sheet, fmt.Sprintf("C%d", row), topology), 64)
			if err != nil || !validWeight(dimensionWeight) {
				return profile, origin, fmt.Errorf("工作表“%s”C%d 维度权重无效", sheet, row)
			}
			profile.Dimensions = append(profile.Dimensions, hongxingDimension{Name: dimensionName, Weight: dimensionWeight, SortOrder: len(profile.Dimensions)})
			dimensionRows = append(dimensionRows, row)
		}
		if len(profile.Dimensions) == 0 {
			return profile, origin, fmt.Errorf("工作表“%s”第%d行指标没有所属维度", sheet, row)
		}
		index := len(profile.Dimensions) - 1
		formula, _ := book.GetCellFormula(sheet, fmt.Sprintf("F%d", row))
		expected := fmt.Sprintf("ROUND($C$%d*E%d/100,1)", dimensionRows[index], row)
		if normalizeSourceFormula(formula) != normalizeSourceFormula(expected) {
			profile.SourceAnomalies = append(profile.SourceAnomalies, map[string]any{"code": "composite_formula_mismatch", "source_cell": fmt.Sprintf("F%d", row), "actual": formula, "expected": "=" + expected})
		}
		contribution, _ := book.GetCellFormula(sheet, fmt.Sprintf("I%d", row))
		expectedContribution := fmt.Sprintf(`IF(H%d="","",ROUND(F%d*H%d/100,2))`, row, row, row)
		if normalizeSourceFormula(contribution) != normalizeSourceFormula(expectedContribution) {
			profile.SourceAnomalies = append(profile.SourceAnomalies, map[string]any{"code": "contribution_formula_mismatch", "source_cell": fmt.Sprintf("I%d", row), "actual": contribution, "expected": "=" + expectedContribution})
		}
		scorerLabel := strings.TrimSpace(sourceStoredValue(book, sheet, fmt.Sprintf("G%d", row), topology))
		scorer := sourceScorerAliases[scorerLabel]
		if scorer == "" {
			scorer = "UNKNOWN"
			profile.SourceAnomalies = append(profile.SourceAnomalies, map[string]any{"code": "unknown_scorer", "source_cell": fmt.Sprintf("G%d", row), "value": scorerLabel})
		}
		guidance := sourceStoredValue(book, sheet, fmt.Sprintf("J%d", row), topology)
		dataSource := ""
		if match := sourceDataSourcePattern.FindStringSubmatch(guidance); len(match) > 1 {
			dataSource = strings.TrimSpace(match[1])
		}
		item := hongxingItem{templateItemInput: templateItemInput{Name: name, ItemWeight: weight, ScorerTypes: []string{scorer}, Required: true, ScoringGuidance: guidance, DataSource: dataSource, Order: itemOrder}, SourceRow: row, SourceScorerLabel: scorerLabel}
		profile.Dimensions[index].Items = append(profile.Dimensions[index].Items, item)
		itemOrder++
	}
	if itemOrder == 0 {
		return profile, origin, fmt.Errorf("岗位工作表“%s”没有有效考核指标", sheet)
	}
	for _, dimension := range profile.Dimensions {
		var total float64
		cells := []any{}
		for _, item := range dimension.Items {
			total += item.ItemWeight
			cells = append(cells, fmt.Sprintf("E%d", item.SourceRow))
		}
		if !approximately100(total) {
			profile.SourceAnomalies = append(profile.SourceAnomalies, map[string]any{"code": "dimension_item_weight_sum", "dimension": dimension.Name, "weight_sum": total, "cells": cells})
		}
	}
	return profile, origin, nil
}

func normalizeSourceFormula(formula string) string {
	var normalized strings.Builder
	inString := false
	for _, character := range strings.TrimPrefix(strings.TrimSpace(formula), "=") {
		if character == '"' {
			inString = !inString
			normalized.WriteRune(character)
		} else if inString {
			normalized.WriteRune(character)
		} else if !unicode.IsSpace(character) {
			normalized.WriteRune(unicode.ToUpper(character))
		}
	}
	return normalized.String()
}

func sourceHRDiagnostics(book *excelize.File, sheet string, topology sourceXMLSheet, quarterly bool) hongxingHRDiagnostic {
	result := hongxingHRDiagnostic{Sheet: sheet, Kind: "monthly"}
	end, weightRow, annualCol, lastPeriod := 33, 38, "P", "O"
	if quarterly {
		result.Kind = "quarterly"
		end, weightRow, annualCol, lastPeriod = 34, 39, "K", "G"
	}
	for row := 5; row <= end; row++ {
		number := sourceStoredValue(book, sheet, fmt.Sprintf("A%d", row), topology)
		real := false
		for col := 2; col <= 8; col++ {
			coordinate, _ := excelize.CoordinatesToCellName(col, row)
			if strings.TrimSpace(sourceStoredValue(book, sheet, coordinate, topology)) != "" {
				real = true
			}
		}
		if strings.TrimSpace(number) == "" && !real {
			continue
		}
		for _, col := range []string{"I", "J"} {
			coordinate := fmt.Sprintf("%s%d", col, row)
			formula, _ := book.GetCellFormula(sheet, coordinate)
			if formula == "" {
				result.FormulaGaps = append(result.FormulaGaps, hongxingFormulaGap{Cell: coordinate, Row: row, Reason: "detail_row_formula_missing", IsRealDataRow: real})
			}
		}
	}
	for row := 40; row < 50; row++ {
		coordinate := fmt.Sprintf("%s%d", annualCol, row)
		formula, _ := book.GetCellFormula(sheet, coordinate)
		if formula == "" {
			continue
		}
		weights := fmt.Sprintf("$D$%d:$%s$%d", weightRow, lastPeriod, weightRow)
		values := fmt.Sprintf("D%d:%s%d", row, lastPeriod, row)
		if !strings.Contains(formula, weights) || !strings.Contains(formula, values) {
			result.AnnualFormulaIssues = append(result.AnnualFormulaIssues, hongxingAnnualFormulaIssue{Cell: coordinate, Formula: "=" + strings.TrimPrefix(formula, "=")})
		}
	}
	return result
}

func sourceFormulaCells(book *excelize.File, sheet string, topology sourceXMLSheet) ([]models.FormulaCell, error) {
	masters := map[string]sourceXMLCell{}
	for _, cell := range topology.Cells {
		if cell.Formula != nil && cell.Formula.Type == "shared" && cell.Formula.Text != "" {
			masters[cell.Formula.SharedIndex] = cell
		}
	}
	coordinates := []string{}
	for coordinate, cell := range topology.Cells {
		if cell.Formula != nil {
			coordinates = append(coordinates, coordinate)
		}
	}
	sort.Strings(coordinates)
	result := make([]models.FormulaCell, 0, len(coordinates))
	for _, coordinate := range coordinates {
		cell := topology.Cells[coordinate]
		formula := cell.Formula
		expanded, err := book.GetCellFormula(sheet, coordinate)
		if err != nil {
			return nil, fmt.Errorf("工作表“%s”%s 共享公式无法展开", sheet, coordinate)
		}
		kind := formula.Type
		if kind == "" {
			kind = "normal"
		}
		record := models.FormulaCell{Coordinate: coordinate, FormulaType: kind, RawFormula: formula.Text, ExpandedFormula: expanded, CachedValue: cell.Value, DiagnosticStatus: "ok"}
		if record.RawFormula != "" {
			record.RawFormula = "=" + strings.TrimPrefix(record.RawFormula, "=")
		}
		if record.ExpandedFormula != "" {
			record.ExpandedFormula = "=" + strings.TrimPrefix(record.ExpandedFormula, "=")
		}
		if kind == "shared" {
			master, exists := masters[formula.SharedIndex]
			if !exists || expanded == "" {
				return nil, fmt.Errorf("工作表“%s”%s 共享公式缺少主公式", sheet, coordinate)
			}
			record.SharedIndex = formula.SharedIndex
			record.SharedMasterCoordinate = master.Coordinate
			record.SharedRef = master.Formula.Ref
		}
		result = append(result, record)
	}
	return result, nil
}
