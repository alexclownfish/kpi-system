package handlers

import (
	"encoding/json"
	"testing"

	"dootask-kpi-server/models"
)

func TestHongxingCatalogRepairsOnlyLegacyLocalFalseBlocksWithoutPublishing(t *testing.T) {
	db := catalogTestDB(t)
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	var catalog hongxingCatalog
	if err := json.Unmarshal(hongxingCatalogJSON, &catalog); err != nil {
		t.Fatal(err)
	}
	var templates []models.KPITemplate
	db.Where("source_issues_json <> ?", "[]").Order("id").Find(&templates)
	if len(templates) != 5 {
		t.Fatalf("source warning templates=%d", len(templates))
	}
	local, unknown, invalid := templates[0], templates[1], templates[2]
	for _, template := range []models.KPITemplate{local, unknown, invalid} {
		if err := db.Model(&models.KPITemplate{}).Where("id = ?", template.ID).Updates(map[string]any{"source_blocked": true, "status": "draft", "is_active": false}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&models.KPITemplate{}).Where("id = ?", unknown.ID).Update("source_issues_json", `[{"code":"unknown_source_failure","cells":["G12"]}]`).Error; err != nil {
		t.Fatal(err)
	}
	var invalidItem models.KPIItem
	db.Where("template_id = ?", invalid.ID).Order("id").First(&invalidItem)
	if err := db.Model(&invalidItem).Update("item_weight", invalidItem.ItemWeight-1).Error; err != nil {
		t.Fatal(err)
	}
	var beforeItems []models.KPIItem
	db.Where("template_id = ?", local.ID).Order("id").Find(&beforeItems)
	var auditBefore int64
	db.Model(&models.AuditLog{}).Count(&auditBefore)
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	var fixed models.KPITemplate
	db.First(&fixed, local.ID)
	if fixed.SourceBlocked || fixed.Status != "draft" || fixed.IsActive || fixed.SourceIssuesJSON == "[]" {
		t.Fatalf("legacy repair changed draft status/provenance: %+v", fixed)
	}
	var issues []map[string]any
	if err := json.Unmarshal([]byte(fixed.SourceIssuesJSON), &issues); err != nil {
		t.Fatal(err)
	}
	for _, issue := range issues {
		if issue["severity"] != "warning" || issue["cells"] == nil || issue["weight_sum"] == nil {
			t.Fatalf("legacy diagnostic provenance lost: %+v", issue)
		}
	}
	var afterItems []models.KPIItem
	db.Where("template_id = ?", local.ID).Order("id").Find(&afterItems)
	for index, item := range beforeItems {
		if afterItems[index].ItemWeight != item.ItemWeight || afterItems[index].CompositeWeight != item.CompositeWeight || afterItems[index].Name != item.Name {
			t.Fatal("legacy warning repair rewrote source weights")
		}
	}
	for _, id := range []uint{unknown.ID, invalid.ID} {
		var untouched models.KPITemplate
		db.First(&untouched, id)
		if !untouched.SourceBlocked || untouched.Status != "draft" || untouched.IsActive {
			t.Fatalf("unrelated/invalid blocker acknowledged: %+v", untouched)
		}
	}
	var auditAfter int64
	db.Model(&models.AuditLog{}).Count(&auditAfter)
	if auditAfter != auditBefore+1 {
		t.Fatalf("repair audit count=%d before=%d", auditAfter, auditBefore)
	}
	if err := SeedHongxingCatalog(db); err != nil {
		t.Fatal(err)
	}
	var auditRepeated int64
	db.Model(&models.AuditLog{}).Count(&auditRepeated)
	if auditRepeated != auditAfter {
		t.Fatal("legacy repair is not idempotent")
	}
}
