package models

import (
	"errors"
	"fmt"
	"log"
	"net/mail"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// 初始化数据库连接
func InitDB() {
	var err error

	// 创建db目录
	os.MkdirAll("db", 0755)

	// 连接SQLite数据库
	logLevel := logger.Info
	if isProduction() {
		logLevel = logger.Warn
	}
	DB, err = gorm.Open(sqlite.Open("db/kpi.db"), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		log.Fatal("数据库连接失败:", err)
	}

	log.Println("数据库连接成功")

	// 自动迁移数据库表
	err = DB.AutoMigrate(
		&Role{},
		&Permission{},
		&UserRole{},
		&RolePermission{},
		&DataScope{},
		&RoleDataScope{},
		&AuditLog{},
		&Department{},
		&Employee{},
		&KPITemplate{},
		&KPIItem{},
		&KPIEvaluation{},
		&KPIScore{},
		&EvaluationResultSnapshot{},
		&EvaluationConfirmation{},
		&FinalScoreImportBatch{},
		&EvaluationComment{},
		&EvaluationInvitation{},
		&InvitedScore{},
		&SystemSetting{},
		&PerformanceRule{},
	)
	if err != nil {
		log.Fatal("数据库迁移失败:", err)
	}
	seedAccessControl()
	repairAccessControlConstraints()

	log.Println("数据库表迁移完成")
}

func isProduction() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production")
}

// seedAccessControl creates the stable MVP catalog idempotently. Business data
// can be migrated from Employee.Role without changing existing records.
func seedAccessControl() {
	roles := []Role{
		{Code: "super_admin", Name: "超级管理员", IsSystem: true}, {Code: "hr_admin", Name: "HR管理员", IsSystem: true},
		{Code: "performance_admin", Name: "绩效专员", IsSystem: true}, {Code: "department_manager", Name: "部门负责人", IsSystem: true},
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
		{Code: "role:view", Name: "查看角色", Resource: "role", Action: "view"}, {Code: "role:create", Name: "创建角色", Resource: "role", Action: "create"}, {Code: "role:edit", Name: "编辑角色", Resource: "role", Action: "edit"}, {Code: "role:delete", Name: "删除角色", Resource: "role", Action: "delete"}, {Code: "system:view", Name: "查看系统设置", Resource: "system", Action: "view"}, {Code: "system:edit", Name: "修改系统设置", Resource: "system", Action: "edit"}, {Code: "audit:view", Name: "查看审计日志", Resource: "audit", Action: "view"},
	}
	for _, role := range roles {
		DB.Where("code = ?", role.Code).FirstOrCreate(&role, Role{Code: role.Code})
	}
	DB.Model(&Role{}).Where("code IN ?", []string{"super_admin", "hr_admin", "performance_admin", "department_manager", "reviewer", "analyst", "employee"}).Update("is_system", true)
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
		"super_admin":        {"user:view", "user:create", "user:edit", "user:disable", "employee:view", "employee:create", "employee:edit", "employee:delete", "employee:assign_department", "employee:assign_role", "department:view", "department:create", "department:edit", "department:delete", "department:assign_manager", "kpi:view", "kpi:create", "kpi:edit", "kpi:delete", "kpi:publish", "kpi:disable", "assessment:view", "assessment:create", "assessment:edit", "assessment:delete", "assessment:publish", "assessment:submit", "assessment:review", "assessment:approve", "review:view", "review:create", "review:accept", "review:reject", "review:submit", "review:manage", "report:view", "report:export", "report:company", "report:department", "report:personal", "role:view", "role:create", "role:edit", "role:delete", "system:view", "system:edit", "audit:view"},
		"hr_admin":           {"user:view", "user:create", "user:edit", "user:disable", "employee:view", "employee:create", "employee:edit", "employee:delete", "employee:assign_department", "employee:assign_role", "department:view", "department:create", "department:edit", "department:delete", "department:assign_manager", "kpi:view", "kpi:create", "kpi:edit", "kpi:delete", "kpi:publish", "kpi:disable", "assessment:view", "assessment:create", "assessment:edit", "assessment:delete", "assessment:publish", "assessment:review", "assessment:approve", "review:view", "review:create", "review:accept", "review:reject", "review:submit", "review:manage", "report:view", "report:export", "report:company", "report:department", "report:personal"},
		"performance_admin":  {"department:view", "kpi:view", "kpi:create", "kpi:edit", "kpi:delete", "kpi:publish", "kpi:disable", "assessment:view", "assessment:create", "assessment:edit", "assessment:delete", "assessment:publish", "assessment:review", "assessment:approve", "review:view", "review:create", "review:accept", "review:reject", "review:submit", "review:manage", "report:view", "report:export", "report:company", "report:department"},
		"department_manager": {"employee:view", "department:view", "kpi:view", "assessment:view", "assessment:create", "assessment:submit", "assessment:review", "review:view", "review:create", "review:accept", "review:reject", "review:submit", "report:view", "report:export", "report:department"},
		"reviewer":           {"review:view", "review:accept", "review:reject", "review:submit"}, "analyst": {"employee:view", "department:view", "kpi:view", "assessment:view", "report:view", "report:export", "report:company", "report:department", "report:personal"}, "employee": {"employee:view", "kpi:view", "assessment:view", "assessment:submit", "review:view", "review:create", "review:accept", "review:reject", "review:submit", "report:view", "report:personal"},
	}
	for roleCode, codes := range rolePermissions {
		var role Role
		if DB.Where("code = ?", roleCode).First(&role).Error != nil {
			continue
		}
		for _, code := range codes {
			var p Permission
			if DB.Where("code = ?", code).First(&p).Error == nil {
				DB.Where("role_id = ? AND permission_id = ?", role.ID, p.ID).FirstOrCreate(&RolePermission{RoleID: role.ID, PermissionID: p.ID})
			}
		}
	}
	roleScopes := map[string]string{"super_admin": "ALL", "hr_admin": "ALL", "performance_admin": "ALL", "department_manager": "DEPARTMENT", "reviewer": "ASSIGNED", "analyst": "ALL", "employee": "SELF"}
	for roleCode, scopeCode := range roleScopes {
		var role Role
		var scope DataScope
		if DB.Where("code = ?", roleCode).First(&role).Error == nil && DB.Where("code = ?", scopeCode).First(&scope).Error == nil {
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

// 创建测试数据
func CreateTestData() {
	// 检查是否已有数据
	var count int64
	var count2 int64
	var ruleCount int64
	DB.Model(&Department{}).Count(&count)
	DB.Model(&KPITemplate{}).Count(&count2)
	DB.Model(&PerformanceRule{}).Count(&ruleCount)
	if count > 0 && count2 > 0 && ruleCount > 0 {
		log.Println("测试数据已存在，跳过创建")
		return
	}

	// 根据系统模式创建测试数据
	if os.Getenv("SYSTEM_MODE") == "integrated" {
		log.Println("集成模式，仅创建KPI模板")
	} else {
		log.Println("独立模式，创建测试数据")
		CreateTestDataForUser()
	}
	CreateTestDataForTemplate()
	CreateTestDataForPerformanceRule()

	log.Println("测试数据创建完成")
}

// 创建测试数据（用户）
func CreateTestDataForUser() {
	// 创建部门
	departments := []Department{
		{Name: "技术部", Description: "负责产品研发和技术支持"},
		{Name: "市场部", Description: "负责市场营销和客户关系"},
		{Name: "人事部", Description: "负责人力资源管理"},
		{Name: "财务部", Description: "负责财务管理和会计"},
	}

	for _, dept := range departments {
		DB.Create(&dept)
	}

	// 生成默认密码哈希
	defaultPassword := "123456"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal("密码哈希生成失败:", err)
	}

	// 创建员工
	employees := []Employee{
		{Name: "张三", Email: "zhangsan@company.com", Password: string(hashedPassword), Position: "技术总监", DepartmentID: 1, Role: "manager"},
		{Name: "李四", Email: "lisi@company.com", Password: string(hashedPassword), Position: "高级开发工程师", DepartmentID: 1, ManagerID: getUintPtr(1), Role: "employee"},
		{Name: "王五", Email: "wangwu@company.com", Password: string(hashedPassword), Position: "前端开发工程师", DepartmentID: 1, ManagerID: getUintPtr(1), Role: "employee"},
		{Name: "赵六", Email: "zhaoliu@company.com", Password: string(hashedPassword), Position: "市场总监", DepartmentID: 2, Role: "manager"},
		{Name: "钱七", Email: "qianqi@company.com", Password: string(hashedPassword), Position: "市场专员", DepartmentID: 2, ManagerID: getUintPtr(4), Role: "employee"},
		{Name: "孙八", Email: "sunba@company.com", Password: string(hashedPassword), Position: "HR经理", DepartmentID: 3, Role: "hr"},
		{Name: "周九", Email: "zhoujiu@company.com", Password: string(hashedPassword), Position: "财务经理", DepartmentID: 4, Role: "manager"},
	}

	for _, emp := range employees {
		DB.Create(&emp)
	}
}

// 创建测试数据（KPI模板）
func CreateTestDataForTemplate() {
	// 创建KPI模板
	templates := []KPITemplate{
		{Name: "技术岗位月度考核", Description: "适用于技术人员的月度绩效考核", Period: "monthly"},
		{Name: "市场岗位季度考核", Description: "适用于市场人员的季度绩效考核", Period: "quarterly"},
		{Name: "管理岗位年度考核", Description: "适用于管理人员的年度绩效考核", Period: "yearly"},
	}

	for _, template := range templates {
		DB.Create(&template)
	}

	// 创建KPI考核项目
	items := []KPIItem{
		// 技术岗位月度考核项目
		{TemplateID: 1, Name: "代码质量", Description: "代码规范性、可维护性评估", MaxScore: 20, Order: 1},
		{TemplateID: 1, Name: "任务完成度", Description: "按时完成分配的开发任务", MaxScore: 25, Order: 2},
		{TemplateID: 1, Name: "技术创新", Description: "技术方案创新和改进", MaxScore: 15, Order: 3},
		{TemplateID: 1, Name: "团队协作", Description: "与团队成员的协作配合", MaxScore: 20, Order: 4},
		{TemplateID: 1, Name: "学习成长", Description: "技术学习和个人提升", MaxScore: 20, Order: 5},

		// 市场岗位季度考核项目
		{TemplateID: 2, Name: "销售业绩", Description: "季度销售目标达成情况", MaxScore: 40, Order: 1},
		{TemplateID: 2, Name: "客户维护", Description: "客户关系维护和满意度", MaxScore: 30, Order: 2},
		{TemplateID: 2, Name: "市场活动", Description: "市场推广活动执行效果", MaxScore: 30, Order: 3},

		// 管理岗位年度考核项目
		{TemplateID: 3, Name: "团队管理", Description: "团队建设和人员管理", MaxScore: 30, Order: 1},
		{TemplateID: 3, Name: "业务发展", Description: "部门业务发展和目标达成", MaxScore: 35, Order: 2},
		{TemplateID: 3, Name: "战略规划", Description: "部门战略规划和执行", MaxScore: 35, Order: 3},
	}

	for _, item := range items {
		DB.Create(&item)
	}

	// 演示模式显式开启自助注册，生产默认值由 EnsureSystemDefaults 保持关闭。
	setting := SystemSetting{Key: "allow_registration", Value: "true", Type: "boolean"}
	DB.Where("key = ?", setting.Key).Assign(SystemSetting{Value: "true", Type: "boolean"}).FirstOrCreate(&setting)
}

// 创建测试数据（绩效规则）
func CreateTestDataForPerformanceRule() {
	var count int64
	DB.Model(&PerformanceRule{}).Count(&count)
	if count > 0 {
		return
	}

	defaultRule := DefaultPerformanceRule()
	DB.Create(&defaultRule)
}

// 辅助函数：获取uint指针
func getUintPtr(val uint) *uint {
	return &val
}
