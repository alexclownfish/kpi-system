package models

import (
	"errors"
	"fmt"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func templateActivationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:template-activation-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&KPITemplate{}, &KPIEvaluation{}, &AuditLog{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestTemplateActivationDefaultsInactiveOnCreate(t *testing.T) {
	db := templateActivationDB(t)
	for _, status := range []string{"", "draft", "retired"} {
		template := KPITemplate{Name: "未启用模板", Period: "monthly", Status: status, IsActive: false}
		if err := db.Create(&template).Error; err != nil {
			t.Fatal(err)
		}
		var stored KPITemplate
		if err := db.First(&stored, template.ID).Error; err != nil {
			t.Fatal(err)
		}
		if template.IsActive || stored.IsActive {
			t.Fatalf("GORM replaced false for status=%q: inserted=%+v stored=%+v", status, template, stored)
		}
	}
}

func TestTemplateActivationRepairIsNarrowAuditedAndIdempotent(t *testing.T) {
	db := templateActivationDB(t)
	templates := []KPITemplate{
		{Name: "受影响草稿", Status: "draft", IsActive: true},
		{Name: "受影响停用模板", Status: "retired", IsActive: true},
		{Name: "已发布模板", Status: "published", IsActive: true},
		{Name: "已发布但停用模板", Status: "published", IsActive: false},
		{Name: "正常草稿", Status: "draft", IsActive: false},
		{Name: "未知历史状态", Status: "legacy", IsActive: true},
	}
	for index := range templates {
		if err := db.Create(&templates[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	evaluation := KPIEvaluation{TemplateID: templates[0].ID, Status: "completed", TotalScore: 88, TemplateSnapshotJSON: `{"template_name":"历史快照","is_active":true}`}
	if err := db.Create(&evaluation).Error; err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := repairTemplateActivationState(db); err != nil {
			t.Fatal(err)
		}
	}
	for index, original := range templates {
		var stored KPITemplate
		if err := db.First(&stored, original.ID).Error; err != nil {
			t.Fatal(err)
		}
		wantActive := original.IsActive
		if index < 2 {
			wantActive = false
		}
		if stored.Status != original.Status || stored.Name != original.Name || stored.IsActive != wantActive {
			t.Fatalf("repair changed unrelated fields or failed status repair: %+v", stored)
		}
		var audits int64
		if err := db.Model(&AuditLog{}).Where("action = ? AND resource = ? AND resource_id = ? AND result = ?", "template_activation_repaired", "kpi_template", fmt.Sprint(original.ID), "SUCCESS").Count(&audits).Error; err != nil {
			t.Fatal(err)
		}
		wantAudits := int64(0)
		if index < 2 {
			wantAudits = 1
		}
		if audits != wantAudits {
			t.Fatalf("template=%d audit count=%d want=%d", original.ID, audits, wantAudits)
		}
	}
	var storedEvaluation KPIEvaluation
	if err := db.First(&storedEvaluation, evaluation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedEvaluation.Status != evaluation.Status || storedEvaluation.TotalScore != evaluation.TotalScore || storedEvaluation.TemplateSnapshotJSON != evaluation.TemplateSnapshotJSON {
		t.Fatalf("repair changed historical evaluation: %+v", storedEvaluation)
	}
}

func TestTemplateActivationMigrationPreservesExistingRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:template-activation-upgrade-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	legacySchema := struct {
		ID       uint   `gorm:"primaryKey"`
		Name     string `gorm:"not null"`
		Status   string `gorm:"size:16;default:draft"`
		IsActive bool   `gorm:"default:true"`
	}{}
	if err := db.Table("kpi_templates").AutoMigrate(&legacySchema); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO kpi_templates (id, name, status) VALUES (1, '旧草稿', 'draft'), (2, '已发布模板', 'published')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&KPITemplate{}, &AuditLog{}); err != nil {
		t.Fatal(err)
	}
	if err := repairTemplateActivationState(db); err != nil {
		t.Fatal(err)
	}
	var templates []KPITemplate
	if err := db.Order("id").Find(&templates).Error; err != nil {
		t.Fatal(err)
	}
	if len(templates) != 2 || templates[0].Name != "旧草稿" || templates[0].Status != "draft" || templates[0].IsActive || templates[1].Name != "已发布模板" || templates[1].Status != "published" || !templates[1].IsActive {
		t.Fatalf("migration lost existing template state: %+v", templates)
	}
	template := KPITemplate{Name: "迁移后新草稿", Status: "draft"}
	if err := db.Create(&template).Error; err != nil {
		t.Fatal(err)
	}
	if template.IsActive {
		t.Fatal("migration retained the former active default")
	}
}

func TestTemplateActivationRepairRollsBackWhenAuditFails(t *testing.T) {
	db := templateActivationDB(t)
	template := KPITemplate{Name: "受影响草稿", Status: "draft", IsActive: true}
	if err := db.Create(&template).Error; err != nil {
		t.Fatal(err)
	}
	auditErr := errors.New("audit write unavailable")
	if err := db.Callback().Create().Before("gorm:create").Register("fail_template_repair_audit", func(tx *gorm.DB) {
		if tx.Statement.Table == "audit_logs" {
			tx.AddError(auditErr)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := repairTemplateActivationState(db); !errors.Is(err, auditErr) {
		t.Fatalf("repair error=%v, want audit failure", err)
	}
	var stored KPITemplate
	if err := db.First(&stored, template.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.IsActive || stored.Status != "draft" {
		t.Fatalf("failed repair did not roll back: %+v", stored)
	}
}
