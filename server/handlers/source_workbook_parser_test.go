package handlers

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func sourceParserTestWorkbook(t *testing.T, mutate func(*excelize.File)) []byte {
	t.Helper()
	book := excelize.NewFile()
	t.Cleanup(func() { book.Close() })
	book.SetSheetName("Sheet1", "项目绩效考核流程")
	book.SetCellValue("项目绩效考核流程", "A1", "项目绩效考核流程")
	book.SetCellValue("项目绩效考核流程", "A2", "流程适用范围")
	for step := 1; step <= 8; step++ {
		row := step + 5
		book.SetCellValue("项目绩效考核流程", fmt.Sprintf("A%d", row), step)
		book.SetCellValue("项目绩效考核流程", fmt.Sprintf("B%d", row), fmt.Sprintf("流程步骤%d", step))
		book.SetCellValue("项目绩效考核流程", fmt.Sprintf("D%d", row), "责任人")
		book.SetCellValue("项目绩效考核流程", fmt.Sprintf("E%d", row), "执行要求")
		book.SetCellValue("项目绩效考核流程", fmt.Sprintf("I%d", row), "1个工作日")
	}
	sheet := "01-ID工业设计-工程师"
	book.NewSheet(sheet)
	for cell, value := range map[string]any{"F3": "部门", "H3": "真实来源部门", "D11": "考核指标", "B12": "交付质量", "C12": 100, "D12": "来源指标甲", "D13": "来源指标乙", "E12": 60, "E13": 40, "G12": "PM", "G13": "产品经理", "J12": "按实际完成情况评分 数据来源：项目计划", "A14": "综合权重合计"} {
		if err := book.SetCellValue(sheet, cell, value); err != nil {
			t.Fatal(err)
		}
	}
	book.SetCellFormula(sheet, "F12", "ROUND($C$12*E12/100,1)")
	book.SetCellFormula(sheet, "F13", "ROUND($C$12*E13/100,1)")
	book.SetCellFormula(sheet, "I12", `IF(H12="","",ROUND(F12*H12/100,2))`)
	book.SetCellFormula(sheet, "I13", `IF(H13="","",ROUND(F13*H13/100,2))`)
	book.MergeCell(sheet, "B12", "B13")
	book.MergeCell(sheet, "C12", "C13")
	for _, hr := range []string{"HR汇总（月度）", "HR汇总（季度）"} {
		book.NewSheet(hr)
		book.SetCellValue(hr, "A5", 1)
		book.SetCellValue(hr, "A6", 2)
		book.SetCellValue(hr, "A17", 13)
		formulaType, ref := excelize.STCellFormulaTypeShared, "I5:I6"
		book.SetCellFormula(hr, "I5", `IF(H5="","",H5*G5)`, excelize.FormulaOpts{Type: &formulaType, Ref: &ref})
		ref = "J5:J6"
		book.SetCellFormula(hr, "J5", `IF(I5="","",IF(I5>=90,"S","A"))`, excelize.FormulaOpts{Type: &formulaType, Ref: &ref})
	}
	book.SetCellFormula("HR汇总（季度）", "K40", `IFERROR(SUMPRODUCT($D$39:$J$39,D40:J40),"")`)
	if mutate != nil {
		mutate(book)
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func sourceParserSHA(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestParseSourceWorkbookReadsActualRevisedCellsAndPersistsImportSnapshot(t *testing.T) {
	data := sourceParserTestWorkbook(t, func(book *excelize.File) {
		book.SetCellValue("01-ID工业设计-工程师", "D12", "修订后的实际指标")
		book.SetCellValue("01-ID工业设计-工程师", "E12", 55)
		book.SetCellValue("01-ID工业设计-工程师", "E13", 45)
	})
	sha := sourceParserSHA(data)
	parsed, err := parseSourceWorkbook(data, sha, "修订绩效规则.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Catalog.SourceSHA256 != sha || parsed.Catalog.SourceFilename != "修订绩效规则.xlsx" || parsed.Catalog.Version != "hx-v3-"+sha[:58] {
		t.Fatalf("source identity lost: %+v", parsed.Catalog)
	}
	if parsed.Summary.ProcessSheets != 1 || parsed.Summary.PositionTemplates != 1 || parsed.Summary.HRSummarySheets != 2 || parsed.Summary.Indicators != 2 || parsed.Summary.SourceDepartments != 1 {
		t.Fatalf("summary=%+v", parsed.Summary)
	}
	if parsed.Catalog.Flow == nil || len(parsed.Catalog.Flow.Steps) != 8 {
		t.Fatalf("workflow steps were not parsed: %+v", parsed.Catalog.Flow)
	}
	profile := parsed.Catalog.Profiles[0]
	if profile.Department != "真实来源部门" || len(profile.Dimensions) != 1 || len(profile.Dimensions[0].Items) != 2 {
		t.Fatalf("profile or merged dimensions wrong: %+v", profile)
	}
	first := profile.Dimensions[0].Items[0]
	if first.Name != "修订后的实际指标" || first.ItemWeight != 55 || first.SourceRow != 12 || first.DataSource != "项目计划" || first.ScorerTypes[0] != "PM" {
		t.Fatalf("actual source fields ignored: %+v", first)
	}
	if len(profile.SourceAnomalies) != 0 {
		t.Fatalf("valid revised rules incorrectly diagnosed: %+v", profile.SourceAnomalies)
	}
	metadata, err := json.Marshal(parsed.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := sourceCatalogFromMetadata(string(metadata))
	if err != nil {
		t.Fatal(err)
	}
	if restored.SourceSHA256 != sha || restored.Profiles[0].Dimensions[0].Items[0].Name != first.Name || restored.Profiles[0].Dimensions[0].Items[0].ItemWeight != 55 {
		t.Fatal("import snapshot substituted a different source catalog")
	}
	if _, err := sourceCatalogFromMetadata(`{"source_filename":"old-only-name.xlsx"}`); err == nil {
		t.Fatal("metadata without immutable catalog accepted")
	}
	if _, err := parseSourceWorkbook(data, strings.Repeat("f", 64), "wrong-sha.xlsx"); err == nil {
		t.Fatal("mismatched content digest accepted")
	}
}

func TestParseSourceWorkbookSharedFormulaAuditAndEmptyVersusPopulatedHRRows(t *testing.T) {
	data := sourceParserTestWorkbook(t, func(book *excelize.File) { book.SetCellValue("HR汇总（月度）", "B17", "实际员工") })
	parsed, err := parseSourceWorkbook(data, sourceParserSHA(data), "来源.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	for _, summary := range parsed.Catalog.HRSummaryDiagnostics {
		if len(summary.FormulaGaps) != 2 {
			t.Fatalf("summary=%+v", summary)
		}
		for _, gap := range summary.FormulaGaps {
			if gap.IsRealDataRow != (summary.Kind == "monthly") || gap.Row != 17 {
				t.Fatalf("numbered placeholder mistaken for business input: %+v", gap)
			}
		}
	}
	if len(parsed.Catalog.HRSummaryDiagnostics[1].AnnualFormulaIssues) != 1 {
		t.Fatal("quarterly D:J scope issue missing")
	}
	found := false
	for _, formula := range parsed.FormulaCells["HR汇总（月度）"] {
		if formula.Coordinate == "I6" {
			found = true
			if formula.FormulaType != "shared" || formula.SharedMasterCoordinate != "I5" || formula.SharedRef != "I5:I6" || formula.RawFormula != "" || formula.ExpandedFormula != `=IF(H6="","",H6*G6)` {
				t.Fatalf("shared follower audit lost: %+v", formula)
			}
		}
	}
	if !found {
		t.Fatal("shared follower formula not retained")
	}
}

func TestParseSourceWorkbookRejectsMalformedWeightsUnknownSheetsAndMissingMapping(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*excelize.File)
		message string
	}{
		{"weight", func(book *excelize.File) { book.SetCellValue("01-ID工业设计-工程师", "E12", "待填写") }, "权重无效"},
		{"unknown_sheet", func(book *excelize.File) { book.NewSheet("无法解释的附加表") }, "无法识别工作表"},
		{"duplicate_profile", func(book *excelize.File) {
			index, _ := book.GetSheetIndex("01-ID工业设计-工程师")
			duplicate, _ := book.NewSheet("另一份来源-工程师")
			book.CopySheet(index, duplicate)
		}, "重复的部门、岗位和周期模板"},
		{"missing_mapping", func(book *excelize.File) {
			book.SetCellValue("01-ID工业设计-工程师", "H3", "")
			book.SetSheetName("01-ID工业设计-工程师", "自定义部门-工程师")
		}, "没有受控映射"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := sourceParserTestWorkbook(t, tc.mutate)
			_, err := parseSourceWorkbook(data, sourceParserSHA(data), "无效来源.xlsx")
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("error=%v want=%s", err, tc.message)
			}
		})
	}
}

func TestParseSourceWorkbookControlledMissingDepartmentAndFormulaDiagnostics(t *testing.T) {
	data := sourceParserTestWorkbook(t, func(book *excelize.File) {
		book.SetCellValue("01-ID工业设计-工程师", "H3", "")
		book.SetCellFormula("01-ID工业设计-工程师", "F12", "HYPERLINK(\"https://example.invalid\",\"bad\")")
		book.SetCellValue("01-ID工业设计-工程师", "G13", "未知评分职责")
	})
	parsed, err := parseSourceWorkbook(data, sourceParserSHA(data), "来源.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	origin := parsed.Metadata.DepartmentOrigins["01-ID工业设计-工程师"]
	if origin.RawValue != "" || origin.Value != "ID 工业设计部" || origin.MappingReason != "controlled_position_group" {
		t.Fatalf("mapping provenance=%+v", origin)
	}
	issues := parsed.Catalog.Profiles[0].SourceAnomalies
	if len(issues) != 2 || issues[0]["code"] != "composite_formula_mismatch" || issues[1]["code"] != "unknown_scorer" {
		t.Fatalf("formula/scorer diagnoses=%+v", issues)
	}
}

func TestParseSourceWorkbookBlocksUnsupportedContributionFormula(t *testing.T) {
	for _, formula := range []string{"F12*H12/50", `IF(H12=" ","",ROUND(F12*H12/100,2))`} {
		data := sourceParserTestWorkbook(t, func(book *excelize.File) {
			book.SetCellFormula("01-ID工业设计-工程师", "I12", formula)
		})
		parsed, err := parseSourceWorkbook(data, sourceParserSHA(data), "变更计算规则.xlsx")
		if err != nil {
			t.Fatal(err)
		}
		issues, blocked := profileSourceIssues(parsed.Catalog.Profiles[0])
		if !blocked || len(issues) != 1 || issues[0]["code"] != "contribution_formula_mismatch" || issues[0]["source_cell"] != "I12" {
			t.Fatalf("unsupported contribution formula silently accepted: %+v", issues)
		}
	}
}

func TestParseSourceWorkbookRejectsOversizedGridAndAmbiguousZipEntries(t *testing.T) {
	data := sourceParserTestWorkbook(t, func(book *excelize.File) {
		book.SetCellValue("01-ID工业设计-工程师", "A5001", "超出支持范围")
	})
	if _, err := parseSourceWorkbook(data, sourceParserSHA(data), "过大来源.xlsx"); err == nil || !strings.Contains(err.Error(), "5000行") {
		t.Fatalf("unbounded worksheet grid accepted: %v", err)
	}
	valid := sourceParserTestWorkbook(t, nil)
	archive, err := zip.NewReader(bytes.NewReader(valid), int64(len(valid)))
	if err != nil {
		t.Fatal(err)
	}
	var duplicated bytes.Buffer
	writer := zip.NewWriter(&duplicated)
	for _, entry := range archive.File {
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		copies := 1
		if entry.Name == "xl/workbook.xml" {
			copies = 2
		}
		for copy := 0; copy < copies; copy++ {
			target, err := writer.Create(entry.Name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := target.Write(content); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := parseSourceWorkbook(duplicated.Bytes(), sourceParserSHA(duplicated.Bytes()), "重复来源.xlsx"); err == nil || !strings.Contains(err.Error(), "重复的内部来源文件") {
		t.Fatalf("ambiguous duplicate ZIP source accepted: %v", err)
	}
}

func TestParseSourceWorkbookOriginalSourceMatchesCatalogOracle(t *testing.T) {
	filename := os.Getenv("KPI_SOURCE_WORKBOOK_TEST")
	if filename == "" {
		t.Skip("set KPI_SOURCE_WORKBOOK_TEST to run the original source workbook oracle")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseSourceWorkbook(data, sourceParserSHA(data), "荭杏研发项目绩效考核表 (1).xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Catalog.SourceSheetCount != 21 || parsed.Summary.PositionTemplates != 18 || parsed.Summary.Indicators != 137 || parsed.Summary.SourceDepartments != 9 {
		t.Fatalf("source counts=%+v", parsed.Summary)
	}
	var oracle hongxingCatalog
	if err := json.Unmarshal(hongxingCatalogJSON, &oracle); err != nil {
		t.Fatal(err)
	}
	for index, expected := range oracle.Profiles {
		actual := parsed.Catalog.Profiles[index]
		if actual.Sheet != expected.Sheet || actual.Department != expected.Department || actual.Position != expected.Position || len(actual.Dimensions) != len(expected.Dimensions) {
			t.Fatalf("source profile %d differs from Python oracle", index)
		}
		for dimensionIndex, dimension := range expected.Dimensions {
			actualDimension := actual.Dimensions[dimensionIndex]
			if actualDimension.Name != dimension.Name || actualDimension.Weight != dimension.Weight || len(actualDimension.Items) != len(dimension.Items) {
				t.Fatalf("dimension %s/%s differs", actual.Sheet, dimension.Name)
			}
			for itemIndex, item := range dimension.Items {
				actualItem := actualDimension.Items[itemIndex]
				if actualItem.Name != item.Name || actualItem.ItemWeight != item.ItemWeight || actualItem.SourceRow != item.SourceRow || actualItem.SourceScorerLabel != item.SourceScorerLabel || actualItem.ScoringGuidance != item.ScoringGuidance || actualItem.DataSource != item.DataSource || fmt.Sprint(actualItem.ScorerTypes) != fmt.Sprint(item.ScorerTypes) {
					t.Fatalf("indicator differs %s row=%d actual=%+v expected=%+v", actual.Sheet, item.SourceRow, actualItem, item)
				}
			}
		}
	}
}
