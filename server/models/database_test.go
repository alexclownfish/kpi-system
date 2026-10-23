package models

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
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
	if roleCount != 10 {
		t.Fatalf("role count=%d, want 10", roleCount)
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
	var productSubmitPermissions int64
	DB.Table("role_permissions rp").
		Joins("JOIN roles r ON r.id = rp.role_id").
		Joins("JOIN permissions p ON p.id = rp.permission_id").
		Where("r.code = ? AND p.code = ?", "product_manager", "assessment:submit").
		Count(&productSubmitPermissions)
	if productSubmitPermissions != 1 {
		t.Fatalf("product_manager assessment:submit=%d, want 1", productSubmitPermissions)
	}
}

func TestCustomizedSystemRoleSurvivesSeeding(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprintf("empty=%v", empty), func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:custom-seed-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			DB = db
			if err := DB.AutoMigrate(&Role{}, &Permission{}, &UserRole{}, &RolePermission{}, &DataScope{}, &RoleDataScope{}); err != nil {
				t.Fatal(err)
			}
			seedAccessControl()
			var role Role
			DB.Where("code = ?", "pm").First(&role)
			DB.Where("role_id = ?", role.ID).Delete(&RolePermission{})
			if !empty {
				var p Permission
				DB.Where("code = ?", "assessment:view").First(&p)
				DB.Create(&RolePermission{RoleID: role.ID, PermissionID: p.ID})
			}
			if err := DB.Model(&role).Update("access_customized", true).Error; err != nil {
				t.Fatal(err)
			}
			var self DataScope
			DB.Where("code = ?", "SELF").First(&self)
			DB.Where("role_id = ?", role.ID).Delete(&RoleDataScope{})
			DB.Create(&RoleDataScope{RoleID: role.ID, DataScopeID: self.ID})
			seedAccessControl()
			seedAccessControl()
			var permissions, scopes int64
			DB.Model(&RolePermission{}).Where("role_id = ?", role.ID).Count(&permissions)
			DB.Model(&RoleDataScope{}).Where("role_id = ?", role.ID).Count(&scopes)
			want := int64(1)
			if empty {
				want = 0
			}
			if permissions != want || scopes != 1 {
				t.Fatalf("defaults restored: perms=%d scope=%d", permissions, scopes)
			}
			var binding RoleDataScope
			DB.Where("role_id = ?", role.ID).First(&binding)
			if binding.DataScopeID != self.ID {
				t.Fatal("scope restored")
			}
			var other Role
			DB.Where("code = ?", "product_manager").First(&other)
			DB.Model(&RolePermission{}).Where("role_id = ?", other.ID).Count(&permissions)
			if permissions == 0 {
				t.Fatal("other defaults lost")
			}
		})
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

func TestBootstrapSuperAdminCreatesFreshProductionOwnerFromPasswordFile(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:fresh-bootstrap-test-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	DB = db
	if err := DB.AutoMigrate(&Role{}, &Permission{}, &UserRole{}, &RolePermission{}, &DataScope{}, &RoleDataScope{}, &AuditLog{}, &Department{}, &Employee{}); err != nil {
		t.Fatal(err)
	}
	seedAccessControl()

	password := "A-production-test-password-123!"
	passwordFile := filepath.Join(t.TempDir(), "bootstrap-password")
	if err := os.WriteFile(passwordFile, []byte(password+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_ENV", "production")
	t.Setenv("BOOTSTRAP_ADMIN_EMAIL", "owner@example.com")
	t.Setenv("BOOTSTRAP_ADMIN_NAME", "admin")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD_FILE", passwordFile)
	t.Setenv("BOOTSTRAP_SUPER_ADMIN_EMAIL", "")

	if err := BootstrapSuperAdmin(); err != nil {
		t.Fatalf("fresh bootstrap failed: %v", err)
	}

	var employee Employee
	if err := DB.Where("email = ?", "owner@example.com").First(&employee).Error; err != nil {
		t.Fatal(err)
	}
	if employee.Name != "admin" || employee.Role != "super_admin" || !employee.IsActive {
		t.Fatalf("unexpected owner: %+v", employee)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(employee.Password), []byte(password)); err != nil {
		t.Fatalf("stored password is not the expected bcrypt hash: %v", err)
	}
	var role Role
	if err := DB.Table("roles r").Select("r.*").Joins("JOIN user_roles ur ON ur.role_id = r.id").Where("ur.user_id = ?", employee.ID).First(&role).Error; err != nil {
		t.Fatal(err)
	}
	if role.Code != "super_admin" {
		t.Fatalf("role=%s, want super_admin", role.Code)
	}

	// 删除一次性密码文件后重复启动仍应成功，因为已有有效超级管理员。
	if err := os.Remove(passwordFile); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapSuperAdmin(); err != nil {
		t.Fatalf("idempotent bootstrap failed after credential removal: %v", err)
	}
	var employeeCount int64
	DB.Model(&Employee{}).Where("email = ?", "owner@example.com").Count(&employeeCount)
	if employeeCount != 1 {
		t.Fatalf("owner count=%d, want 1", employeeCount)
	}
}

func TestProductionBootstrapRequiresExplicitOwner(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:required-bootstrap-test-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	DB = db
	if err := DB.AutoMigrate(&Role{}, &Permission{}, &UserRole{}, &RolePermission{}, &DataScope{}, &RoleDataScope{}, &AuditLog{}, &Department{}, &Employee{}); err != nil {
		t.Fatal(err)
	}
	seedAccessControl()
	t.Setenv("APP_ENV", "production")
	t.Setenv("BOOTSTRAP_ADMIN_EMAIL", "")
	t.Setenv("BOOTSTRAP_SUPER_ADMIN_EMAIL", "")

	if err := BootstrapSuperAdmin(); err == nil {
		t.Fatal("production bootstrap unexpectedly allowed a deployment without an owner")
	}
}
