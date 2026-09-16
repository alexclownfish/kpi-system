package models

import (
	"log"
	"os"

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
	DB, err = gorm.Open(sqlite.Open("db/kpi.db"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
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

	log.Println("数据库表迁移完成")
}

// seedAccessControl creates the stable MVP catalog idempotently. Business data
// can be migrated from Employee.Role without changing existing records.
func seedAccessControl() {
	roles := []Role{
		{Code: "super_admin", Name: "超级管理员"}, {Code: "hr_admin", Name: "HR管理员"},
		{Code: "performance_admin", Name: "绩效专员"}, {Code: "department_manager", Name: "部门负责人"},
		{Code: "reviewer", Name: "评审人"}, {Code: "analyst", Name: "数据分析员"}, {Code: "employee", Name: "普通员工"},
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

	// 创建默认系统设置
	settings := []SystemSetting{
		{Key: "allow_registration", Value: "true", Type: "boolean"},
	}

	for _, setting := range settings {
		DB.Create(&setting)
	}
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
