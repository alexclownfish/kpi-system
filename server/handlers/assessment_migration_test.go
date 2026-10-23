package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func migrationFixture(t *testing.T) (*gorm.DB, models.Employee, models.Employee, models.KPIEvaluation, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:migration-apply-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.Role{}, &models.Permission{}, &models.UserRole{}, &models.RolePermission{}, &models.DataScope{}, &models.RoleDataScope{}, &models.AuditLog{},
		&models.Department{}, &models.Employee{}, &models.KPITemplate{}, &models.KPIDimension{}, &models.KPIItem{}, &models.KPIEvaluation{}, &models.KPIScore{},
		&models.EvaluationResponsibility{}, &models.EvaluationScoringTask{}, &models.EvaluationCalculationResult{}, &models.EvaluationInvitation{}, &models.InvitedScore{},
		&models.AssessmentAuditFinding{}, &models.MigrationBatch{}, &models.EvaluationResultSnapshot{}, &models.EvaluationConfirmation{},
	); err != nil {
		t.Fatal(err)
	}
	models.DB = db
	department := models.Department{Name: "迁移测试部"}
	db.Create(&department)
	admin := createWorkflowFixtureEmployee(t, db, "迁移管理员", "migration-admin-"+t.Name()+"@test", "HR", department.ID)
	other := createWorkflowFixtureEmployee(t, db, "普通用户", "migration-other-"+t.Name()+"@test", "员工", department.ID)
	template := models.KPITemplate{Name: "迁移模板", Period: "monthly", Version: 2, CalculationVersion: canonicalCalculationVersion}
	if err := db.Create(&template).Error; err != nil {
		t.Fatal(err)
	}
	item := models.KPIItem{TemplateID: template.ID, Name: "交付", MaxScore: 100, ItemWeight: 100, CompositeWeight: 100, Required: true, ScorerTypesJSON: `["PM"]`}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	template.Items = []models.KPIItem{item}

	// Migration scope is deliberately explicit in the fixture. This mirrors the
	// production role/data-scope join and avoids falling back to legacy role names.
	allScope := models.DataScope{Code: "ALL", Name: "全公司"}
	if err := db.Create(&allScope).Error; err != nil {
		t.Fatal(err)
	}
	adminRole := models.Role{Code: "migration_admin", Name: "迁移管理员"}
	if err := db.Create(&adminRole).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.UserRole{UserID: admin.ID, RoleID: adminRole.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RoleDataScope{RoleID: adminRole.ID, DataScopeID: allScope.ID}).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := buildWorkflowTemplateSnapshot(template, []workflowScoringAssignment{{ItemID: item.ID, ScorerType: "PM", UserID: admin.ID, UserName: admin.Name, SourceRule: "fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	evaluation := createWorkflowFixtureEvaluation(t, db, other.ID, template.ID, workflowStateScoring, snapshot)
	legacyScore := 72.0
	db.Create(&models.KPIScore{EvaluationID: evaluation.ID, ItemID: item.ID, FinalScore: &legacyScore})
	invitation := models.EvaluationInvitation{EvaluationID: evaluation.ID, InviterID: admin.ID, InviteeID: other.ID, Status: "completed"}
	db.Create(&invitation)
	db.Create(&models.InvitedScore{InvitationID: invitation.ID, ItemID: item.ID, Score: &legacyScore})
	return db, admin, other, evaluation, snapshot
}

func createMigrationBatch(t *testing.T, db *gorm.DB, actor uint, evaluationID uint, snapshot string) models.MigrationBatch {
	t.Helper()
	parsed, err := parseEvaluationSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal([]migrationPlanRecord{{EvaluationID: evaluationID, SourceChecksum: parsed.Checksum, Snapshot: parsed}})
	batch := models.MigrationBatch{ID: "batch-" + t.Name(), Mode: "dry_run", Status: "completed", SourceVersion: "legacy-or-weighted-v2", TargetVersion: canonicalCalculationVersion, PayloadJSON: string(payload), FiltersJSON: `{}`, SummaryJSON: `{}`, OperatorID: actor}
	if err := db.Create(&batch).Error; err != nil {
		t.Fatal(err)
	}
	return batch
}

func runApplyMigration(t *testing.T, db *gorm.DB, actor uint, batchID string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/assessment-migrations/:batchId/apply", func(c *gin.Context) { c.Set("user_id", actor); ApplyAssessmentMigration(c) })
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/assessment-migrations/"+batchID+"/apply", nil)
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestApplyMigrationCreatesTasksAndPreservesLegacyRows(t *testing.T) {
	db, admin, _, evaluation, snapshot := migrationFixture(t)
	batch := createMigrationBatch(t, db, admin.ID, evaluation.ID, snapshot)
	response := runApplyMigration(t, db, admin.ID, batch.ID)
	if response.Code != 200 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var taskCount, scoreCount, inviteCount, invitedScoreCount int64
	db.Model(&models.EvaluationScoringTask{}).Where("evaluation_id = ?", evaluation.ID).Count(&taskCount)
	db.Model(&models.KPIScore{}).Where("evaluation_id = ?", evaluation.ID).Count(&scoreCount)
	db.Model(&models.EvaluationInvitation{}).Where("evaluation_id = ?", evaluation.ID).Count(&inviteCount)
	db.Model(&models.InvitedScore{}).Joins("JOIN evaluation_invitations ON evaluation_invitations.id = invited_scores.invitation_id").Where("evaluation_invitations.evaluation_id = ?", evaluation.ID).Count(&invitedScoreCount)
	if taskCount != 1 || scoreCount != 1 || inviteCount != 1 || invitedScoreCount != 1 {
		t.Fatalf("tasks=%d scores=%d invitations=%d invited_scores=%d", taskCount, scoreCount, inviteCount, invitedScoreCount)
	}
	var stored models.MigrationBatch
	db.First(&stored, "id = ?", batch.ID)
	if stored.Status != "applied" || stored.Mode != "apply" {
		t.Fatalf("batch=%+v", stored)
	}
}

func TestApplyMigrationRejectsChecksumConflictAndDoesNotWrite(t *testing.T) {
	db, admin, _, evaluation, snapshot := migrationFixture(t)
	batch := createMigrationBatch(t, db, admin.ID, evaluation.ID, snapshot)
	db.Model(&models.KPIEvaluation{}).Where("id = ?", evaluation.ID).Update("template_snapshot_json", `{"calculation_version":"weighted-v2","checksum":"changed","items":[]}`)
	response := runApplyMigration(t, db, admin.ID, batch.ID)
	if response.Code != 409 || !containsBody(response.Body.String(), "MIGRATION_SOURCE_CHANGED") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var count int64
	db.Model(&models.EvaluationScoringTask{}).Count(&count)
	if count != 0 {
		t.Fatalf("tasks were written after checksum conflict: %d", count)
	}
}

func TestApplyMigrationRejectsAlreadyApplied(t *testing.T) {
	db, admin, _, evaluation, snapshot := migrationFixture(t)
	batch := createMigrationBatch(t, db, admin.ID, evaluation.ID, snapshot)
	if response := runApplyMigration(t, db, admin.ID, batch.ID); response.Code != 200 {
		t.Fatalf("first apply status=%d", response.Code)
	}
	response := runApplyMigration(t, db, admin.ID, batch.ID)
	if response.Code != 409 || !containsBody(response.Body.String(), "MIGRATION_ALREADY_APPLIED") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestApplyMigrationRejectsUnauthorizedScopeAndRollsBack(t *testing.T) {
	db, admin, other, evaluation, snapshot := migrationFixture(t)
	// The target must belong to someone other than the actor. A SELF-scoped
	// employee is allowed to migrate their own assessment, which would make
	// this test accidentally exercise an authorized path.
	if err := db.Model(&models.KPIEvaluation{}).Where("id = ?", evaluation.ID).Update("employee_id", admin.ID).Error; err != nil {
		t.Fatal(err)
	}
	// Keep the employee in a separate data scope so ApplyEvaluationScope cannot
	// accidentally treat the user as an administrator in a minimal fixture.
	selfScope := models.DataScope{Code: "SELF", Name: "本人"}
	db.Create(&selfScope)
	role := models.Role{Code: "migration_employee", Name: "迁移普通用户"}
	db.Create(&role)
	db.Create(&models.UserRole{UserID: other.ID, RoleID: role.ID})
	db.Create(&models.RoleDataScope{RoleID: role.ID, DataScopeID: selfScope.ID})
	batch := createMigrationBatch(t, db, admin.ID, evaluation.ID, snapshot)
	response := runApplyMigration(t, db, other.ID, batch.ID)
	if response.Code != 409 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var taskCount int64
	db.Model(&models.EvaluationScoringTask{}).Count(&taskCount)
	if taskCount != 0 {
		t.Fatalf("unauthorized apply wrote tasks: %d", taskCount)
	}
}

func containsBody(body, text string) bool { return strings.Contains(body, text) }
