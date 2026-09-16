package models

import (
	"fmt"
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
