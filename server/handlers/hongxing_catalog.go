package handlers

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

//go:embed data/hongxing-catalog-v2.json
var hongxingCatalogJSON []byte

// Legacy source retained for compatibility and diagnostics; new seeding uses v2.
//
//go:embed data/hongxing-catalog-v1.json
var hongxingCatalogV1JSON []byte

type hongxingCatalog struct {
	Version               string                 `json:"version"`
	SourceSHA256          string                 `json:"source_sha256"`
	SourceFilename        string                 `json:"source_filename"`
	SourceSheetCount      int                    `json:"source_sheet_count"`
	SourceTemplateCount   int                    `json:"source_template_count"`
	SourceDepartmentCount int                    `json:"source_department_count"`
	SourceDepartments     []string               `json:"source_departments"`
	Profiles              []hongxingProfile      `json:"profiles"`
	HRSummaryDiagnostics  []hongxingHRDiagnostic `json:"hr_summary_diagnostics"`
	Flow                  *hongxingFlow          `json:"flow,omitempty"`
}

type hongxingFlow struct {
	Sheet string             `json:"sheet"`
	Scope string             `json:"scope"`
	Steps []hongxingFlowStep `json:"steps"`
}

type hongxingFlowStep struct {
	Step           int    `json:"step"`
	Name           string `json:"name"`
	Responsibility string `json:"responsibility"`
	Requirements   string `json:"requirements"`
	Deadline       string `json:"deadline"`
	SourceRow      int    `json:"source_row"`
}

type hongxingHRDiagnostic struct {
	Sheet               string                       `json:"sheet"`
	Kind                string                       `json:"kind"`
	FormulaGaps         []hongxingFormulaGap         `json:"formula_gaps"`
	AnnualFormulaIssues []hongxingAnnualFormulaIssue `json:"annual_formula_issues"`
}

type hongxingFormulaGap struct {
	Cell          string `json:"cell"`
	Row           int    `json:"row"`
	Reason        string `json:"reason"`
	IsRealDataRow bool   `json:"is_real_data_row"`
	Severity      string `json:"severity,omitempty"`
}

type hongxingAnnualFormulaIssue struct {
	Cell    string `json:"cell"`
	Formula string `json:"formula"`
}

type hongxingProfile struct {
	Sheet           string              `json:"sheet"`
	Department      string              `json:"department"`
	Position        string              `json:"position"`
	Period          string              `json:"period"`
	Dimensions      []hongxingDimension `json:"dimensions"`
	SourceAnomalies []map[string]any    `json:"source_anomalies"`
}

type hongxingDimension struct {
	Name      string         `json:"name"`
	Weight    float64        `json:"weight"`
	SortOrder int            `json:"sort_order"`
	Items     []hongxingItem `json:"items"`
}

type hongxingItem struct {
	templateItemInput
	SourceRow         int    `json:"source_row"`
	SourceScorerLabel string `json:"source_scorer_label"`
}

func SeedHongxingCatalog(db *gorm.DB) error {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("SEED_HONGXING_CATALOG")))
	if mode == "false" {
		return nil
	}
	if mode != "" && mode != "true" {
		return errors.New("SEED_HONGXING_CATALOG 必须为 true 或 false")
	}
	var catalog hongxingCatalog
	if err := json.Unmarshal(hongxingCatalogJSON, &catalog); err != nil {
		return fmt.Errorf("读取荭杏业务目录失败: %w", err)
	}
	if err := seedHongxingCatalog(db, catalog); err != nil {
		return err
	}
	if err := seedHongxingHierarchy(db); err != nil {
		return err
	}
	return repairDepartmentLeaderManagers(db)
}

func seedHongxingCatalog(db *gorm.DB, catalog hongxingCatalog) error {
	if catalog.Version != "hongxing-catalog-v2" || len(catalog.Profiles) != 18 || catalog.SourceSheetCount != 21 || catalog.SourceTemplateCount != 18 || catalog.SourceDepartmentCount != 9 || len(catalog.SourceDepartments) != 9 || len(catalog.SourceSHA256) != 64 {
		return errors.New("荭杏业务目录版本或来源不完整")
	}
	itemCount := 0
	departments := make(map[string]struct{}, len(catalog.SourceDepartments))
	for _, department := range catalog.SourceDepartments {
		if strings.TrimSpace(department) == "" {
			return errors.New("荭杏业务目录来源部门不能为空")
		}
		departments[department] = struct{}{}
	}
	if len(departments) != 9 {
		return errors.New("荭杏业务目录来源部门数量不一致")
	}
	for _, profile := range catalog.Profiles {
		for _, dimension := range profile.Dimensions {
			itemCount += len(dimension.Items)
		}
	}
	if itemCount != 137 {
		return fmt.Errorf("荭杏业务目录指标数量不一致: %d", itemCount)
	}
	return importHongxingCatalog(db, catalog, "publish", true)
}

var errSourcePublishConflict = errors.New("source template publication conflict")

// Local dimension sums are source warnings under the selected workbook rule.
// Only the rounded composite total and genuinely blocking source issues prevent
// publication. Keep every issue and its original cells as provenance.
func profileSourceIssues(profile hongxingProfile) ([]map[string]any, bool) {
	issues := make([]map[string]any, 0, len(profile.SourceAnomalies)+1)
	blocked := false
	for _, source := range profile.SourceAnomalies {
		issue := make(map[string]any, len(source)+1)
		for key, value := range source {
			issue[key] = value
		}
		code, _ := issue["code"].(string)
		severity, _ := issue["severity"].(string)
		if strings.EqualFold(code, "dimension_item_weight_sum") {
			severity = "warning"
		} else if severity != "warning" {
			severity = "blocking"
		}
		issue["severity"] = severity
		blocked = blocked || severity == "blocking"
		issues = append(issues, issue)
	}
	weights := make([]float64, 0, len(profile.Dimensions))
	groups := make([][]float64, 0, len(profile.Dimensions))
	for _, dimension := range profile.Dimensions {
		weights = append(weights, dimension.Weight)
		group := make([]float64, 0, len(dimension.Items))
		for _, item := range dimension.Items {
			group = append(group, item.ItemWeight)
		}
		groups = append(groups, group)
	}
	if err := ValidateTemplateWeights(weights, groups); err != nil {
		blocked = true
		issues = append(issues, map[string]any{"code": "composite_weight_total", "severity": "blocking", "message": err.Error()})
	}
	return issues, blocked
}

func validateSourceCatalog(catalog hongxingCatalog) error {
	if catalog.Version == "" || len(catalog.SourceSHA256) != 64 || len(catalog.Profiles) == 0 || catalog.SourceTemplateCount != len(catalog.Profiles) {
		return errors.New("工作簿目录或来源不完整，请重新验证工作簿")
	}
	departments := make(map[string]bool)
	profiles := make(map[string]bool)
	for _, profile := range catalog.Profiles {
		if strings.TrimSpace(profile.Department) == "" || strings.TrimSpace(profile.Position) == "" || profile.Period != "monthly" {
			return errors.New("初始模板部门、岗位或周期无效")
		}
		key := profile.Department + "/" + profile.Position
		if profiles[key] {
			return fmt.Errorf("重复初始模板: %s", key)
		}
		profiles[key] = true
		departments[profile.Department] = true
		if len(profile.Dimensions) == 0 {
			return fmt.Errorf("初始模板 %s 缺少维度", key)
		}
		for _, dimension := range profile.Dimensions {
			if strings.TrimSpace(dimension.Name) == "" || !validWeight(dimension.Weight) || len(dimension.Items) == 0 {
				return fmt.Errorf("初始模板 %s 维度名称、权重或指标无效", key)
			}
			for _, item := range dimension.Items {
				if !validWeight(item.ItemWeight) {
					return fmt.Errorf("初始模板 %s 指标权重无效: %s", key, item.Name)
				}
			}
		}
	}
	if catalog.SourceDepartmentCount != len(departments) {
		return errors.New("工作簿部门数量与模板目录不一致")
	}
	return nil
}

func importHongxingCatalog(db *gorm.DB, catalog hongxingCatalog, mode string, bootstrap bool) error {
	return importHongxingCatalogWithDepartments(db, catalog, mode, bootstrap, nil)
}

func importHongxingCatalogWithDepartments(db *gorm.DB, catalog hongxingCatalog, mode string, bootstrap bool, mappedDepartments map[string]uint) error {
	if err := validateSourceCatalog(catalog); err != nil {
		return err
	}
	if mode != "draft" && mode != "publish" {
		return errors.New("导入模式无效")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.BusinessInitialization{}).Where("key = ?", catalog.Version).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 && bootstrap {
			// Older deployments may already contain the v2 catalog but predate
			// the dedicated source_sha256 column. Backfill provenance without
			// rewriting any HR-maintained template structure or status.
			if err := tx.Model(&models.KPITemplate{}).
				Where("source_catalog_version = ? AND (source_sha256 IS NULL OR source_sha256 = '')", catalog.Version).
				Update("source_sha256", catalog.SourceSHA256).Error; err != nil {
				return err
			}
			return repairLegacyLocalWeightBlocks(tx, catalog)
		}
		departments := make(map[string]uint)
		profiles := make(map[string]bool)
		effectiveProfiles := make(map[string]bool)
		for _, profile := range catalog.Profiles {
			if profile.Department == "" || profile.Position == "" || profile.Period != "monthly" {
				return errors.New("初始模板部门、岗位或周期无效")
			}
			key := profile.Department + "/" + profile.Position
			if profiles[key] {
				return fmt.Errorf("重复初始模板: %s", key)
			}
			profiles[key] = true
			for _, dimension := range profile.Dimensions {
				if !validWeight(dimension.Weight) || len(dimension.Items) == 0 {
					return fmt.Errorf("初始模板 %s 维度权重或指标无效", key)
				}
				for _, sourceItem := range dimension.Items {
					if !validWeight(sourceItem.ItemWeight) {
						return fmt.Errorf("初始模板 %s 指标权重无效: %s", key, sourceItem.Name)
					}
				}
			}
			deptID, exists := departments[profile.Department]
			if mappedID := mappedDepartments[profile.Department]; mappedID != 0 {
				// An explicit mapping always wins over a same-named source
				// department that may already exist from an earlier import.
				deptID, exists = mappedID, true
				departments[profile.Department] = mappedID
			}
			if !exists {
				var found []models.Department
				departmentQuery := tx.Where("name = ?", profile.Department)
				if mappedID := mappedDepartments[profile.Department]; mappedID != 0 {
					departmentQuery = tx.Where("id = ?", mappedID)
				}
				if err := departmentQuery.Find(&found).Error; err != nil {
					return err
				}
				if len(found) > 1 {
					return fmt.Errorf("部门“%s”存在重名，请先处理歧义", profile.Department)
				}
				if len(found) == 1 {
					deptID = found[0].ID
				} else {
					department := models.Department{Name: profile.Department, Description: "荭杏研发项目绩效考核表初始部门"}
					if err := tx.Create(&department).Error; err != nil {
						return err
					}
					deptID = department.ID
				}
				departments[profile.Department] = deptID
			}
			effectiveKey := fmt.Sprintf("%d/%s/%s", deptID, strings.ToLower(profile.Position), profile.Period)
			if effectiveProfiles[effectiveKey] {
				return fmt.Errorf("%w: 部门映射产生重复的部门、岗位和周期模板：%s", errSourceMappingInvalid, key)
			}
			effectiveProfiles[effectiveKey] = true
			position := models.JobPosition{DepartmentID: deptID, Name: profile.Position}
			if err := tx.Where("department_id = ? AND name = ?", deptID, profile.Position).FirstOrCreate(&position).Error; err != nil {
				return err
			}
			query := tx.Model(&models.KPITemplate{}).Where("department_id = ? AND LOWER(position_type) = LOWER(?) AND period = ?", deptID, profile.Position, profile.Period)
			if err := query.Count(&count).Error; err != nil {
				return err
			}
			// Never overwrite or re-publish an HR-maintained matching template.
			if count > 0 && bootstrap {
				continue
			}
			var sameSource int64
			if err := query.Session(&gorm.Session{}).Where("source_sha256 = ?", catalog.SourceSHA256).Count(&sameSource).Error; err != nil {
				return err
			}
			if sameSource > 0 {
				if mode == "publish" {
					var published int64
					if err := tx.Model(&models.KPITemplate{}).Where("department_id = ? AND LOWER(position_type) = LOWER(?) AND period = ? AND source_sha256 = ? AND status = ? AND is_active = ? AND source_blocked = ?", deptID, profile.Position, profile.Period, catalog.SourceSHA256, "published", true, false).Count(&published).Error; err != nil {
						return err
					}
					if published != 1 {
						return fmt.Errorf("%w: %s 的来源版本尚未启用发布，请在模板列表中显式发布", errSourcePublishConflict, key)
					}
				}
				continue
			}
			replacedTemplateIDs := make([]uint, 0)
			if mode == "publish" {
				// A new workbook revision replaces the currently active rule only
				// after every new template has been materialized and validated in
				// this transaction. Historical versions remain queryable as
				// retired templates and already-created evaluations keep snapshots.
				var activeTemplates []models.KPITemplate
				if err := tx.Model(&models.KPITemplate{}).
					Where("department_id = ? AND LOWER(position_type) = LOWER(?) AND period = ? AND status = ? AND is_active = ?", deptID, profile.Position, profile.Period, "published", true).
					Order("version DESC, id DESC").
					Find(&activeTemplates).Error; err != nil {
					return err
				}
				for _, activeTemplate := range activeTemplates {
					replacedTemplateIDs = append(replacedTemplateIDs, activeTemplate.ID)
				}
			}
			version := 1
			if !bootstrap {
				var maxVersion int
				if err := tx.Model(&models.KPITemplate{}).Where("department_id = ? AND LOWER(position_type) = LOWER(?) AND period = ?", deptID, profile.Position, profile.Period).Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error; err != nil {
					return err
				}
				version = maxVersion + 1
			}
			layout, _, err := marshalExportLayout(defaultExportLayout(exportPresetFinalSignoff))
			if err != nil {
				return err
			}
			profileIssues, blocked := profileSourceIssues(profile)
			if mode == "publish" && blocked && !bootstrap {
				return fmt.Errorf("%w: %s 存在阻断来源问题", errSourcePublishConflict, key)
			}
			issues, err := json.Marshal(profileIssues)
			if err != nil {
				return err
			}
			workflowRulesJSON := ""
			if catalog.Flow != nil {
				rawWorkflow, marshalErr := json.Marshal(catalog.Flow)
				if marshalErr != nil {
					return marshalErr
				}
				workflowRulesJSON = string(rawWorkflow)
			}
			status := "published"
			active := true
			if mode == "draft" || blocked {
				status = "draft"
				active = false
			}
			template := models.KPITemplate{Name: profile.Department + " / " + profile.Position + "项目绩效考核", Description: "来源：" + catalog.SourceFilename + "，" + profile.Sheet,
				DepartmentID: &deptID, PositionType: profile.Position, Period: profile.Period, Version: version, Status: status, CalculationVersion: canonicalCalculationVersion, IsActive: active, ExportLayoutJSON: layout, SourceCatalogVersion: catalog.Version, SourceSHA256: catalog.SourceSHA256, SourceIssuesJSON: string(issues), SourceBlocked: blocked, WorkflowRulesJSON: workflowRulesJSON}
			if err := tx.Create(&template).Error; err != nil {
				return err
			}
			for _, input := range profile.Dimensions {
				if strings.TrimSpace(input.Name) == "" {
					return errors.New("初始维度名称不能为空")
				}
				dimension := models.KPIDimension{TemplateID: template.ID, Name: input.Name, Weight: input.Weight, SortOrder: input.SortOrder}
				if err := tx.Create(&dimension).Error; err != nil {
					return err
				}
				for _, inputItem := range input.Items {
					if strings.TrimSpace(inputItem.Name) == "" || !inputItem.Required || len(inputItem.ScorerTypes) == 0 {
						return errors.New("初始指标或评分规则不完整")
					}
					for _, code := range inputItem.ScorerTypes {
						if code != "PM" && code != "CTO" && code != "DEPARTMENT_MANAGER" && code != "PRODUCT_MANAGER" {
							if bootstrap || status != "draft" || !blocked {
								return fmt.Errorf("初始评分类型无效: %s", code)
							}
						}
					}
					scorers, err := json.Marshal(inputItem.ScorerTypes)
					if err != nil {
						return err
					}
					item := models.KPIItem{TemplateID: template.ID, DimensionID: &dimension.ID, Name: inputItem.Name, Description: "原表评分人：" + inputItem.SourceScorerLabel,
						MaxScore: 100, Order: inputItem.Order, ItemWeight: inputItem.ItemWeight, CompositeWeight: CompositeWeight(dimension.Weight, inputItem.ItemWeight),
						ScoringGuidance: inputItem.ScoringGuidance, DataSource: inputItem.DataSource, ScorerTypesJSON: string(scorers), Required: true}
					if err := tx.Create(&item).Error; err != nil {
						return err
					}
				}
			}
			loaded, err := loadCalculationTemplate(tx, template.ID)
			if err != nil {
				return err
			}
			if !blocked {
				if err := validateTemplateForPublish(loaded); err != nil {
					return fmt.Errorf("初始模板 %s 校验失败: %w", template.Name, err)
				}
			}
			if mode == "publish" && len(replacedTemplateIDs) > 0 {
				if err := tx.Model(&models.KPITemplate{}).
					Where("id IN ?", replacedTemplateIDs).
					Updates(map[string]any{"status": "retired", "is_active": false}).Error; err != nil {
					return err
				}
				retiredIDs := make([]string, 0, len(replacedTemplateIDs))
				for _, id := range replacedTemplateIDs {
					retiredIDs = append(retiredIDs, strconv.FormatUint(uint64(id), 10))
				}
				if err := tx.Create(&models.AuditLog{
					Action:     "template_version_switch",
					Resource:   "kpi_template",
					ResourceID: strconv.FormatUint(uint64(template.ID), 10),
					Result:     "SUCCESS",
					Details:    "source_sha256=" + catalog.SourceSHA256 + ";retired_ids=" + strings.Join(retiredIDs, ","),
				}).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Where("key = ?", catalog.Version).FirstOrCreate(&models.BusinessInitialization{Key: catalog.Version, SourceSHA256: catalog.SourceSHA256}).Error; err != nil {
			return err
		}
		action := "import_hongxing_catalog"
		if bootstrap {
			action = "initialize_hongxing_catalog"
		}
		return tx.Create(&models.AuditLog{Action: action, Resource: "business_catalog", ResourceID: catalog.Version, Result: "SUCCESS", Details: "source_sha256=" + catalog.SourceSHA256 + ";mode=" + mode}).Error
	})
}

// Upgrade only the old, known local-sum false blockers. Do not publish drafts,
// rewrite weights, or acknowledge unrelated source failures during startup.
func repairLegacyLocalWeightBlocks(tx *gorm.DB, catalog hongxingCatalog) error {
	var templates []models.KPITemplate
	if err := tx.Where("source_sha256 = ? AND source_blocked = ?", catalog.SourceSHA256, true).Find(&templates).Error; err != nil {
		return err
	}
	for _, template := range templates {
		var issues []map[string]any
		if json.Unmarshal([]byte(template.SourceIssuesJSON), &issues) != nil || len(issues) == 0 {
			continue
		}
		onlyLocal := true
		for _, issue := range issues {
			code, _ := issue["code"].(string)
			if !strings.EqualFold(code, "dimension_item_weight_sum") {
				onlyLocal = false
			}
			issue["severity"] = "warning"
		}
		if !onlyLocal {
			continue
		}
		loaded, err := loadCalculationTemplate(tx, template.ID)
		if err != nil {
			return err
		}
		loaded.SourceBlocked = false
		if validateTemplateForPublish(loaded) != nil {
			continue
		}
		encoded, err := json.Marshal(issues)
		if err != nil {
			return err
		}
		if err := tx.Model(&models.KPITemplate{}).Where("id = ?", template.ID).Updates(map[string]any{"source_blocked": false, "source_issues_json": string(encoded)}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.AuditLog{Action: "repair_local_weight_source_diagnostic", Resource: "kpi_template", ResourceID: strconv.FormatUint(uint64(template.ID), 10), Result: "SUCCESS", Details: "source_sha256=" + catalog.SourceSHA256 + ";rule=rounded_composite_total_100;status_preserved=" + template.Status}).Error; err != nil {
			return err
		}
	}
	return nil
}

func GetDepartmentPositions(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的部门ID"})
		return
	}
	var department models.Department
	if err := models.DB.First(&department, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "部门不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取部门失败"})
		}
		return
	}
	positions := []models.JobPosition{}
	if err := models.DB.Where("department_id = ?", id).Order("id").Find(&positions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取岗位目录失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": positions})
}
