package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"dootask-kpi-server/models"
)

const canonicalCalculationVersion = "weighted-v2"

type evaluationResolvedScorer struct {
	ItemID     uint   `json:"item_id"`
	ScorerType string `json:"scorer_type"`
	UserID     uint   `json:"user_id"`
	UserName   string `json:"user_name,omitempty"`
	SourceRule string `json:"source_rule,omitempty"`
}

type workflowTemplateIdentity struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	Version int    `json:"version"`
	Period  string `json:"period"`
}

type workflowSnapshotDimension struct {
	ID          uint                   `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Weight      float64                `json:"weight"`
	Items       []workflowSnapshotItem `json:"items"`
}

type workflowSnapshotItem struct {
	ID              uint     `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"description,omitempty"`
	MaxScore        float64  `json:"max_score"`
	ItemWeight      float64  `json:"item_weight"`
	CompositeWeight float64  `json:"composite_weight"`
	Required        bool     `json:"required"`
	ScorerTypes     []string `json:"scorer_types"`
	Aggregation     string   `json:"aggregation"`
	ScoringGuidance string   `json:"scoring_guidance,omitempty"`
	DataSource      string   `json:"data_source,omitempty"`
}

type workflowEvaluationSnapshot struct {
	Template workflowTemplateIdentity `json:"template"`
	// These fields make the assessment's business context auditable even when
	// the employee, organization, or template is edited later.
	SourceCatalogVersion string                      `json:"source_catalog_version,omitempty"`
	SourceSHA256         string                      `json:"source_sha256,omitempty"`
	DepartmentID         *uint                       `json:"department_id,omitempty"`
	PositionType         string                      `json:"position_type,omitempty"`
	CalculationVersion   string                      `json:"calculation_version"`
	Dimensions           []workflowSnapshotDimension `json:"dimensions"`
	ResolvedScorers      []evaluationResolvedScorer  `json:"resolved_scorers"`
	SignoffRules         []workflowSnapshotSignoff   `json:"signoff_rules,omitempty"`
	ExportLayout         models.ResultExportLayout   `json:"export_layout"`
}

// workflowSnapshotSignoff is immutable paper/online sign-off responsibility
// metadata. It is deliberately separate from RBAC roles and resolved scorer
// assignments so later role changes cannot rewrite a historical assessment.
type workflowSnapshotSignoff struct {
	SignerType string `json:"signer_type"`
	Required   bool   `json:"required"`
	Sequence   int    `json:"sequence"`
	Source     string `json:"source"`
}

type parsedEvaluationSnapshot struct {
	TemplateID           uint
	TemplateName         string
	TemplateVersion      int
	Period               string
	SourceCatalogVersion string
	SourceSHA256         string
	DepartmentID         *uint
	PositionType         string
	CalculationVersion   string
	Dimensions           []workflowSnapshotDimension
	Items                []workflowSnapshotItem
	ResolvedScorers      []evaluationResolvedScorer
	SignoffRules         []workflowSnapshotSignoff
	ExportLayout         models.ResultExportLayout
	Checksum             string
	Legacy               bool
}

// scorerTypeLabel turns the stable responsibility code stored in a snapshot
// into the Chinese role label shown in printable exports. Unknown/custom
// responsibility codes remain visible instead of being dropped.
func scorerTypeLabel(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "PM":
		return "项目经理"
	case "DEPARTMENT_MANAGER", "DEPARTMENT_HEAD":
		return "部门负责人"
	case "PRODUCT_MANAGER", "PRODUCT_OWNER":
		return "产品经理"
	case "CTO", "TECHNICAL_DIRECTOR":
		return "CTO"
	case "DIRECT_MANAGER":
		return "直属负责人"
	default:
		return strings.TrimSpace(code)
	}
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// responsibilitySigningLabels derives paper-signature labels only while the
// template still carries the old generic defaults. Any explicitly customised
// labels remain untouched and are copied into the immutable assessment
// snapshot by the caller.
func responsibilitySigningLabels(layout models.ResultExportLayout, scorerTypes []string) models.ResultExportLayout {
	layout = normalizeExportLayout(layout)
	generic := defaultExportLayout(layout.Preset).SignatureLabels
	if len(layout.SignatureLabels) > 0 && !sameStrings(layout.SignatureLabels, generic) {
		return layout
	}
	if len(scorerTypes) == 0 {
		return layout
	}
	labels := []string{"员工签字"}
	for _, scorerType := range scorerTypes {
		label := scorerTypeLabel(scorerType)
		if label == "" {
			continue
		}
		labels = appendUniqueString(labels, label+"签字")
	}
	if len(labels) > 0 && len(labels) <= 6 {
		layout.SignatureLabels = labels
	}
	return layout
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if strings.TrimSpace(left[index]) != strings.TrimSpace(right[index]) {
			return false
		}
	}
	return true
}

func parseEvaluationSnapshot(raw string) (parsedEvaluationSnapshot, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return parsedEvaluationSnapshot{}, errors.New("考核缺少模板快照")
	}
	checksum := sha256.Sum256([]byte(raw))
	result := parsedEvaluationSnapshot{Checksum: hex.EncodeToString(checksum[:])}

	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return parsedEvaluationSnapshot{}, fmt.Errorf("模板快照不是有效JSON: %w", err)
	}
	if _, exists := probe["template"]; exists {
		var snapshot workflowEvaluationSnapshot
		if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
			return parsedEvaluationSnapshot{}, fmt.Errorf("解析weighted-v2模板快照失败: %w", err)
		}
		result.TemplateID = snapshot.Template.ID
		result.TemplateName = snapshot.Template.Name
		result.TemplateVersion = snapshot.Template.Version
		result.Period = snapshot.Template.Period
		result.SourceCatalogVersion = snapshot.SourceCatalogVersion
		result.SourceSHA256 = snapshot.SourceSHA256
		result.DepartmentID = snapshot.DepartmentID
		result.PositionType = snapshot.PositionType
		result.CalculationVersion = snapshot.CalculationVersion
		result.Dimensions = snapshot.Dimensions
		result.ResolvedScorers = snapshot.ResolvedScorers
		result.SignoffRules = snapshot.SignoffRules
		result.ExportLayout = snapshot.ExportLayout
		for _, dimension := range snapshot.Dimensions {
			result.Items = append(result.Items, dimension.Items...)
		}
		if err := validateParsedEvaluationSnapshot(result); err != nil {
			return parsedEvaluationSnapshot{}, err
		}
		return result, nil
	}

	var legacy evaluationTemplateSnapshot
	if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
		return parsedEvaluationSnapshot{}, fmt.Errorf("解析旧版模板快照失败: %w", err)
	}
	result.TemplateID = legacy.TemplateID
	result.TemplateName = legacy.TemplateName
	result.TemplateVersion = legacy.TemplateVersion
	result.Period = legacy.Period
	result.SourceCatalogVersion = legacy.SourceCatalogVersion
	result.SourceSHA256 = legacy.SourceSHA256
	result.CalculationVersion = legacy.CalculationVersion
	result.Legacy = true
	for _, item := range legacy.Items {
		aggregation := "average"
		result.Items = append(result.Items, workflowSnapshotItem{
			ID: item.ID, Name: item.Name, Description: item.Description, MaxScore: item.MaxScore,
			ItemWeight: item.ItemWeight, CompositeWeight: item.CompositeWeight, Required: item.Required,
			ScorerTypes: item.ScorerTypes, Aggregation: aggregation,
			ScoringGuidance: item.ScoringGuidance, DataSource: item.DataSource,
		})
	}
	for scorerType, userID := range legacy.Scorers {
		result.ResolvedScorers = append(result.ResolvedScorers, evaluationResolvedScorer{ScorerType: scorerType, UserID: userID, SourceRule: "legacy_scorer_type"})
	}
	if err := validateParsedEvaluationSnapshot(result); err != nil {
		return parsedEvaluationSnapshot{}, err
	}
	return result, nil
}

func validateParsedEvaluationSnapshot(snapshot parsedEvaluationSnapshot) error {
	if snapshot.TemplateID == 0 || strings.TrimSpace(snapshot.TemplateName) == "" {
		return errors.New("模板快照缺少模板标识")
	}
	if len(snapshot.Items) == 0 {
		return errors.New("模板快照没有考核指标")
	}
	seenItems := make(map[uint]struct{}, len(snapshot.Items))
	for _, item := range snapshot.Items {
		if item.ID == 0 {
			return errors.New("模板快照包含无效指标ID")
		}
		if _, exists := seenItems[item.ID]; exists {
			return fmt.Errorf("模板快照包含重复指标ID: %d", item.ID)
		}
		seenItems[item.ID] = struct{}{}
		if item.MaxScore <= 0 {
			return fmt.Errorf("指标%d满分无效", item.ID)
		}
	}
	seenScorers := map[string]struct{}{}
	for _, scorer := range snapshot.ResolvedScorers {
		if scorer.UserID == 0 || strings.TrimSpace(scorer.ScorerType) == "" {
			return errors.New("模板快照包含无效评分人")
		}
		if snapshot.CalculationVersion == canonicalCalculationVersion && scorer.ItemID == 0 {
			return errors.New("新版模板快照必须记录指标级评分人")
		}
		key := fmt.Sprintf("%d:%s:%d", scorer.ItemID, scorer.ScorerType, scorer.UserID)
		if _, exists := seenScorers[key]; exists {
			return fmt.Errorf("模板快照包含重复评分人: %s", key)
		}
		seenScorers[key] = struct{}{}
	}
	seenSigners := map[string]struct{}{}
	for _, rule := range snapshot.SignoffRules {
		signer := strings.ToUpper(strings.TrimSpace(rule.SignerType))
		if signer == "" || rule.Sequence <= 0 {
			return errors.New("模板快照包含无效会签规则")
		}
		if _, exists := seenSigners[signer]; exists {
			return fmt.Errorf("模板快照包含重复会签人: %s", signer)
		}
		seenSigners[signer] = struct{}{}
	}
	return nil
}
