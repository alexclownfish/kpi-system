package models

import (
	"time"
)

// Role represents a business role. Code is stable and used by permission checks.
type Role struct {
	ID               uint         `json:"id" gorm:"primaryKey"`
	Code             string       `json:"code" gorm:"uniqueIndex;not null"`
	Name             string       `json:"name" gorm:"not null"`
	Description      string       `json:"description"`
	IsSystem         bool         `json:"is_system" gorm:"default:false"`
	RequiresManager  bool         `json:"requires_manager" gorm:"default:false"`
	AccessCustomized bool         `json:"-" gorm:"default:false"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
	Permissions      []Permission `json:"permissions,omitempty" gorm:"many2many:role_permissions"`
}

// Permission is a resource/action capability, for example employee:view.
type Permission struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Code        string    `json:"code" gorm:"uniqueIndex;not null"`
	Name        string    `json:"name" gorm:"not null"`
	Resource    string    `json:"resource" gorm:"not null"`
	Action      string    `json:"action" gorm:"not null"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UserRole struct {
	UserID    uint      `json:"user_id" gorm:"primaryKey"`
	RoleID    uint      `json:"role_id" gorm:"primaryKey;index"`
	CreatedAt time.Time `json:"created_at"`
}

type RolePermission struct {
	RoleID       uint      `json:"role_id" gorm:"primaryKey"`
	PermissionID uint      `json:"permission_id" gorm:"primaryKey"`
	CreatedAt    time.Time `json:"created_at"`
}

type DataScope struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	Code        string `json:"code" gorm:"uniqueIndex;not null"`
	Name        string `json:"name" gorm:"not null"`
	Description string `json:"description"`
}

type RoleDataScope struct {
	RoleID      uint      `json:"role_id" gorm:"primaryKey"`
	DataScopeID uint      `json:"data_scope_id" gorm:"primaryKey;index"`
	CreatedAt   time.Time `json:"created_at"`
}

type AuditLog struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	UserID     uint      `json:"user_id" gorm:"index"`
	Action     string    `json:"action" gorm:"not null"`
	Resource   string    `json:"resource" gorm:"not null"`
	ResourceID string    `json:"resource_id"`
	IPAddress  string    `json:"ip_address"`
	Result     string    `json:"result" gorm:"not null"`
	Details    string    `json:"details"`
	CreatedAt  time.Time `json:"created_at"`
}

// 部门模型
type Department struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"not null"`
	Description string    `json:"description"`
	ParentID    *uint     `json:"parent_id" gorm:"index"`
	LeaderID    *uint     `json:"leader_id" gorm:"index"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// 关联关系
	Employees []Employee `json:"employees,omitempty" gorm:"foreignKey:DepartmentID"`
}

// JobPosition is a department-specific business position, not an access role.
type JobPosition struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	DepartmentID uint      `json:"department_id" gorm:"not null;uniqueIndex:idx_department_position"`
	Name         string    `json:"name" gorm:"not null;uniqueIndex:idx_department_position"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type BusinessInitialization struct {
	Key          string `gorm:"primaryKey"`
	SourceSHA256 string `gorm:"not null"`
	CreatedAt    time.Time
}

// SourceWorkbook and its child diagnostics preserve the immutable provenance
// of a workbook validation/import. They are additive tables so old SQLite
// deployments and existing templates remain readable.
type SourceWorkbook struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	SourceName         string    `json:"source_name" gorm:"not null"`
	SourceSHA256       string    `json:"source_sha256" gorm:"size:64;not null;uniqueIndex"`
	ParserVersion      string    `json:"parser_version" gorm:"size:32;not null"`
	UploadedBy         uint      `json:"uploaded_by" gorm:"index"`
	Status             string    `json:"status" gorm:"size:16;not null;index"`
	SheetCount         int       `json:"sheet_count"`
	SourceMetadataJSON string    `json:"-" gorm:"type:text"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type SourceSheet struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	WorkbookID       uint      `json:"workbook_id" gorm:"not null;uniqueIndex:idx_source_sheet_name"`
	Name             string    `json:"name" gorm:"not null;uniqueIndex:idx_source_sheet_name"`
	Kind             string    `json:"kind" gorm:"size:32;not null"`
	SheetOrder       int       `json:"sheet_order"`
	SourceRange      string    `json:"source_range,omitempty"`
	DiagnosticStatus string    `json:"diagnostic_status" gorm:"size:16;not null"`
	MetadataJSON     string    `json:"-" gorm:"type:text"`
	CreatedAt        time.Time `json:"created_at"`
}

type DepartmentMapping struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	WorkbookID      uint      `json:"workbook_id" gorm:"not null;index"`
	SourceLabel     string    `json:"source_label" gorm:"not null"`
	NormalizedLabel string    `json:"normalized_label" gorm:"not null"`
	DepartmentID    *uint     `json:"department_id,omitempty" gorm:"index"`
	MappingReason   string    `json:"mapping_reason" gorm:"size:32;not null"`
	RawValue        string    `json:"raw_value"`
	IsAmbiguous     bool      `json:"is_ambiguous"`
	CreatedAt       time.Time `json:"created_at"`
}

type ImportDiagnostic struct {
	ID              uint   `json:"id" gorm:"primaryKey"`
	WorkbookID      uint   `json:"workbook_id" gorm:"not null;index"`
	SheetID         *uint  `json:"sheet_id,omitempty" gorm:"index"`
	Severity        string `json:"severity" gorm:"size:16;not null;index"`
	Code            string `json:"code" gorm:"size:96;not null"`
	Coordinate      string `json:"coordinate,omitempty"`
	RawValue        string `json:"raw_value,omitempty"`
	NormalizedValue string `json:"normalized_value,omitempty"`
	Message         string `json:"message" gorm:"not null"`
	RepairHint      string `json:"repair_hint,omitempty"`
	// Presentation-only provenance fields. They are hydrated from SourceSheet
	// and FormulaCell when a diagnostic response is serialized; keeping them
	// out of the table preserves compatibility with existing SQLite volumes.
	Sheet           string    `json:"sheet,omitempty" gorm:"-"`
	Cell            string    `json:"cell,omitempty" gorm:"-"`
	PeriodType      string    `json:"period_type,omitempty" gorm:"-"`
	Formula         string    `json:"formula,omitempty" gorm:"-"`
	ExpandedFormula string    `json:"expanded_formula,omitempty" gorm:"-"`
	IsRealDataRow   bool      `json:"is_real_data_row,omitempty" gorm:"-"`
	CreatedAt       time.Time `json:"created_at"`
}

type FormulaCell struct {
	ID                     uint      `json:"id" gorm:"primaryKey"`
	WorkbookID             uint      `json:"workbook_id" gorm:"not null;index"`
	SheetID                *uint     `json:"sheet_id,omitempty" gorm:"index"`
	Coordinate             string    `json:"coordinate" gorm:"not null"`
	FormulaType            string    `json:"formula_type" gorm:"size:32;not null"`
	SharedIndex            string    `json:"shared_index,omitempty"`
	SharedMasterCoordinate string    `json:"shared_master_coordinate,omitempty"`
	SharedRef              string    `json:"shared_ref,omitempty"`
	RawFormula             string    `json:"raw_formula,omitempty"`
	ExpandedFormula        string    `json:"expanded_formula,omitempty"`
	CachedValue            string    `json:"cached_value,omitempty"`
	DiagnosticStatus       string    `json:"diagnostic_status" gorm:"size:16;not null"`
	CreatedAt              time.Time `json:"created_at"`
}

type HRAggregationRule struct {
	ID                  uint      `json:"id" gorm:"primaryKey"`
	WorkbookID          uint      `json:"workbook_id" gorm:"not null;index"`
	SheetID             *uint     `json:"sheet_id,omitempty" gorm:"index"`
	PeriodType          string    `json:"period_type" gorm:"size:16;not null"`
	FieldName           string    `json:"field_name" gorm:"not null"`
	FormulaCell         string    `json:"formula_cell"`
	ExpandedFormula     string    `json:"expanded_formula"`
	AllocationThreshold float64   `json:"allocation_threshold"`
	DiagnosticStatus    string    `json:"diagnostic_status" gorm:"size:16;not null"`
	SourceRow           int       `json:"source_row"`
	CreatedAt           time.Time `json:"created_at"`
}

// 员工模型
type Employee struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	Name          string    `json:"name" gorm:"not null"`
	Email         string    `json:"email" gorm:"unique;not null"`
	DooTaskUserID *uint     `json:"-"`                 // DooTask用户ID，JSON中不返回
	Password      string    `json:"-" gorm:"not null"` // 密码，JSON中不返回
	Position      string    `json:"position"`
	DepartmentID  uint      `json:"department_id"`
	ManagerID     *uint     `json:"manager_id"`                   // 直属上级ID，可以为空
	Role          string    `json:"role" gorm:"default:employee"` // employee, manager, hr
	IsActive      bool      `json:"is_active" gorm:"default:true"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// 关联关系
	Department   Department `json:"department,omitempty" gorm:"foreignKey:DepartmentID"`
	Manager      *Employee  `json:"manager,omitempty" gorm:"foreignKey:ManagerID"`
	Subordinates []Employee `json:"subordinates,omitempty" gorm:"foreignKey:ManagerID"`
}

// KPI模板模型
type KPITemplate struct {
	ID                 uint   `json:"id" gorm:"primaryKey"`
	Name               string `json:"name" gorm:"not null"`
	Description        string `json:"description"`
	Period             string `json:"period"` // monthly, quarterly, yearly
	DepartmentID       *uint  `json:"department_id,omitempty"`
	PositionType       string `json:"position_type"`
	Version            int    `json:"version" gorm:"default:1"`
	Status             string `json:"status" gorm:"size:16;default:draft"`
	CalculationVersion string `json:"calculation_version" gorm:"size:32;default:v1"`
	// SourceCatalogVersion/SourceIssuesJSON preserve the imported workbook
	// provenance and diagnostics. SourceBlocked prevents an anomalous import
	// from being published until HR has reviewed and corrected it.
	SourceCatalogVersion string `json:"source_catalog_version,omitempty" gorm:"size:64"`
	// SourceSHA256 is the immutable source-workbook digest used by new
	// assessments to prove which imported business rules were applied. It is
	// optional for legacy and hand-authored templates.
	SourceSHA256     string           `json:"source_sha256,omitempty" gorm:"size:64;index"`
	SourceIssuesJSON string           `json:"-" gorm:"type:text"`
	SourceIssues     []map[string]any `json:"source_issues,omitempty" gorm:"-"`
	SourceBlocked    bool             `json:"source_blocked" gorm:"default:false"`
	// WorkflowRulesJSON preserves the imported process definition for this
	// version; hand-authored templates fall back to system defaults.
	WorkflowRulesJSON string `json:"-" gorm:"type:text"`
	// Templates become active only through publication. A true database default
	// overrides an explicit Go false value during GORM Create, activating drafts.
	IsActive         bool               `json:"is_active" gorm:"default:false"`
	ExportLayoutJSON string             `json:"-" gorm:"type:text"`
	ExportLayout     ResultExportLayout `json:"export_layout" gorm:"-"`
	CreatedAt        time.Time          `json:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at"`

	// 关联关系
	Items      []KPIItem      `json:"items,omitempty" gorm:"foreignKey:TemplateID"`
	Dimensions []KPIDimension `json:"dimensions,omitempty" gorm:"foreignKey:TemplateID"`
}

// KPIDimension groups indicators and owns their top-level weight.
type KPIDimension struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	TemplateID  uint      `json:"template_id" gorm:"not null;index"`
	Name        string    `json:"name" gorm:"not null"`
	Description string    `json:"description"`
	Weight      float64   `json:"weight"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ResultExportLayout is a controlled, versioned document layout attached to a
// KPI template and copied into finalized result snapshots.
type ResultExportLayout struct {
	Version             int      `json:"version"`
	Preset              string   `json:"preset"`
	Title               string   `json:"title"`
	Orientation         string   `json:"orientation"`
	Columns             []string `json:"columns"`
	ShowSummary         bool     `json:"show_summary"`
	ShowEmployeeOpinion bool     `json:"show_employee_opinion"`
	SignatureLabels     []string `json:"signature_labels"`
}

// KPI考核项目模型
type KPIItem struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	TemplateID      uint      `json:"template_id"`
	Name            string    `json:"name" gorm:"not null"`
	Description     string    `json:"description"`
	MaxScore        float64   `json:"max_score"` // 满分
	Order           int       `json:"order"`     // 排序
	DimensionID     *uint     `json:"dimension_id,omitempty"`
	ItemWeight      float64   `json:"item_weight"`
	CompositeWeight float64   `json:"composite_weight"`
	ScoringGuidance string    `json:"scoring_guidance"`
	DataSource      string    `json:"data_source"`
	ScorerTypesJSON string    `json:"-" gorm:"type:text"`
	ScorerTypes     []string  `json:"scorer_types" gorm:"-"`
	Required        bool      `json:"required" gorm:"default:true"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// 关联关系
	Template  KPITemplate   `json:"template,omitempty" gorm:"foreignKey:TemplateID"`
	Dimension *KPIDimension `json:"dimension,omitempty" gorm:"foreignKey:DimensionID"`
}

// KPI评估记录模型
type KPIEvaluation struct {
	ID                     uint       `json:"id" gorm:"primaryKey"`
	EmployeeID             uint       `json:"employee_id"`
	TemplateID             uint       `json:"template_id"`
	Period                 string     `json:"period"` // 2024-01, 2024-Q1, 2024
	Year                   int        `json:"year"`
	Month                  *int       `json:"month,omitempty"`
	Quarter                *int       `json:"quarter,omitempty"`
	Status                 string     `json:"status" gorm:"default:pending"` // pending, self_evaluated, manager_evaluated, pending_confirm, completed
	TotalScore             float64    `json:"total_score"`
	FinalComment           string     `json:"final_comment"`
	HasObjection           bool       `json:"has_objection" gorm:"default:false"` // 是否有异议
	ObjectionReason        string     `json:"objection_reason"`                   // 异议原因（员工填写）
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
	TemplateVersion        int        `json:"template_version"`
	CalculationVersion     string     `json:"calculation_version" gorm:"size:32"`
	TemplateSnapshotJSON   string     `json:"-" gorm:"type:text"`
	ProjectName            string     `json:"project_name"`
	AllocationRatio        *float64   `json:"allocation_ratio,omitempty"`
	WorkSummaryRequired    bool       `json:"work_summary_required" gorm:"default:false"`
	WorkSummary            string     `json:"work_summary"`
	WorkRisks              string     `json:"work_risks"`
	SupportNeeded          string     `json:"support_needed"`
	NextPlan               string     `json:"next_plan"`
	WorkSummarySubmittedAt *time.Time `json:"work_summary_submitted_at,omitempty"`

	// 关联关系
	Employee Employee    `json:"employee,omitempty" gorm:"foreignKey:EmployeeID"`
	Template KPITemplate `json:"template,omitempty" gorm:"foreignKey:TemplateID"`
	Scores   []KPIScore  `json:"scores,omitempty" gorm:"foreignKey:EvaluationID"`
}

// EvaluationResponsibility assigns a business scoring responsibility to a
// user independently from RBAC roles. Scope fields determine future scorer
// resolution; existing evaluation tasks retain their snapshotted assignee.
type EvaluationResponsibility struct {
	ID            uint       `json:"id" gorm:"primaryKey"`
	Code          string     `json:"code" gorm:"size:32;not null;index;uniqueIndex:idx_evaluation_responsibility_assignment"`
	Name          string     `json:"name" gorm:"not null"`
	ScopeType     string     `json:"scope_type" gorm:"size:16;not null;uniqueIndex:idx_evaluation_responsibility_assignment"`
	DepartmentID  *uint      `json:"department_id,omitempty" gorm:"uniqueIndex:idx_evaluation_responsibility_assignment"`
	PositionType  string     `json:"position_type" gorm:"size:128;uniqueIndex:idx_evaluation_responsibility_assignment"`
	ProjectID     *uint      `json:"project_id,omitempty" gorm:"uniqueIndex:idx_evaluation_responsibility_assignment"`
	UserID        uint       `json:"user_id" gorm:"not null;index;uniqueIndex:idx_evaluation_responsibility_assignment"`
	Active        bool       `json:"active" gorm:"not null;default:true;index"`
	EffectiveFrom *time.Time `json:"effective_from,omitempty"`
	EffectiveTo   *time.Time `json:"effective_to,omitempty"`
	CreatedBy     uint       `json:"created_by" gorm:"not null"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	User       Employee    `json:"user,omitempty" gorm:"foreignKey:UserID"`
	Department *Department `json:"department,omitempty" gorm:"foreignKey:DepartmentID"`
}

// EvaluationScoringTask is the authoritative item-level assignment for new
// workflow evaluations. A numeric zero is a valid score; nil means no score.
type EvaluationScoringTask struct {
	ID            uint       `json:"id" gorm:"primaryKey"`
	EvaluationID  uint       `json:"evaluation_id" gorm:"not null;index;uniqueIndex:idx_evaluation_item_scorer"`
	ItemID        uint       `json:"item_id" gorm:"not null;index;uniqueIndex:idx_evaluation_item_scorer"`
	ScorerUserID  uint       `json:"scorer_user_id" gorm:"not null;index;uniqueIndex:idx_evaluation_item_scorer"`
	ScorerType    string     `json:"scorer_type" gorm:"size:32;not null;index"`
	Sequence      int        `json:"sequence" gorm:"not null;default:0;index"`
	Required      bool       `json:"required" gorm:"not null;default:true"`
	Status        string     `json:"status" gorm:"size:24;not null;default:pending;index"`
	Score         *float64   `json:"score,omitempty"`
	Comment       string     `json:"comment" gorm:"type:text"`
	NotApplicable bool       `json:"not_applicable" gorm:"not null;default:false"`
	SubmittedAt   *time.Time `json:"submitted_at,omitempty"`
	LockedAt      *time.Time `json:"locked_at,omitempty"`
	Version       int        `json:"version" gorm:"not null;default:1"`
	SourceRule    string     `json:"source_rule" gorm:"type:text"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	Evaluation KPIEvaluation `json:"evaluation,omitempty" gorm:"foreignKey:EvaluationID"`
	Item       KPIItem       `json:"item,omitempty" gorm:"foreignKey:ItemID"`
	Scorer     Employee      `json:"scorer,omitempty" gorm:"foreignKey:ScorerUserID"`
}

// EvaluationCalculationResult stores a versioned canonical result generated
// from the immutable assessment snapshot and submitted scoring tasks.
type EvaluationCalculationResult struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	EvaluationID       uint      `json:"evaluation_id" gorm:"not null;index;uniqueIndex:idx_evaluation_calculation_version"`
	Version            int       `json:"version" gorm:"not null;uniqueIndex:idx_evaluation_calculation_version"`
	CalculationVersion string    `json:"calculation_version" gorm:"size:32;not null;index"`
	SnapshotChecksum   string    `json:"snapshot_checksum" gorm:"size:64;not null"`
	ResultJSON         string    `json:"-" gorm:"type:text;not null"`
	TotalScore         float64   `json:"total_score" gorm:"not null"`
	Grade              string    `json:"grade" gorm:"size:16"`
	CalculatedAt       time.Time `json:"calculated_at" gorm:"not null"`
	CalculatedBy       *uint     `json:"calculated_by,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

// AssessmentAuditFinding is a stable finding from a retained audit run. The
// preview audit may return the same shape without persisting it.
type AssessmentAuditFinding struct {
	ID                   string    `json:"id" gorm:"primaryKey;size:96"`
	RunID                string    `json:"run_id" gorm:"size:36;not null;index"`
	EvaluationID         *uint     `json:"evaluation_id,omitempty" gorm:"index"`
	Category             string    `json:"category" gorm:"size:32;not null;index"`
	Severity             string    `json:"severity" gorm:"size:24;not null;index"`
	Code                 string    `json:"code" gorm:"size:64;not null;index"`
	Message              string    `json:"message" gorm:"not null"`
	DetailsJSON          string    `json:"-" gorm:"type:text"`
	EligibleForMigration bool      `json:"eligible_for_migration" gorm:"not null;default:false"`
	CreatedAt            time.Time `json:"created_at"`
}

// MigrationBatch records a reviewed dry-run or an applied migration. Apply
// handlers must revalidate the immutable payload before writing business data.
type MigrationBatch struct {
	ID            string     `json:"id" gorm:"primaryKey;size:36"`
	Mode          string     `json:"mode" gorm:"size:16;not null;index"`
	Status        string     `json:"status" gorm:"size:16;not null;index"`
	SourceVersion string     `json:"source_version" gorm:"size:32;not null"`
	TargetVersion string     `json:"target_version" gorm:"size:32;not null"`
	FiltersJSON   string     `json:"-" gorm:"type:text;not null"`
	PayloadJSON   string     `json:"-" gorm:"type:text;not null"`
	SummaryJSON   string     `json:"-" gorm:"type:text;not null"`
	OperatorID    uint       `json:"operator_id" gorm:"not null;index"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// EvaluationResultSnapshot is an immutable, checksummed version of a finalized
// evaluation result used for exports and sign-off evidence.
type EvaluationResultSnapshot struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	EvaluationID uint      `json:"evaluation_id" gorm:"not null;uniqueIndex:idx_evaluation_snapshot_version;uniqueIndex:idx_evaluation_snapshot_checksum"`
	Version      int       `json:"version" gorm:"not null;uniqueIndex:idx_evaluation_snapshot_version"`
	SnapshotJSON string    `json:"-" gorm:"type:text;not null"`
	Checksum     string    `json:"checksum" gorm:"size:64;not null;uniqueIndex:idx_evaluation_snapshot_checksum"`
	CreatedBy    uint      `json:"created_by" gorm:"not null"`
	CreatedAt    time.Time `json:"created_at"`
}

// EvaluationConfirmation records the single effective final confirmation for
// an evaluation, whether completed online or by archived paper signature.
type EvaluationConfirmation struct {
	ID                     uint                     `json:"id" gorm:"primaryKey"`
	EvaluationID           uint                     `json:"evaluation_id" gorm:"not null;uniqueIndex"`
	SnapshotID             uint                     `json:"snapshot_id" gorm:"not null;uniqueIndex"`
	Method                 string                   `json:"method" gorm:"size:16;not null"`
	ConfirmedBy            uint                     `json:"confirmed_by" gorm:"not null"`
	ConfirmedAt            time.Time                `json:"confirmed_at" gorm:"not null"`
	SignedAt               *time.Time               `json:"signed_at,omitempty"`
	HandledBy              *uint                    `json:"handled_by,omitempty"`
	AttachmentStoredName   string                   `json:"-"`
	AttachmentOriginalName string                   `json:"attachment_name,omitempty"`
	AttachmentContentType  string                   `json:"attachment_content_type,omitempty"`
	AttachmentSize         int64                    `json:"attachment_size,omitempty"`
	Remark                 string                   `json:"remark" gorm:"size:500"`
	CreatedAt              time.Time                `json:"created_at"`
	UpdatedAt              time.Time                `json:"updated_at"`
	Snapshot               EvaluationResultSnapshot `json:"snapshot" gorm:"foreignKey:SnapshotID"`
	Confirmer              Employee                 `json:"confirmer,omitempty" gorm:"foreignKey:ConfirmedBy"`
	Handler                *Employee                `json:"handler,omitempty" gorm:"foreignKey:HandledBy"`
}

// EvaluationSignoffSupplement stores a paper-signature attachment uploaded
// after an evaluation was already finalized online. It is deliberately
// separate from EvaluationConfirmation so the original confirmation method,
// timestamp and snapshot remain immutable.
type EvaluationSignoffSupplement struct {
	ID                     uint       `json:"id" gorm:"primaryKey"`
	EvaluationID           uint       `json:"evaluation_id" gorm:"not null;index"`
	UploadedBy             uint       `json:"uploaded_by" gorm:"not null;index"`
	UploadedAt             time.Time  `json:"uploaded_at" gorm:"not null"`
	SignedAt               *time.Time `json:"signed_at,omitempty"`
	AttachmentStoredName   string     `json:"-" gorm:"not null"`
	AttachmentOriginalName string     `json:"attachment_name,omitempty"`
	AttachmentContentType  string     `json:"attachment_content_type,omitempty"`
	AttachmentSize         int64      `json:"attachment_size,omitempty"`
	Remark                 string     `json:"remark" gorm:"size:500"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
	Uploader               Employee   `json:"uploader,omitempty" gorm:"foreignKey:UploadedBy"`
}

// FinalScoreImportBatch stores a server-validated preview so commit requests
// cannot replace the reviewed workbook contents with client supplied data.
type FinalScoreImportBatch struct {
	ID          string     `json:"id" gorm:"primaryKey;size:36"`
	CreatedBy   uint       `json:"created_by" gorm:"not null;index"`
	FileName    string     `json:"file_name" gorm:"not null"`
	Status      string     `json:"status" gorm:"size:16;not null;index"`
	PayloadJSON string     `json:"-" gorm:"type:text;not null"`
	SummaryJSON string     `json:"-" gorm:"type:text;not null"`
	ExpiresAt   time.Time  `json:"expires_at" gorm:"not null;index"`
	CommittedAt *time.Time `json:"committed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// KPI具体得分模型
type KPIScore struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	EvaluationID   uint      `json:"evaluation_id"`
	ItemID         uint      `json:"item_id"`
	SelfScore      *float64  `json:"self_score,omitempty"`              // 自评分数
	SelfComment    string    `json:"self_comment"`                      // 自评评价
	ManagerScore   *float64  `json:"manager_score,omitempty"`           // 上级评分
	ManagerComment string    `json:"manager_comment"`                   // 上级评价
	ManagerAuto    bool      `json:"manager_auto" gorm:"default:false"` // 是否自动填入上级评分
	HRScore        *float64  `json:"hr_score,omitempty"`                // HR评分
	HRComment      string    `json:"hr_comment"`                        // HR评价
	FinalScore     *float64  `json:"final_score,omitempty"`             // 最终得分
	FinalComment   string    `json:"final_comment"`                     // 最终得分说明
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// 关联关系
	Evaluation KPIEvaluation `json:"evaluation,omitempty" gorm:"foreignKey:EvaluationID"`
	Item       KPIItem       `json:"item,omitempty" gorm:"foreignKey:ItemID"`
}

// 评论模型
type EvaluationComment struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	EvaluationID uint      `json:"evaluation_id"`
	UserID       uint      `json:"user_id"` // 评论者ID
	Content      string    `json:"content" gorm:"not null"`
	IsPrivate    bool      `json:"is_private" gorm:"default:false"` // 是否仅自己可见
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// 关联关系
	Evaluation KPIEvaluation `json:"evaluation,omitempty" gorm:"foreignKey:EvaluationID"`
	User       Employee      `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// 系统设置模型
type SystemSetting struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Key       string    `json:"key" gorm:"unique;not null"`
	Value     string    `json:"value" gorm:"not null"`
	Type      string    `json:"type" gorm:"default:string"` // string, boolean, number, json
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// 评估邀请模型
type EvaluationInvitation struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	EvaluationID uint      `json:"evaluation_id"`
	InviterID    uint      `json:"inviter_id"`                    // 邀请者ID（HR）
	InviteeID    uint      `json:"invitee_id"`                    // 被邀请者ID
	Status       string    `json:"status" gorm:"default:pending"` // pending, accepted, declined, completed
	Message      string    `json:"message"`                       // 邀请消息
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// 关联关系
	Evaluation KPIEvaluation  `json:"evaluation,omitempty" gorm:"foreignKey:EvaluationID"`
	Inviter    Employee       `json:"inviter,omitempty" gorm:"foreignKey:InviterID"`
	Invitee    Employee       `json:"invitee,omitempty" gorm:"foreignKey:InviteeID"`
	Scores     []InvitedScore `json:"scores,omitempty" gorm:"foreignKey:InvitationID"`
}

// 邀请评分模型
type InvitedScore struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	InvitationID uint      `json:"invitation_id"`
	ItemID       uint      `json:"item_id"`
	Score        *float64  `json:"score,omitempty"` // 邀请评分
	Comment      string    `json:"comment"`         // 邀请评价说明
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// 关联关系
	Invitation EvaluationInvitation `json:"invitation,omitempty" gorm:"foreignKey:InvitationID"`
	Item       KPIItem              `json:"item,omitempty" gorm:"foreignKey:ItemID"`
}

// 绩效评分规则模型
type PerformanceRule struct {
	ID             uint                        `json:"id" gorm:"primaryKey"`
	NoInvitation   PerformanceRuleNoInvitation `json:"no_invitation" gorm:"embedded;embeddedPrefix:no_invitation_"`
	WithInvitation PerformanceRuleWithInvite   `json:"with_invitation" gorm:"embedded;embeddedPrefix:with_invitation_"`
	Enabled        bool                        `json:"enabled" gorm:"not null;default:false"`
	CreatedAt      time.Time                   `json:"created_at"`
	UpdatedAt      time.Time                   `json:"updated_at"`
}

// 无邀请评分规则
type PerformanceRuleNoInvitation struct {
	SelfWeight     float64 `json:"self_weight" gorm:"not null;default:10"`
	SuperiorWeight float64 `json:"superior_weight" gorm:"not null;default:90"`
}

// 有邀请评分规则
type PerformanceRuleWithInvite struct {
	Employee PerformanceRuleEmployee `json:"employee" gorm:"embedded;embeddedPrefix:employee_"`
}

// 员工邀请评分规则
type PerformanceRuleEmployee struct {
	SelfWeight           float64 `json:"self_weight" gorm:"not null;default:10"`
	InviteSuperiorWeight float64 `json:"invite_superior_weight" gorm:"not null;default:30"`
	SuperiorWeight       float64 `json:"superior_weight" gorm:"not null;default:60"`
}

// DefaultPerformanceRule 返回默认绩效规则配置
func DefaultPerformanceRule() PerformanceRule {
	return PerformanceRule{
		Enabled: false,
		NoInvitation: PerformanceRuleNoInvitation{
			SelfWeight:     10,
			SuperiorWeight: 90,
		},
		WithInvitation: PerformanceRuleWithInvite{
			Employee: PerformanceRuleEmployee{
				SelfWeight:           10,
				InviteSuperiorWeight: 30,
				SuperiorWeight:       60,
			},
		},
	}
}
