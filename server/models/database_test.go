package models

import (
	"fmt"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSeedAccessControlCreatesMVPCatalog(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:catalog-test-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	DB = db
	if err := DB.AutoMigrate(&Role{}, &Permission{}, &UserRole{}, &RolePermission{}, &DataScope{}, &RoleDataScope{}); err != nil {
		t.Fatal(err)
	}
	seedAccessControl()

	var roleCount int64
	DB.Model(&Role{}).Count(&roleCount)
	if roleCount != 7 {
		t.Fatalf("role count=%d, want 7", roleCount)
	}
	for _, code := range []string{
		"department:assign_manager", "kpi:disable", "assessment:publish",
		"review:accept", "review:reject", "report:company", "report:department",
		"report:personal", "role:view", "role:create", "role:edit", "role:delete",
	} {
		var count int64
		DB.Model(&Permission{}).Where("code = ?", code).Count(&count)
		if count != 1 {
			t.Errorf("permission %s count=%d, want 1", code, count)
		}
	}
}

func TestBootstrapSuperAdminRequiresExplicitEmailAndAuditsPromotion(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:bootstrap-test-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	DB = db
	if err := DB.AutoMigrate(&Role{}, &Permission{}, &UserRole{}, &RolePermission{}, &DataScope{}, &RoleDataScope{}, &AuditLog{}, &Department{}, &Employee{}); err != nil {
		t.Fatal(err)
	}
	seedAccessControl()
	department := Department{Name: "管理部"}
	DB.Create(&department)
	employee := Employee{Name: "系统所有者", Email: "owner@test.local", Password: "hash", DepartmentID: department.ID, Role: "hr", IsActive: true}
	DB.Create(&employee)
	var hr Role
	DB.Where("code = ?", "hr_admin").First(&hr)
	DB.Create(&UserRole{UserID: employee.ID, RoleID: hr.ID})

	t.Setenv("BOOTSTRAP_SUPER_ADMIN_EMAIL", "")
	bootstrapSuperAdmin()
	current, err := func() (Role, error) {
		var role Role
		err := DB.Table("roles r").Select("r.*").Joins("JOIN user_roles ur ON ur.role_id = r.id").Where("ur.user_id = ?", employee.ID).First(&role).Error
		return role, err
	}()
	if err != nil || current.Code != "hr_admin" {
		t.Fatalf("implicit bootstrap changed role: %+v err=%v", current, err)
	}

	t.Setenv("BOOTSTRAP_SUPER_ADMIN_EMAIL", strings.ToUpper(employee.Email))
	bootstrapSuperAdmin()
	current, err = func() (Role, error) {
		var role Role
		err := DB.Table("roles r").Select("r.*").Joins("JOIN user_roles ur ON ur.role_id = r.id").Where("ur.user_id = ?", employee.ID).First(&role).Error
		return role, err
	}()
	if err != nil || current.Code != "super_admin" {
		t.Fatalf("explicit bootstrap role=%+v err=%v", current, err)
	}
	var auditCount int64
	DB.Model(&AuditLog{}).Where("action = ? AND user_id = ? AND result = ?", "bootstrap_super_admin", employee.ID, "SUCCESS").Count(&auditCount)
	if auditCount != 1 {
		t.Fatalf("audit count=%d, want 1", auditCount)
	}
}
