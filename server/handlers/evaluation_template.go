package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type templateMatchPreview struct {
	WorkflowMode   string                      `json:"workflow_mode"`
	Template       models.KPITemplate          `json:"template"`
	Scorers        map[string]scorerMatchEntry `json:"scorers"`
	Candidates     []scorerMatchCandidate      `json:"scorer_candidates"`
	Ready          bool                        `json:"ready"`
	BlockingErrors []string                    `json:"blocking_errors"`
}

type scorerMatchEntry struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type scorerMatchCandidate struct {
	ScorerType string `json:"scorer_type"`
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	Email      string `json:"email"`
}

func snapshotScorers(db *gorm.DB, evaluation models.KPIEvaluation) (map[string]uint, []models.KPIItem, error) {
	if evaluation.TemplateSnapshotJSON != "" {
		var snapshot evaluationTemplateSnapshot
		if err := json.Unmarshal([]byte(evaluation.TemplateSnapshotJSON), &snapshot); err == nil && len(snapshot.Scorers) > 0 {
			return snapshot.Scorers, snapshot.Items, nil
		}
	}
	var employee models.Employee
	if err := db.First(&employee, evaluation.EmployeeID).Error; err != nil {
		return nil, nil, err
	}
	template, err := loadCalculationTemplate(db, evaluation.TemplateID)
	if err != nil {
		return nil, nil, err
	}
	scorers, err := resolveTemplateScorers(db, employee, template)
	return scorers, template.Items, err
}

// ensureTemplateInvitations materializes template scorer assignments into the
// existing invitation workflow. It is idempotent and also repairs evaluations
// created before template-driven scorer tasks were introduced.
func ensureTemplateInvitations(db *gorm.DB, evaluation models.KPIEvaluation, inviterID uint) ([]models.EvaluationInvitation, error) {
	scorers, items, err := snapshotScorers(db, evaluation)
	if err != nil {
		return nil, err
	}
	if inviterID == 0 {
		inviterID = evaluation.EmployeeID
	}
	created := make([]models.EvaluationInvitation, 0)
	seen := map[uint]bool{}
	for _, inviteeID := range scorers {
		if inviteeID == 0 || inviteeID == evaluation.EmployeeID || seen[inviteeID] {
			continue
		}
		seen[inviteeID] = true
		var existing models.EvaluationInvitation
		if err := db.Where("evaluation_id = ? AND invitee_id = ?", evaluation.ID, inviteeID).First(&existing).Error; err == nil {
			continue
		}
		invitation := models.EvaluationInvitation{EvaluationID: evaluation.ID, InviterID: inviterID, InviteeID: inviteeID, Status: "pending", Message: "根据考核模板自动生成的评分任务"}
		if err := db.Create(&invitation).Error; err != nil {
			return nil, err
		}
		for _, item := range items {
			if err := db.Create(&models.InvitedScore{InvitationID: invitation.ID, ItemID: item.ID}).Error; err != nil {
				return nil, err
			}
		}
		created = append(created, invitation)
	}
	return created, nil
}

func PreviewEvaluationTemplate(c *gin.Context) {
	employeeID, err := strconv.ParseUint(c.Query("employee_id"), 10, 32)
	if err != nil || employeeID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "employee_id 参数无效"})
		return
	}
	period := strings.TrimSpace(c.Query("period"))
	if period == "" {
		period = "monthly"
	}
	if !CanAccessEmployee(actorID(c), uint(employeeID)) {
		respondError(c, http.StatusForbidden, "ASSESSMENT_SCOPE_FORBIDDEN", "被考核员工超出数据范围", nil)
		return
	}
	var employee models.Employee
	if err := models.DB.First(&employee, employeeID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "员工不存在"})
		return
	}
	template, err := findPublishedTemplate(models.DB, employee, period)
	if err != nil {
		status := http.StatusNotFound
		code := "TEMPLATE_NOT_FOUND"
		if strings.Contains(err.Error(), "TEMPLATE_MATCH_AMBIGUOUS") {
			status = http.StatusConflict
			code = "TEMPLATE_MATCH_AMBIGUOUS"
		}
		respondError(c, status, code, "无法匹配考核模板", map[string]any{"reason": err.Error()})
		return
	}
	scorerEntries := map[string]scorerMatchEntry{}
	candidates := make([]scorerMatchCandidate, 0)
	scorers := map[string]uint{}
	if models.EvaluationWorkflowWritesEnabled() || template.CalculationVersion == canonicalCalculationVersion {
		var assignments []workflowScoringAssignment
		assignments, err = resolveTemplateScoringAssignments(models.DB, employee, template)
		for _, assignment := range assignments {
			key := assignment.ScorerType
			if existing, ok := scorers[key]; ok && existing != assignment.UserID {
				key = fmt.Sprintf("%s:%d", key, assignment.UserID)
			}
			scorers[key] = assignment.UserID
		}
	} else {
		scorers, err = resolveTemplateScorers(models.DB, employee, template)
	}
	blocking := []string{}
	if template.CalculationVersion == canonicalCalculationVersion && !models.EvaluationWorkflowWritesEnabled() {
		blocking = append(blocking, "当前为 "+models.EvaluationWorkflowMode()+" 模式，新模板考核需管理员显式启用 EVALUATION_WORKFLOW_MODE=enabled")
	}
	if err != nil {
		blocking = append(blocking, err.Error())
	}
	for scorerType, scorerID := range scorers {
		var scorer models.Employee
		if err := models.DB.First(&scorer, scorerID).Error; err == nil {
			scorerEntries[scorerType] = scorerMatchEntry{ID: scorer.ID, Name: scorer.Name, Email: scorer.Email}
			candidates = append(candidates, scorerMatchCandidate{ScorerType: scorerType, ID: scorer.ID, Name: scorer.Name, Email: scorer.Email})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ScorerType != candidates[j].ScorerType {
			return candidates[i].ScorerType < candidates[j].ScorerType
		}
		return candidates[i].ID < candidates[j].ID
	})
	hydrateTemplateLayout(&template)
	c.JSON(http.StatusOK, gin.H{"data": templateMatchPreview{WorkflowMode: models.EvaluationWorkflowMode(), Template: template, Scorers: scorerEntries, Candidates: candidates, Ready: len(blocking) == 0, BlockingErrors: blocking}})
}

// evaluationTemplateSnapshot is stored on the evaluation so later template
// edits cannot change an already-created assessment.
type evaluationTemplateSnapshot struct {
	TemplateID           uint                  `json:"template_id"`
	TemplateName         string                `json:"template_name"`
	TemplateVersion      int                   `json:"template_version"`
	SourceCatalogVersion string                `json:"source_catalog_version,omitempty"`
	SourceSHA256         string                `json:"source_sha256,omitempty"`
	CalculationVersion   string                `json:"calculation_version"`
	DepartmentID         *uint                 `json:"department_id,omitempty"`
	PositionType         string                `json:"position_type"`
	Period               string                `json:"period"`
	Dimensions           []models.KPIDimension `json:"dimensions"`
	Items                []models.KPIItem      `json:"items"`
	Scorers              map[string]uint       `json:"scorers"`
}

func resolveTemplateScorers(db *gorm.DB, employee models.Employee, template models.KPITemplate) (map[string]uint, error) {
	// This resolver is kept only for legacy assessments and preview compatibility.
	// New workflow assessment creation must use resolveTemplateScoringAssignments,
	// which requires an explicit EvaluationResponsibility record.
	result := map[string]uint{}
	for _, item := range template.Items {
		for _, scorerType := range item.ScorerTypes {
			if _, exists := result[scorerType]; exists {
				continue
			}
			var scorer models.Employee
			query := db.Where("employees.is_active = ?", true)
			switch scorerType {
			case "DIRECT_MANAGER":
				if employee.ManagerID != nil {
					if err := managerAssignmentError(db, employee, *employee.ManagerID); err != nil {
						return nil, err
					}
				}
				if employee.ManagerID == nil {
					return nil, fmt.Errorf("员工“%s”没有直属负责人，无法解析评分人", employee.Name)
				}
				if err := query.First(&scorer, *employee.ManagerID).Error; err != nil {
					return nil, fmt.Errorf("直属负责人不存在，无法解析评分人")
				}
			case "DEPARTMENT_MANAGER":
				if isDepartmentHead(db, employee) {
					users, _, err := resolveHeadSupervisor(db, employee)
					if err != nil {
						return nil, err
					}
					scorer = users[0]
					break
				}
				users, configured, err := explicitDepartmentLeader(db, employee.DepartmentID, employee.ID)
				if err != nil {
					return nil, err
				}
				if configured {
					if err := managerAssignmentError(db, employee, users[0].ID); err != nil {
						return nil, err
					}
					scorer = users[0]
					break
				}
				if err := findScorer(db, query.Where("employees.id <> ?", employee.ID), employee.DepartmentID, []string{"department_manager", "department_head"}, []string{"manager", "hr"}, []string{"部门负责人", "部门主管", "department_manager"}, true, &scorer); err != nil {
					return nil, fmt.Errorf("部门“%d”没有可用的部门负责人，请先给在职账号绑定部门负责人职责", employee.DepartmentID)
				}
			case "PM", "PRODUCT_MANAGER", "CTO":
				roleCodes := map[string][]string{"PM": {"pm", "project_manager"}, "PRODUCT_MANAGER": {"product_manager", "product_owner"}, "CTO": {"cto", "technical_director"}}[scorerType]
				positions := map[string][]string{"PM": {"PM", "项目经理", "项目负责人"}, "PRODUCT_MANAGER": {"PRODUCT_MANAGER", "产品经理", "产品负责人"}, "CTO": {"CTO", "首席技术官", "技术总监"}}[scorerType]
				if err := findScorer(db, query, employee.DepartmentID, roleCodes, nil, positions, scorerType != "CTO", &scorer); err != nil {
					return nil, fmt.Errorf("无法解析%s类型评分人，请先给在职账号绑定对应职责", scorerType)
				}
			default:
				return nil, fmt.Errorf("不支持的评分人类型：%s", scorerType)
			}
			if scorer.ID == employee.ID {
				return nil, fmt.Errorf("%s评分人不能为被考核者本人", scorerType)
			}
			result[scorerType] = scorer.ID
		}
	}
	return result, nil
}

func findScorer(db *gorm.DB, base *gorm.DB, departmentID uint, roleCodes, legacyRoles, positions []string, preferDepartment bool, destination *models.Employee) error {
	roleQuery := func(q *gorm.DB, sameDepartment bool) *gorm.DB {
		if sameDepartment {
			q = q.Where("employees.department_id = ?", departmentID)
		}
		conditions := []string{"1=0"}
		args := []any{}
		if len(roleCodes) > 0 {
			conditions = append(conditions, "r.code IN ?")
			args = append(args, roleCodes)
		}
		if len(legacyRoles) > 0 {
			conditions = append(conditions, "employees.role IN ?")
			args = append(args, legacyRoles)
		}
		if len(positions) > 0 {
			conditions = append(conditions, "employees.position IN ?")
			args = append(args, positions)
		}
		return q.Joins("LEFT JOIN user_roles ur ON ur.user_id = employees.id LEFT JOIN roles r ON r.id = ur.role_id").Where("("+strings.Join(conditions[1:], " OR ")+")", args...)
	}
	if preferDepartment {
		if err := roleQuery(base, true).Order("employees.id").First(destination).Error; err == nil {
			return nil
		}
	}
	return roleQuery(base, false).Order("employees.id").First(destination).Error
}

func findPublishedTemplate(db *gorm.DB, employee models.Employee, period string) (models.KPITemplate, error) {
	period = strings.ToLower(strings.TrimSpace(period))
	if period != "monthly" && period != "quarterly" && period != "yearly" {
		return models.KPITemplate{}, fmt.Errorf("考核周期无效：%s", period)
	}
	if employee.DepartmentID == 0 {
		return models.KPITemplate{}, fmt.Errorf("员工“%s”未配置部门，无法匹配考核模板", employee.Name)
	}
	rawPosition := strings.TrimSpace(employee.Position)
	if rawPosition == "" {
		return models.KPITemplate{}, fmt.Errorf("员工“%s”未配置岗位，无法匹配考核模板", employee.Name)
	}
	positions := []string{rawPosition}
	canonical := strings.TrimSpace(canonicalEmployeePosition(db, employee))
	if canonical != "" && !strings.EqualFold(canonical, rawPosition) {
		positions = append(positions, canonical)
	}
	var templates []models.KPITemplate
	query := db.Preload("Dimensions").Preload("Items").
		Where("status = ? AND is_active = ? AND period = ?", "published", true, period).
		Where("department_id = ?", employee.DepartmentID)
	// Match exact employee position first, then a single canonical catalog
	// position. Do the candidate count before selecting the newest version so a
	// misconfigured duplicate cannot be silently hidden by ordering.
	if err := query.Where("LOWER(TRIM(position_type)) IN ?", lowerStrings(positions)).Order("version DESC, id DESC").Find(&templates).Error; err != nil {
		return models.KPITemplate{}, err
	}
	if len(templates) == 0 {
		return models.KPITemplate{}, fmt.Errorf("未找到员工“%s”适用的已发布考核模板（部门=%d，岗位=%s，周期=%s）", employee.Name, employee.DepartmentID, employee.Position, period)
	}
	// If raw and canonical names point to the same template, de-duplicate by
	// ID. Distinct template IDs remain a hard ambiguity even when one has a
	// newer version; HR must retire/fix one before creating an assessment.
	unique := make(map[uint]models.KPITemplate, len(templates))
	for _, candidate := range templates {
		unique[candidate.ID] = candidate
	}
	if len(unique) != 1 {
		return models.KPITemplate{}, fmt.Errorf("TEMPLATE_MATCH_AMBIGUOUS: 员工“%s”匹配到多个已发布考核模板（部门=%d，岗位=%s，周期=%s），请先停用重复模板", employee.Name, employee.DepartmentID, employee.Position, period)
	}
	var template models.KPITemplate
	for _, candidate := range unique {
		template = candidate
	}
	hydrateCalculationTemplate(&template)
	hydrateTemplateLayout(&template)
	return template, nil
}

func lowerStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		seen := false
		for _, existing := range result {
			if existing == value {
				seen = true
				break
			}
		}
		if !seen {
			result = append(result, value)
		}
	}
	return result
}

func templateMatchesEmployee(db *gorm.DB, template models.KPITemplate, employee models.Employee, period string) bool {
	period = strings.ToLower(strings.TrimSpace(period))
	return template.DepartmentID != nil && *template.DepartmentID == employee.DepartmentID && strings.EqualFold(strings.TrimSpace(template.Period), period) &&
		(strings.EqualFold(strings.TrimSpace(template.PositionType), strings.TrimSpace(employee.Position)) || strings.EqualFold(strings.TrimSpace(template.PositionType), canonicalEmployeePosition(db, employee)))
}

func buildEvaluationTemplateSnapshot(template models.KPITemplate) (string, error) {
	// Scorer resolution is deliberately performed by the caller with employee context.
	return buildEvaluationTemplateSnapshotWithScorers(template, nil)
}

func buildEvaluationTemplateSnapshotWithScorers(template models.KPITemplate, scorers map[string]uint) (string, error) {
	snapshot := evaluationTemplateSnapshot{
		TemplateID: template.ID, TemplateName: template.Name, TemplateVersion: template.Version,
		SourceCatalogVersion: template.SourceCatalogVersion, SourceSHA256: template.SourceSHA256,
		CalculationVersion: template.CalculationVersion, DepartmentID: template.DepartmentID,
		PositionType: template.PositionType, Period: template.Period,
		Dimensions: template.Dimensions, Items: template.Items,
		Scorers: scorers,
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return "", errors.New("生成考核模板快照失败")
	}
	return string(raw), nil
}

// buildWorkflowTemplateSnapshot stores the full item-level scorer resolution
// for new workflow assessments. The legacy snapshot builder above remains
// available for compatibility and migration of older assessments.
func buildWorkflowTemplateSnapshot(template models.KPITemplate, assignments []workflowScoringAssignment) (string, error) {
	itemsByDimension := make(map[uint][]workflowSnapshotItem)
	for _, item := range template.Items {
		scorerTypes := append([]string(nil), item.ScorerTypes...)
		if len(scorerTypes) == 0 && item.ScorerTypesJSON != "" {
			_ = json.Unmarshal([]byte(item.ScorerTypesJSON), &scorerTypes)
		}
		dimensionID := uint(0)
		if item.DimensionID != nil {
			dimensionID = *item.DimensionID
		}
		itemsByDimension[dimensionID] = append(itemsByDimension[dimensionID], workflowSnapshotItem{
			ID: item.ID, Name: item.Name, Description: item.Description, MaxScore: item.MaxScore,
			ItemWeight: item.ItemWeight, CompositeWeight: item.CompositeWeight, Required: item.Required,
			ScorerTypes: scorerTypes, Aggregation: "average", ScoringGuidance: item.ScoringGuidance,
			DataSource: item.DataSource,
		})
	}
	dimensions := make([]workflowSnapshotDimension, 0, len(template.Dimensions))
	for _, dimension := range template.Dimensions {
		dimensions = append(dimensions, workflowSnapshotDimension{
			ID: dimension.ID, Name: dimension.Name, Description: dimension.Description,
			Weight: dimension.Weight, Items: itemsByDimension[dimension.ID],
		})
	}
	// Keep orphaned legacy items in the snapshot rather than silently dropping
	// them; validation and audit can then report the malformed template.
	if orphaned := itemsByDimension[0]; len(orphaned) > 0 {
		dimensions = append(dimensions, workflowSnapshotDimension{ID: 0, Name: "未归类指标", Weight: 0, Items: orphaned})
	}
	resolved := make([]evaluationResolvedScorer, 0, len(assignments))
	resolvedTypes := make([]string, 0, len(assignments))
	for _, assignment := range assignments {
		resolved = append(resolved, evaluationResolvedScorer{
			ItemID: assignment.ItemID, ScorerType: assignment.ScorerType, UserID: assignment.UserID,
			UserName: assignment.UserName, SourceRule: assignment.SourceRule,
		})
		resolvedTypes = appendUniqueString(resolvedTypes, assignment.ScorerType)
	}
	layout := responsibilitySigningLabels(template.ExportLayout, resolvedTypes)
	signoffRules := workflowSignoffRulesForTemplate(template)
	raw, err := json.Marshal(workflowEvaluationSnapshot{
		Template:             workflowTemplateIdentity{ID: template.ID, Name: template.Name, Version: template.Version, Period: template.Period},
		SourceCatalogVersion: template.SourceCatalogVersion,
		SourceSHA256:         template.SourceSHA256,
		DepartmentID:         template.DepartmentID,
		PositionType:         template.PositionType,
		CalculationVersion:   canonicalCalculationVersion,
		Dimensions:           dimensions,
		ResolvedScorers:      resolved,
		SignoffRules:         signoffRules,
		ExportLayout:         layout,
	})
	if err != nil {
		return "", errors.New("生成新版考核模板快照失败")
	}
	return string(raw), nil
}

// defaultWorkflowSignoffRules mirrors the Hongxing process default while
// keeping signer responsibilities in the immutable snapshot. Templates with
// custom signature labels still retain those labels in ExportLayout; this
// list controls the structured signer contract used by workflow/audit code.
func defaultWorkflowSignoffRules(position string) []workflowSnapshotSignoff {
	position = strings.ToLower(strings.TrimSpace(position))
	if strings.Contains(position, "工程师") || strings.Contains(position, "engineer") {
		return []workflowSnapshotSignoff{
			{SignerType: "SUBJECT", Required: true, Sequence: 1, Source: "process_default"},
			{SignerType: "PM", Required: true, Sequence: 2, Source: "process_default"},
			{SignerType: "PRODUCT_MANAGER", Required: true, Sequence: 3, Source: "process_default"},
		}
	}
	return []workflowSnapshotSignoff{
		{SignerType: "SUBJECT", Required: true, Sequence: 1, Source: "process_default"},
		{SignerType: "CTO", Required: true, Sequence: 2, Source: "process_default"},
	}
}

func workflowSignoffRulesForTemplate(template models.KPITemplate) []workflowSnapshotSignoff {
	if strings.TrimSpace(template.WorkflowRulesJSON) != "" {
		var flow hongxingFlow
		if json.Unmarshal([]byte(template.WorkflowRulesJSON), &flow) == nil {
			for _, step := range flow.Steps {
				if step.Step != 7 {
					continue
				}
				rules := []workflowSnapshotSignoff{{SignerType: "SUBJECT", Required: true, Sequence: 1, Source: "excel_process"}}
				position := strings.ToLower(strings.TrimSpace(template.PositionType))
				if strings.Contains(position, "工程师") || strings.Contains(position, "engineer") {
					rules = append(rules,
						workflowSnapshotSignoff{SignerType: "PM", Required: true, Sequence: 2, Source: "excel_process"},
						workflowSnapshotSignoff{SignerType: "PRODUCT_MANAGER", Required: true, Sequence: 3, Source: "excel_process"})
				} else {
					rules = append(rules, workflowSnapshotSignoff{SignerType: "CTO", Required: true, Sequence: 2, Source: "excel_process"})
				}
				if len(rules) > 1 {
					return rules
				}
			}
		}
	}
	return defaultWorkflowSignoffRules(template.PositionType)
}
