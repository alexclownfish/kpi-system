package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDeleteEvaluationCleansWorkflowChildrenAndPaperAttachment(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:kpi-delete-evaluation?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.Department{}, &models.Employee{}, &models.KPITemplate{}, &models.KPIDimension{}, &models.KPIItem{},
		&models.KPIEvaluation{}, &models.KPIScore{}, &models.EvaluationScoringTask{}, &models.EvaluationCalculationResult{},
		&models.EvaluationResultSnapshot{}, &models.EvaluationConfirmation{}, &models.EvaluationComment{},
		&models.EvaluationInvitation{}, &models.InvitedScore{},
		&models.EvaluationSignoffSupplement{},
	); err != nil {
		t.Fatal(err)
	}
	models.DB = db
	department := models.Department{Name: "删除测试部门"}
	if err := db.Create(&department).Error; err != nil {
		t.Fatal(err)
	}
	employee := models.Employee{Name: "删除测试员工", Email: "delete-evaluation@test", DepartmentID: department.ID, IsActive: true}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatal(err)
	}
	outsider := models.Employee{Name: "删除范围外员工", Email: "delete-outsider@test", DepartmentID: department.ID, IsActive: true}
	if err := db.Create(&outsider).Error; err != nil {
		t.Fatal(err)
	}
	template := models.KPITemplate{Name: "删除测试模板", Period: "monthly"}
	if err := db.Create(&template).Error; err != nil {
		t.Fatal(err)
	}
	dimension := models.KPIDimension{TemplateID: template.ID, Name: "维度"}
	item := models.KPIItem{TemplateID: template.ID, DimensionID: &dimension.ID, Name: "指标", MaxScore: 100}
	if err := db.Create(&dimension).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	evaluation := models.KPIEvaluation{EmployeeID: employee.ID, TemplateID: template.ID, Period: "monthly", Year: 2026, Month: ptrIntForDeleteTest(9), Status: workflowStateCompleted, TemplateSnapshotJSON: `{}`}
	if err := db.Create(&evaluation).Error; err != nil {
		t.Fatal(err)
	}
	score := 80.0
	db.Create(&models.KPIScore{EvaluationID: evaluation.ID, ItemID: item.ID, FinalScore: &score})
	db.Create(&models.EvaluationScoringTask{EvaluationID: evaluation.ID, ItemID: item.ID, ScorerUserID: employee.ID, ScorerType: "PM"})
	db.Create(&models.EvaluationCalculationResult{EvaluationID: evaluation.ID, Version: 1, ResultJSON: `{}`, TotalScore: 80})
	snapshot := models.EvaluationResultSnapshot{EvaluationID: evaluation.ID, Version: 1, SnapshotJSON: `{}`, Checksum: "delete-checksum", CreatedBy: employee.ID}
	db.Create(&snapshot)
	db.Create(&models.EvaluationComment{EvaluationID: evaluation.ID, UserID: employee.ID, Content: "comment"})
	invitation := models.EvaluationInvitation{EvaluationID: evaluation.ID, InviterID: employee.ID, InviteeID: employee.ID}
	db.Create(&invitation)
	db.Create(&models.InvitedScore{InvitationID: invitation.ID, ItemID: item.ID, Score: &score})
	uploadDir := t.TempDir()
	t.Setenv("SIGNOFF_UPLOAD_DIR", uploadDir)
	attachment := filepath.Join(uploadDir, "delete-signoff.pdf")
	if err := os.WriteFile(attachment, []byte("signed"), 0600); err != nil {
		t.Fatal(err)
	}
	db.Create(&models.EvaluationConfirmation{EvaluationID: evaluation.ID, SnapshotID: snapshot.ID, Method: "paper", ConfirmedBy: employee.ID, AttachmentStoredName: "delete-signoff.pdf"})
	supplementAttachment := filepath.Join(uploadDir, "delete-supplement.png")
	if err := os.WriteFile(supplementAttachment, []byte("supplement"), 0600); err != nil {
		t.Fatal(err)
	}
	db.Create(&models.EvaluationSignoffSupplement{EvaluationID: evaluation.ID, UploadedBy: employee.ID, UploadedAt: time.Now(), AttachmentStoredName: "delete-supplement.png", AttachmentOriginalName: "supplement.png"})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/evaluations/:id", func(c *gin.Context) {
		c.Set("user_id", outsider.ID)
		DeleteEvaluation(c)
	})
	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest("DELETE", "/evaluations/"+uintStringForDeleteTest(evaluation.ID), nil))
	if unauthorized.Code != 403 {
		t.Fatalf("out-of-scope delete status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}
	router = gin.New()
	router.DELETE("/evaluations/:id", func(c *gin.Context) { c.Set("user_id", employee.ID); DeleteEvaluation(c) })
	record := httptest.NewRecorder()
	request := httptest.NewRequest("DELETE", "/evaluations/"+uintStringForDeleteTest(evaluation.ID), nil)
	router.ServeHTTP(record, request)
	if record.Code != 200 {
		t.Fatalf("status=%d body=%s", record.Code, record.Body.String())
	}
	for _, model := range []any{
		&models.KPIEvaluation{}, &models.KPIScore{}, &models.EvaluationScoringTask{}, &models.EvaluationCalculationResult{},
		&models.EvaluationResultSnapshot{}, &models.EvaluationConfirmation{}, &models.EvaluationComment{},
		&models.EvaluationInvitation{}, &models.InvitedScore{}, &models.EvaluationSignoffSupplement{},
	} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("orphan rows remain in %T: %d", model, count)
		}
	}
	if _, err := os.Stat(attachment); !os.IsNotExist(err) {
		t.Fatalf("paper attachment still exists: %v", err)
	}
	if _, err := os.Stat(supplementAttachment); !os.IsNotExist(err) {
		t.Fatalf("supplement attachment still exists: %v", err)
	}
}

func TestDeleteEvaluationRestoresSupplementWhenChildDeleteFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:kpi-delete-rollback?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.Department{}, &models.Employee{}, &models.KPITemplate{}, &models.KPIDimension{}, &models.KPIItem{},
		&models.KPIEvaluation{}, &models.KPIScore{}, &models.EvaluationScoringTask{}, &models.EvaluationCalculationResult{},
		&models.EvaluationResultSnapshot{}, &models.EvaluationConfirmation{}, &models.EvaluationComment{},
		&models.EvaluationInvitation{}, &models.InvitedScore{}, &models.EvaluationSignoffSupplement{},
	); err != nil {
		t.Fatal(err)
	}
	models.DB = db

	department := models.Department{Name: "删除回滚部门"}
	if err := db.Create(&department).Error; err != nil {
		t.Fatal(err)
	}
	employee := models.Employee{Name: "删除回滚员工", Email: "delete-rollback@test", DepartmentID: department.ID, IsActive: true}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatal(err)
	}
	template := models.KPITemplate{Name: "删除回滚模板", Period: "monthly"}
	if err := db.Create(&template).Error; err != nil {
		t.Fatal(err)
	}
	dimension := models.KPIDimension{TemplateID: template.ID, Name: "维度"}
	if err := db.Create(&dimension).Error; err != nil {
		t.Fatal(err)
	}
	item := models.KPIItem{TemplateID: template.ID, DimensionID: &dimension.ID, Name: "指标", MaxScore: 100}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	evaluation := models.KPIEvaluation{EmployeeID: employee.ID, TemplateID: template.ID, Period: "monthly", Year: 2026, Status: workflowStateCompleted, TemplateSnapshotJSON: `{}`}
	if err := db.Create(&evaluation).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := models.EvaluationResultSnapshot{EvaluationID: evaluation.ID, Version: 1, SnapshotJSON: `{}`, Checksum: "rollback-checksum", CreatedBy: employee.ID}
	if err := db.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	confirmationAttachmentDir := t.TempDir()
	t.Setenv("SIGNOFF_UPLOAD_DIR", confirmationAttachmentDir)
	confirmationAttachment := filepath.Join(confirmationAttachmentDir, "rollback-confirmation.pdf")
	supplementAttachment := filepath.Join(confirmationAttachmentDir, "rollback-supplement.png")
	if err := os.WriteFile(confirmationAttachment, []byte("confirmation"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(supplementAttachment, []byte("supplement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.EvaluationConfirmation{
		EvaluationID: evaluation.ID, SnapshotID: snapshot.ID, Method: "paper", ConfirmedBy: employee.ID,
		AttachmentStoredName: "rollback-confirmation.pdf", AttachmentOriginalName: "confirmation.pdf",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.EvaluationSignoffSupplement{
		EvaluationID: evaluation.ID, UploadedBy: employee.ID, UploadedAt: time.Now(),
		AttachmentStoredName: "rollback-supplement.png", AttachmentOriginalName: "supplement.png",
	}).Error; err != nil {
		t.Fatal(err)
	}

	// Force a failure after the handler has staged attachments. The child-delete
	// loop reaches comments after confirmation and supplement reads, so dropping
	// that table exercises the transaction/file restoration path.
	if err := db.Migrator().DropTable(&models.EvaluationComment{}); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/evaluations/:id", func(c *gin.Context) {
		c.Set("user_id", employee.ID)
		DeleteEvaluation(c)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/evaluations/"+uintStringForDeleteTest(evaluation.ID), nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s, want child-delete failure", recorder.Code, recorder.Body.String())
	}

	var retained models.KPIEvaluation
	if err := db.First(&retained, evaluation.ID).Error; err != nil {
		t.Fatalf("evaluation should be restored after failed delete: %v", err)
	}
	var supplement models.EvaluationSignoffSupplement
	if err := db.Where("evaluation_id = ?", evaluation.ID).First(&supplement).Error; err != nil {
		t.Fatalf("supplement row should be restored: %v", err)
	}
	for _, path := range []string{confirmationAttachment, supplementAttachment} {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("attachment %s should be restored: %v", path, readErr)
		}
		if len(content) == 0 {
			t.Fatalf("restored attachment %s is empty", path)
		}
	}
	entries, err := os.ReadDir(confirmationAttachmentDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".deleting-") {
			t.Fatalf("staging directory remained after rollback: %s", entry.Name())
		}
	}
}

func ptrIntForDeleteTest(value int) *int { return &value }

func uintStringForDeleteTest(value uint) string {
	if value == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for value > 0 {
		buf = append([]byte{byte('0' + value%10)}, buf...)
		value /= 10
	}
	return string(buf)
}
