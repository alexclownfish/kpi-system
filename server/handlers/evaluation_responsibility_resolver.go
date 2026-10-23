package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"dootask-kpi-server/models"
	"gorm.io/gorm"
)

// workflowScoringAssignment is the assessment-time result of resolving one
// template scorer rule to one concrete user. It is copied into the immutable
// snapshot and then materialized as an EvaluationScoringTask.
type workflowScoringAssignment struct {
	ItemID     uint
	ScorerType string
	UserID     uint
	UserName   string
	SourceRule string
}

// scorerResolutionError carries machine-readable missing responsibility data
// while retaining a human-readable combined message for legacy callers.
type scorerResolutionError struct {
	Issues  []error
	Missing []map[string]any
}

func (e *scorerResolutionError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "评分职责解析失败"
	}
	return errors.Join(e.Issues...).Error()
}

func (e *scorerResolutionError) Unwrap() []error {
	if e == nil {
		return nil
	}
	return e.Issues
}

// resolveTemplateScoringAssignments resolves scorer rules at assessment
// creation time. New workflow records intentionally use business
// responsibilities instead of guessing from RBAC role names or employee.role.
// Existing legacy records continue to use resolveTemplateScorers for backward
// compatibility until they are explicitly migrated.
func resolveTemplateScoringAssignments(db *gorm.DB, employee models.Employee, template models.KPITemplate) ([]workflowScoringAssignment, error) {
	assignments := make([]workflowScoringAssignment, 0)
	seen := make(map[string]struct{})
	issues := []error{}
	missing := make([]map[string]any, 0)
	failedTypes := map[string]bool{}
	for _, item := range template.Items {
		itemTypes := append([]string(nil), item.ScorerTypes...)
		if len(itemTypes) == 0 && strings.TrimSpace(item.ScorerTypesJSON) != "" {
			if err := json.Unmarshal([]byte(item.ScorerTypesJSON), &itemTypes); err != nil {
				issues = append(issues, fmt.Errorf("指标“%s”评分职责配置不是有效JSON", item.Name))
				continue
			}
		}
		if item.Required && len(itemTypes) == 0 {
			issues = append(issues, fmt.Errorf("必填指标“%s”未配置评分职责", item.Name))
			missing = append(missing, map[string]any{"item": item.Name, "scorer_types": []string{}})
			continue
		}
		for _, rawType := range itemTypes {
			scorerType := strings.ToUpper(strings.TrimSpace(rawType))
			if scorerType == "" {
				continue
			}
			users, sourceRule, err := resolveScorerUsersByResponsibility(db, employee, scorerType)
			if err != nil {
				if !failedTypes[scorerType] {
					issues = append(issues, fmt.Errorf("指标“%s”无法解析%s评分人：%w", item.Name, scorerType, err))
					missing = append(missing, map[string]any{"item": item.Name, "scorer_type": scorerType})
					failedTypes[scorerType] = true
				}
				continue
			}
			if len(users) == 0 {
				if !failedTypes[scorerType] {
					issues = append(issues, fmt.Errorf("指标“%s”没有可用的%s评分人，请先维护评分职责", item.Name, scorerType))
					missing = append(missing, map[string]any{"item": item.Name, "scorer_type": scorerType})
					failedTypes[scorerType] = true
				}
				continue
			}
			for _, user := range users {
				key := fmt.Sprintf("%d:%d", item.ID, user.ID)
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				assignments = append(assignments, workflowScoringAssignment{
					ItemID: item.ID, ScorerType: scorerType, UserID: user.ID,
					UserName: user.Name, SourceRule: sourceRule,
				})
			}
		}
	}
	if len(issues) > 0 {
		return assignments, &scorerResolutionError{Issues: issues, Missing: missing}
	}
	if len(assignments) == 0 {
		return assignments, &scorerResolutionError{Issues: []error{errors.New("模板没有可生成的评分任务")}, Missing: missing}
	}
	return assignments, nil
}

func resolveScorerUsersByResponsibility(db *gorm.DB, employee models.Employee, scorerType string) ([]models.Employee, string, error) {
	if scorerType == "DEPARTMENT_MANAGER" {
		if isDepartmentHead(db, employee) {
			return resolveHeadSupervisor(db, employee)
		}
		users, configured, err := explicitDepartmentLeader(db, employee.DepartmentID, employee.ID)
		if configured || err != nil {
			if err == nil {
				err = managerAssignmentError(db, employee, users[0].ID)
			}
			return users, "DEPARTMENT_LEADER", err
		}
	}
	if scorerType == "DIRECT_MANAGER" {
		if employee.ManagerID == nil || *employee.ManagerID == 0 {
			return nil, "DIRECT_MANAGER", fmt.Errorf("员工“%s”没有直属负责人", employee.Name)
		}
		if err := managerAssignmentError(db, employee, *employee.ManagerID); err != nil {
			return nil, "DIRECT_MANAGER", err
		}
		var manager models.Employee
		if err := db.Where("id = ? AND is_active = ?", *employee.ManagerID, true).First(&manager).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, "DIRECT_MANAGER", fmt.Errorf("直属负责人不存在或已离职")
			}
			return nil, "DIRECT_MANAGER", err
		}
		return []models.Employee{manager}, "DIRECT_MANAGER", nil
	}

	if !isSupportedResponsibilityCode(scorerType) {
		return nil, scorerType, fmt.Errorf("不支持的评分职责类型")
	}

	query := activeResponsibilities(db, scorerType)

	// A responsibility may be company-wide, department-scoped, or
	// position-scoped. Project-scoped resolution is deliberately rejected until
	// the assessment model has a project identity to match against.
	query = query.Where(`
		(scope_type = 'company') OR
		(scope_type = 'department' AND department_id = ?) OR
		(scope_type = 'position' AND (department_id IS NULL OR department_id = ?) AND position_type IN ?)
	`, employee.DepartmentID, employee.DepartmentID, []string{employee.Position, canonicalEmployeePosition(db, employee)})

	var responsibilities []models.EvaluationResponsibility
	if err := query.Order("id ASC").Find(&responsibilities).Error; err != nil {
		return nil, scorerType, err
	}
	users := make([]models.Employee, 0, len(responsibilities))
	seenUsers := make(map[uint]struct{})
	for _, responsibility := range responsibilities {
		if responsibility.User.ID == 0 || responsibility.User.ID == employee.ID || !responsibility.User.IsActive {
			continue
		}
		if _, exists := seenUsers[responsibility.User.ID]; exists {
			continue
		}
		seenUsers[responsibility.User.ID] = struct{}{}
		users = append(users, responsibility.User)
	}
	return users, "evaluation_responsibility", nil
}

func isSupportedResponsibilityCode(code string) bool {
	switch code {
	case "PM", "DEPARTMENT_MANAGER", "PRODUCT_MANAGER", "CTO":
		return true
	default:
		return false
	}
}
