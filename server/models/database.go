package models

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// 初始化数据库连接
func InitDB() {
	var err error
	DB, err = openConfiguredDatabase()
	if err != nil {
		log.Fatal("数据库连接失败: ", err)
	}

	log.Println("数据库连接成功")

	if err := migrateSchema(DB); err != nil {
		log.Fatal("数据库迁移失败: ", err)
	}
	if err := repairTemplateActivationState(DB); err != nil {
		log.Fatal("模板启用状态修复失败:", err)
	}
	if err := repairScoringTaskSequence(DB); err != nil {
		log.Fatal("评分任务顺位修复失败:", err)
	}
	seedAccessControl()
	repairAccessControlConstraints()

	log.Println("数据库表迁移完成")
}

// InitSchema creates the PostgreSQL schema for a one-time data migration.
// It intentionally does not seed roles, repair records, or bootstrap accounts.
func InitSchema() error {
	if strings.ToLower(strings.TrimSpace(os.Getenv("DB_DRIVER"))) != "postgres" {
		return errors.New("init-schema 需要 DB_DRIVER=postgres")
	}
	db, err := openConfiguredDatabase()
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	return migrateSchema(db)
}

func openConfiguredDatabase() (*gorm.DB, error) {
	dialector, err := configuredDatabaseDialector()
	if err != nil {
		return nil, fmt.Errorf("数据库配置无效: %w", err)
	}
	logLevel := logger.Info
	if isProduction() {
		logLevel = logger.Warn
	}
	connectLevel := logLevel
	if dialector.Name() == "postgres" {
		// Connection errors can include the DSN; suppress GORM's open-time logger.
		connectLevel = logger.Silent
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(connectLevel)})
	if err != nil {
		if dialector.Name() == "postgres" {
			return nil, errors.New("PostgreSQL 数据库连接失败；请检查连接参数及服务状态")
		}
		return nil, err
	}
	db.Config.Logger = logger.Default.LogMode(logLevel)
	return db, nil
}

func migrateSchema(db *gorm.DB) error {
	return db.AutoMigrate(
		&Role{},
		&Permission{},
		&UserRole{},
		&RolePermission{},
		&DataScope{},
		&RoleDataScope{},
		&AuditLog{},
		&Department{},
		&JobPosition{},
		&BusinessInitialization{},
		&SourceWorkbook{},
		&SourceSheet{},
		&DepartmentMapping{},
		&ImportDiagnostic{},
		&FormulaCell{},
		&HRAggregationRule{},
		&Employee{},
		&KPITemplate{},
		&KPIDimension{},
		&KPIItem{},
		&KPIEvaluation{},
		&KPIScore{},
		&EvaluationResponsibility{},
		&EvaluationScoringTask{},
		&EvaluationCalculationResult{},
		&AssessmentAuditFinding{},
		&MigrationBatch{},
		&EvaluationResultSnapshot{},
		&EvaluationConfirmation{},
		&EvaluationSignoffSupplement{},
		&FinalScoreImportBatch{},
		&EvaluationComment{},
		&EvaluationInvitation{},
		&InvitedScore{},
		&SystemSetting{},
		&PerformanceRule{},
	)
}

// DB_DRIVER must be explicit for PostgreSQL. Existing installations retain
// the SQLite default; a failed PostgreSQL connection never falls back to it.
func configuredDatabaseDialector() (gorm.Dialector, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DB_DRIVER"))) {
	case "", "sqlite":
		if err := os.MkdirAll("db", 0755); err != nil {
			return nil, fmt.Errorf("创建 SQLite 数据目录失败: %w", err)
		}
		return sqlite.Open("db/kpi.db"), nil
	case "postgres":
		dsn, err := postgresDSNFromEnv()
		if err != nil {
			return nil, err
		}
		return postgres.Open(dsn), nil
	default:
		return nil, errors.New("DB_DRIVER 只能是 sqlite 或 postgres")
	}
}

func postgresDSNFromEnv() (string, error) {
	host := strings.TrimSpace(os.Getenv("PGHOST"))
	user := strings.TrimSpace(os.Getenv("PGUSER"))
	database := strings.TrimSpace(os.Getenv("PGDATABASE"))
	passwordFile := strings.TrimSpace(os.Getenv("PGPASSWORD_FILE"))
	if host == "" || user == "" || database == "" || passwordFile == "" {
		return "", errors.New("PostgreSQL 需要 PGHOST、PGUSER、PGDATABASE 和 PGPASSWORD_FILE")
	}
	port := strings.TrimSpace(os.Getenv("PGPORT"))
	if port == "" {
		port = "5432"
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", errors.New("PGPORT 必须是 1 到 65535 之间的端口")
	}
	passwordBytes, err := os.ReadFile(passwordFile)
	if err != nil {
		return "", errors.New("无法读取 PGPASSWORD_FILE")
	}
	password := strings.TrimRight(string(passwordBytes), "\r\n")
	if password == "" {
		return "", errors.New("PGPASSWORD_FILE 不能为空")
	}
	sslmode := strings.TrimSpace(os.Getenv("PGSSLMODE"))
	if sslmode == "" {
		sslmode = "disable" // Docker Compose uses its private network.
	}
	options := url.Values{"sslmode": {sslmode}, "connect_timeout": {"5"}}
	dsn := &url.URL{
		Scheme: "postgres", User: url.UserPassword(user, password),
		Host: net.JoinHostPort(host, port), Path: "/" + database,
		RawQuery: options.Encode(),
	}
	return dsn.String(), nil
}

func repairScoringTaskSequence(db *gorm.DB) error {
	return db.Exec(`
		UPDATE evaluation_scoring_tasks
		SET sequence = CASE UPPER(scorer_type)
			WHEN 'PM' THEN 1
			WHEN 'PRODUCT_MANAGER' THEN 3
			ELSE 2
		END
		WHERE sequence = 0
	`).Error
}

// repairTemplateActivationState fixes only the inconsistent activation flag
// left by the former default:true column. Published templates and historical
// assessment snapshots remain untouched; each repaired template is audited.
func repairTemplateActivationState(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var templates []KPITemplate
		if err := tx.Select("id", "status").
			Where("status IN ? AND is_active = ?", []string{"draft", "retired"}, true).
			Find(&templates).Error; err != nil {
			return err
		}
		for _, template := range templates {
			result := tx.Model(&KPITemplate{}).
				Where("id = ? AND status = ? AND is_active = ?", template.ID, template.Status, true).
				Update("is_active", false)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				continue
			}
			if err := tx.Create(&AuditLog{
				Action: "template_activation_repaired", Resource: "kpi_template",
				ResourceID: fmt.Sprint(template.ID), Result: "SUCCESS",
				Details: "status=" + template.Status + ";is_active=true->false;reason=inactive_template_default_repair",
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func isProduction() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production")
}

const (
	EvaluationWorkflowModeDisabled = "disabled"
	EvaluationWorkflowModeAudit    = "audit"
	EvaluationWorkflowModeEnabled  = "enabled"
)

// EvaluationWorkflowMode returns the explicitly configured rollout mode. An
// unknown or empty value is kept in the safe audit-only state so an ordinary
// deployment cannot start writing migrated workflow data accidentally.
func EvaluationWorkflowMode() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("EVALUATION_WORKFLOW_MODE")))
	switch mode {
	case EvaluationWorkflowModeDisabled, EvaluationWorkflowModeAudit, EvaluationWorkflowModeEnabled:
		return mode
	default:
		return EvaluationWorkflowModeAudit
	}
}

func EvaluationWorkflowAuditEnabled() bool {
	return EvaluationWorkflowMode() != EvaluationWorkflowModeDisabled
}

func EvaluationWorkflowWritesEnabled() bool {
	return EvaluationWorkflowMode() == EvaluationWorkflowModeEnabled
}

// seedAccessControl creates the stable MVP catalog idempotently. Business data
// can be migrated from Employee.Role without changing existing records.
func seedAccessControl() {
	roles := []Role{
		{Code: "super_admin", Name: "超级管理员", IsSystem: true}, {Code: "hr_admin", Name: "HR管理员", IsSystem: true},
		{Code: "performance_admin", Name: "绩效专员", IsSystem: true}, {Code: "department_manager", Name: "部门负责人", IsSystem: true},
		{Code: "pm", Name: "项目经理", IsSystem: true}, {Code: "product_manager", Name: "产品经理", IsSystem: true}, {Code: "cto", Name: "CTO", IsSystem: true},
		{Code: "reviewer", Name: "评审人", IsSystem: true}, {Code: "analyst", Name: "数据分析员", IsSystem: true}, {Code: "employee", Name: "普通员工", IsSystem: true},
	}
	permissions := []Permission{
		{Code: "user:view", Name: "查看用户", Resource: "user", Action: "view"}, {Code: "user:create", Name: "创建用户", Resource: "user", Action: "create"}, {Code: "user:edit", Name: "编辑用户", Resource: "user", Action: "edit"}, {Code: "user:disable", Name: "停用用户", Resource: "user", Action: "disable"},
		{Code: "employee:view", Name: "查看员工", Resource: "employee", Action: "view"}, {Code: "employee:create", Name: "创建员工", Resource: "employee", Action: "create"}, {Code: "employee:edit", Name: "编辑员工", Resource: "employee", Action: "edit"}, {Code: "employee:delete", Name: "删除员工", Resource: "employee", Action: "delete"}, {Code: "employee:assign_department", Name: "调整员工部门", Resource: "employee", Action: "assign_department"}, {Code: "employee:assign_role", Name: "分配员工角色", Resource: "employee", Action: "assign_role"},
		{Code: "department:view", Name: "查看部门", Resource: "department", Action: "view"}, {Code: "department:create", Name: "创建部门", Resource: "department", Action: "create"}, {Code: "department:edit", Name: "编辑部门", Resource: "department", Action: "edit"}, {Code: "department:delete", Name: "删除部门", Resource: "department", Action: "delete"}, {Code: "department:assign_manager", Name: "设置部门负责人", Resource: "department", Action: "assign_manager"},
		{Code: "kpi:view", Name: "查看KPI", Resource: "kpi", Action: "view"}, {Code: "kpi:create", Name: "创建KPI", Resource: "kpi", Action: "create"}, {Code: "kpi:edit", Name: "编辑KPI", Resource: "kpi", Action: "edit"}, {Code: "kpi:delete", Name: "删除KPI", Resource: "kpi", Action: "delete"}, {Code: "kpi:publish", Name: "发布KPI", Resource: "kpi", Action: "publish"}, {Code: "kpi:disable", Name: "停用KPI", Resource: "kpi", Action: "disable"},
		{Code: "assessment:view", Name: "查看考核", Resource: "assessment", Action: "view"}, {Code: "assessment:create", Name: "创建考核", Resource: "assessment", Action: "create"}, {Code: "assessment:edit", Name: "编辑考核", Resource: "assessment", Action: "edit"}, {Code: "assessment:delete", Name: "删除考核", Resource: "assessment", Action: "delete"}, {Code: "assessment:publish", Name: "发布考核", Resource: "assessment", Action: "publish"}, {Code: "assessment:submit", Name: "提交考核", Resource: "assessment", Action: "submit"}, {Code: "assessment:review", Name: "考核评分", Resource: "assessment", Action: "review"}, {Code: "assessment:approve", Name: "审核考核", Resource: "assessment", Action: "approve"},
		{Code: "review:view", Name: "查看评分任务", Resource: "review", Action: "view"}, {Code: "review:create", Name: "发起评分邀请", Resource: "review", Action: "create"}, {Code: "review:accept", Name: "接受评分邀请", Resource: "review", Action: "accept"}, {Code: "review:reject", Name: "拒绝评分邀请", Resource: "review", Action: "reject"}, {Code: "review:submit", Name: "提交评分", Resource: "review", Action: "submit"}, {Code: "review:manage", Name: "管理评分", Resource: "review", Action: "manage"},
		{Code: "report:view", Name: "查看报表", Resource: "report", Action: "view"}, {Code: "report:export", Name: "导出报表", Resource: "report", Action: "export"}, {Code: "report:company", Name: "查看公司报表", Resource: "report", Action: "company"}, {Code: "report:department", Name: "查看部门报表", Resource: "report", Action: "department"}, {Code: "report:personal", Name: "查看个人报表", Resource: "report", Action: "personal"},
		{Code: "role:view", Name: "查看角色", Resource: "role", Action: "view"}, {Code: "role:create", Name: "创建角色", Resource: "role", Action: "create"}, {Code: "role:edit", Name: "编辑角色", Resource: "role", Action: "edit"}, {Code: "role:delete", Name: "删除角色", Resource: "role", Action: "delete"}, {Code: "responsibility:view", Name: "查看评分职责", Resource: "responsibility", Action: "view"}, {Code: "responsibility:create", Name: "创建评分职责", Resource: "responsibility", Action: "create"}, {Code: "responsibility:edit", Name: "编辑评分职责", Resource: "responsibility", Action: "edit"}, {Code: "responsibility:deactivate", Name: "停用评分职责", Resource: "responsibility", Action: "deactivate"}, {Code: "system:view", Name: "查看系统设置", Resource: "system", Action: "view"}, {Code: "system:edit", Name: "修改系统设置", Resource: "system", Action: "edit"}, {Code: "audit:view", Name: "查看审计日志", Resource: "audit", Action: "view"},
	}
	for _, role := range roles {
		DB.Where("code = ?", role.Code).FirstOrCreate(&role, Role{Code: role.Code})
	}
	DB.Model(&Role{}).Where("code IN ?", []string{"super_admin", "hr_admin", "performance_admin", "department_manager", "pm", "product_manager", "cto", "reviewer", "analyst", "employee"}).Update("is_system", true)
	DB.Model(&Role{}).Where("code = ?", "employee").Update("requires_manager", true)
	for _, permission := range permissions {
		DB.Where("code = ?", permission.Code).FirstOrCreate(&permission, Permission{Code: permission.Code})
	}
	scopes := []DataScope{{Code: "SELF", Name: "本人"}, {Code: "DIRECT_SUBORDINATES", Name: "直属下属"}, {Code: "DEPARTMENT", Name: "本部门"}, {Code: "DEPARTMENT_TREE", Name: "本部门及下级"}, {Code: "ASSIGNED", Name: "被授权对象"}, {Code: "ALL", Name: "全公司"}}
	for _, scope := range scopes {
		DB.Where("code = ?", scope.Code).FirstOrCreate(&scope, DataScope{Code: scope.Code})
	}
	// Role defaults are intentionally data-driven and can be changed later.
	rolePermissions := map[string][]string{
		"super_admin":        {"user:view", "user:create", "user:edit", "user:disable", "employee:view", "employee:create", "employee:edit", "employee:delete", "employee:assign_department", "employee:assign_role", "department:view", "department:create", "department:edit", "department:delete", "department:assign_manager", "kpi:view", "kpi:create", "kpi:edit", "kpi:delete", "kpi:publish", "kpi:disable", "assessment:view", "assessment:create", "assessment:edit", "assessment:delete", "assessment:publish", "assessment:submit", "assessment:review", "assessment:approve", "review:view", "review:create", "review:accept", "review:reject", "review:submit", "review:manage", "report:view", "report:export", "report:company", "report:department", "report:personal", "role:view", "role:create", "role:edit", "role:delete", "responsibility:view", "responsibility:create", "responsibility:edit", "responsibility:deactivate", "system:view", "system:edit", "audit:view"},
		"hr_admin":           {"user:view", "user:create", "user:edit", "user:disable", "employee:view", "employee:create", "employee:edit", "employee:delete", "employee:assign_department", "employee:assign_role", "department:view", "department:create", "department:edit", "department:delete", "department:assign_manager", "kpi:view", "kpi:create", "kpi:edit", "kpi:delete", "kpi:publish", "kpi:disable", "assessment:view", "assessment:create", "assessment:edit", "assessment:delete", "assessment:publish", "assessment:review", "assessment:approve", "review:view", "review:create", "review:accept", "review:reject", "review:submit", "review:manage", "report:view", "report:export", "report:company", "report:department", "report:personal", "responsibility:view", "responsibility:create", "responsibility:edit", "responsibility:deactivate", "audit:view"},
		"performance_admin":  {"department:view", "kpi:view", "kpi:create", "kpi:edit", "kpi:delete", "kpi:publish", "kpi:disable", "assessment:view", "assessment:create", "assessment:edit", "assessment:delete", "assessment:publish", "assessment:review", "assessment:approve", "review:view", "review:create", "review:accept", "review:reject", "review:submit", "review:manage", "report:view", "report:export", "report:company", "report:department", "responsibility:view", "responsibility:create", "responsibility:edit", "responsibility:deactivate", "audit:view"},
		"department_manager": {"employee:view", "department:view", "kpi:view", "assessment:view", "assessment:create", "assessment:submit", "assessment:review", "review:view", "review:create", "review:accept", "review:reject", "review:submit", "report:view", "report:export", "report:department"},
		"pm":                 {"employee:view", "department:view", "kpi:view", "assessment:view", "assessment:submit", "assessment:review", "review:view", "review:accept", "review:submit", "report:view", "report:department"},
		"product_manager":    {"employee:view", "department:view", "kpi:view", "assessment:view", "assessment:submit", "assessment:review", "review:view", "review:accept", "review:submit", "report:view", "report:department"},
		"cto":                {"employee:view", "department:view", "kpi:view", "assessment:view", "assessment:submit", "assessment:review", "review:view", "review:accept", "review:submit", "report:view", "report:company", "report:department"},
		"reviewer":           {"review:view", "review:accept", "review:reject", "review:submit"}, "analyst": {"employee:view", "department:view", "kpi:view", "assessment:view", "report:view", "report:export", "report:company", "report:department", "report:personal"}, "employee": {"employee:view", "kpi:view", "assessment:view", "assessment:submit", "review:view", "review:create", "review:accept", "review:reject", "review:submit", "report:view", "report:personal"},
	}
	for roleCode, codes := range rolePermissions {
		var role Role
		if DB.Where("code = ?", roleCode).First(&role).Error != nil {
			continue
		}
		// An explicitly saved empty permission set is also a customization.
		if role.AccessCustomized && role.Code != "super_admin" {
			continue
		}
		for _, code := range codes {
			var p Permission
			if DB.Where("code = ?", code).First(&p).Error == nil {
				DB.Where("role_id = ? AND permission_id = ?", role.ID, p.ID).FirstOrCreate(&RolePermission{RoleID: role.ID, PermissionID: p.ID})
			}
		}
	}
	roleScopes := map[string]string{"super_admin": "ALL", "hr_admin": "ALL", "performance_admin": "ALL", "department_manager": "DEPARTMENT", "pm": "DEPARTMENT", "product_manager": "DEPARTMENT", "cto": "ALL", "reviewer": "ASSIGNED", "analyst": "ALL", "employee": "SELF"}
	for roleCode, scopeCode := range roleScopes {
		var role Role
		var scope DataScope
		if DB.Where("code = ?", roleCode).First(&role).Error == nil && DB.Where("code = ?", scopeCode).First(&scope).Error == nil {
			if role.AccessCustomized && role.Code != "super_admin" {
				continue
			}
			DB.Where("role_id = ? AND data_scope_id = ?", role.ID, scope.ID).FirstOrCreate(&RoleDataScope{RoleID: role.ID, DataScopeID: scope.ID})
		}
	}
}

// repairAccessControlConstraints keeps legacy data while enforcing the MVP's
// single-role and single-data-scope invariants. It is safe to run on every boot.
func repairAccessControlConstraints() {
	var users []uint
	DB.Model(&UserRole{}).Select("user_id").Group("user_id").Having("COUNT(*) > 1").Pluck("user_id", &users)
	for _, userID := range users {
		var employee Employee
		var bindings []UserRole
		DB.Where("user_id = ?", userID).Order("created_at, role_id").Find(&bindings)
		keepRoleID := uint(0)
		if DB.Select("role").First(&employee, userID).Error == nil {
			var role Role
			if DB.Where("code = ?", normalizeSeedRoleCode(employee.Role)).First(&role).Error == nil {
				keepRoleID = role.ID
			}
		}
		if keepRoleID == 0 && len(bindings) > 0 {
			keepRoleID = bindings[0].RoleID
		}
		DB.Where("user_id = ? AND role_id <> ?", userID, keepRoleID).Delete(&UserRole{})
	}

	var roleIDs []uint
	DB.Model(&RoleDataScope{}).Select("role_id").Group("role_id").Having("COUNT(*) > 1").Pluck("role_id", &roleIDs)
	for _, roleID := range roleIDs {
		var bindings []RoleDataScope
		DB.Where("role_id = ?", roleID).Order("created_at, data_scope_id").Find(&bindings)
		if len(bindings) > 0 {
			DB.Where("role_id = ? AND data_scope_id <> ?", roleID, bindings[0].DataScopeID).Delete(&RoleDataScope{})
		}
	}

	if err := DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_user_roles_one_role_per_user ON user_roles(user_id)").Error; err != nil {
		log.Printf("创建用户唯一角色约束失败: %v", err)
	}
	if err := DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_role_data_scopes_one_scope_per_role ON role_data_scopes(role_id)").Error; err != nil {
		log.Printf("创建角色唯一数据范围约束失败: %v", err)
	}
}

func normalizeSeedRoleCode(role string) string {
	switch role {
	case "hr":
		return "hr_admin"
	case "manager":
		return "department_manager"
	case "":
		return "employee"
	default:
		return role
	}
}

// BootstrapSuperAdmin provides an explicit, owner-controlled initialization
// and recovery path. It can create a first owner from BOOTSTRAP_ADMIN_* or
// promote an existing account through the legacy BOOTSTRAP_SUPER_ADMIN_EMAIL.
func BootstrapSuperAdmin() error {
	var activeCount int64
	if err := DB.Table("user_roles ur").
		Joins("JOIN roles r ON r.id = ur.role_id").
		Joins("JOIN employees e ON e.id = ur.user_id").
		Where("r.code = ? AND e.is_active = ?", "super_admin", true).
		Count(&activeCount).Error; err != nil {
		return fmt.Errorf("检查超级管理员状态失败: %w", err)
	}
	if activeCount > 0 {
		return nil
	}

	email := strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_EMAIL"))
	if email == "" {
		email = strings.TrimSpace(os.Getenv("BOOTSTRAP_SUPER_ADMIN_EMAIL"))
	}
	if email == "" {
		message := "当前没有有效超级管理员；请设置 BOOTSTRAP_ADMIN_EMAIL、BOOTSTRAP_ADMIN_NAME 和 BOOTSTRAP_ADMIN_PASSWORD_FILE"
		if isProduction() {
			return errors.New(message)
		}
		log.Println("警告：" + message)
		return nil
	}
	parsedAddress, err := mail.ParseAddress(email)
	if err != nil || !strings.EqualFold(parsedAddress.Address, email) {
		return fmt.Errorf("超级管理员初始化邮箱格式无效: %q", email)
	}

	var employee Employee
	lookupErr := DB.Where("LOWER(email) = LOWER(?)", email).First(&employee).Error
	if lookupErr == nil && !employee.IsActive {
		return fmt.Errorf("超级管理员初始化失败：指定账号 %q 已停用", email)
	}
	if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		return fmt.Errorf("查询超级管理员初始化账号失败: %w", lookupErr)
	}

	var role Role
	if err := DB.Where("code = ?", "super_admin").First(&role).Error; err != nil {
		return fmt.Errorf("超级管理员初始化失败：系统角色不存在: %w", err)
	}

	created := false
	if errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		name := strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_NAME"))
		passwordFile := strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_PASSWORD_FILE"))
		if name == "" || passwordFile == "" {
			return errors.New("创建初始超级管理员需要 BOOTSTRAP_ADMIN_NAME 和 BOOTSTRAP_ADMIN_PASSWORD_FILE")
		}
		passwordBytes, err := os.ReadFile(passwordFile)
		if err != nil {
			return fmt.Errorf("读取初始超级管理员密码文件失败: %w", err)
		}
		password := strings.TrimSpace(string(passwordBytes))
		if len(password) < 12 {
			return errors.New("初始超级管理员密码长度至少为 12 位")
		}
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("初始超级管理员密码加密失败: %w", err)
		}

		err = DB.Transaction(func(tx *gorm.DB) error {
			department := Department{Name: "系统管理", Description: "系统初始化管理部门"}
			if err := tx.Where("name = ?", department.Name).FirstOrCreate(&department).Error; err != nil {
				return err
			}
			employee = Employee{
				Name: name, Email: email, Password: string(hashedPassword), Position: "系统管理员",
				DepartmentID: department.ID, Role: "super_admin", IsActive: true,
			}
			if err := tx.Create(&employee).Error; err != nil {
				return err
			}
			if err := tx.Create(&UserRole{UserID: employee.ID, RoleID: role.ID}).Error; err != nil {
				return err
			}
			return tx.Create(&AuditLog{UserID: employee.ID, Action: "bootstrap_super_admin", Resource: "role", ResourceID: fmt.Sprint(role.ID), Result: "SUCCESS", Details: "created=true email=" + employee.Email}).Error
		})
		if err != nil {
			return fmt.Errorf("创建初始超级管理员失败: %w", err)
		}
		created = true
	}

	if !created {
		err := DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("user_id = ?", employee.ID).Delete(&UserRole{}).Error; err != nil {
				return err
			}
			if err := tx.Create(&UserRole{UserID: employee.ID, RoleID: role.ID}).Error; err != nil {
				return err
			}
			if err := tx.Model(&Employee{}).Where("id = ?", employee.ID).Update("role", "super_admin").Error; err != nil {
				return err
			}
			return tx.Create(&AuditLog{UserID: employee.ID, Action: "bootstrap_super_admin", Resource: "role", ResourceID: fmt.Sprint(role.ID), Result: "SUCCESS", Details: "email=" + employee.Email}).Error
		})
		if err != nil {
			return fmt.Errorf("超级管理员初始化失败: %w", err)
		}
	}
	log.Printf("已按显式配置将账号 %q 初始化为超级管理员；初始化完成后请移除 BOOTSTRAP_ADMIN_* 配置和密码文件", employee.Email)
	return nil
}

// 保留包内旧调用名，便于既有测试覆盖“提升已有账号”的兼容路径。
func bootstrapSuperAdmin() {
	if err := BootstrapSuperAdmin(); err != nil {
		log.Printf("超级管理员初始化失败: %v", err)
	}
}

// EnsureSystemDefaults 写入生产可用且幂等的最小业务默认值。
func EnsureSystemDefaults() error {
	setting := SystemSetting{Key: "allow_registration", Value: "false", Type: "boolean"}
	if err := DB.Where("key = ?", setting.Key).FirstOrCreate(&setting).Error; err != nil {
		return fmt.Errorf("初始化系统注册设置失败: %w", err)
	}
	var ruleCount int64
	if err := DB.Model(&PerformanceRule{}).Count(&ruleCount).Error; err != nil {
		return fmt.Errorf("检查默认绩效规则失败: %w", err)
	}
	if ruleCount == 0 {
		defaultRule := DefaultPerformanceRule()
		if err := DB.Create(&defaultRule).Error; err != nil {
			return fmt.Errorf("初始化默认绩效规则失败: %w", err)
		}
	}
	return nil
}
