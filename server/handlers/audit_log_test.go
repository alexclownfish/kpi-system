package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
)

func grantAuditLogAccess(t *testing.T, userID uint, scope string) {
	t.Helper()
	assignScope(t, userID, scope)
	permission := models.Permission{Code: "audit:view", Name: "查看审计日志"}
	if err := models.DB.Create(&permission).Error; err != nil {
		t.Fatal(err)
	}
	var binding models.UserRole
	if err := models.DB.Where("user_id = ?", userID).First(&binding).Error; err != nil {
		t.Fatal(err)
	}
	if err := models.DB.Create(&models.RolePermission{RoleID: binding.RoleID, PermissionID: permission.ID}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestGetAuditLogsFiltersOperationalDiagnosticsAndReturnsActor(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	actor, _, _ := seedConfirmationEvaluation(t, "pending_confirm")
	grantAuditLogAccess(t, actor.ID, "ALL")
	models.DB.Create(&models.AuditLog{UserID: actor.ID, Action: "assessment_migration_dry_run", Resource: "migration_batch", ResourceID: "batch-1", Result: "SUCCESS", Details: "eligible=1"})
	models.DB.Create(&models.AuditLog{UserID: actor.ID, Action: "login", Resource: "auth", ResourceID: "", Result: "SUCCESS"})

	r := gin.New()
	r.GET("/audit-logs", func(c *gin.Context) { c.Set("user_id", actor.ID); GetAuditLogs(c) })
	record := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/audit-logs?action_prefix=assessment_migration_&limit=10", nil)
	r.ServeHTTP(record, request)
	if record.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", record.Code, record.Body.String())
	}
	var envelope struct {
		Data struct {
			Logs []struct {
				Action string `json:"action"`
				Actor  *struct {
					Name string `json:"name"`
				} `json:"actor"`
			} `json:"logs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(record.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data.Logs) != 1 || envelope.Data.Logs[0].Action != "assessment_migration_dry_run" || envelope.Data.Logs[0].Actor == nil || envelope.Data.Logs[0].Actor.Name != actor.Name {
		t.Fatalf("unexpected filtered audit logs: %s", record.Body.String())
	}
}

func TestGetAuditLogsRejectsInvalidCursorAndLimit(t *testing.T) {
	setupConfirmationDB(t)
	gin.SetMode(gin.TestMode)
	actor, _, _ := seedConfirmationEvaluation(t, "pending_confirm")
	grantAuditLogAccess(t, actor.ID, "ALL")
	r := gin.New()
	r.GET("/audit-logs", func(c *gin.Context) { c.Set("user_id", actor.ID); GetAuditLogs(c) })
	for _, query := range []string{"?limit=0", "?limit=201", "?before_id=nope"} {
		record := httptest.NewRecorder()
		r.ServeHTTP(record, httptest.NewRequest(http.MethodGet, "/audit-logs"+query, nil))
		if record.Code != http.StatusBadRequest || !strings.Contains(record.Body.String(), "error") {
			t.Fatalf("query=%s status=%d body=%s", query, record.Code, record.Body.String())
		}
	}
}

func TestGetAuditLogsDeniesPermissionAndScopedAuditor(t *testing.T) {
	for _, scope := range []string{"", "SELF"} {
		t.Run(fmt.Sprintf("scope-%s", scope), func(t *testing.T) {
			setupConfirmationDB(t)
			actor, _, _ := seedConfirmationEvaluation(t, "pending_confirm")
			if scope != "" {
				grantAuditLogAccess(t, actor.ID, scope)
			}
			r := gin.New()
			r.GET("/audit-logs", func(c *gin.Context) { c.Set("user_id", actor.ID); GetAuditLogs(c) })
			record := httptest.NewRecorder()
			r.ServeHTTP(record, httptest.NewRequest(http.MethodGet, "/audit-logs", nil))
			if record.Code != http.StatusForbidden {
				t.Fatalf("status=%d body=%s", record.Code, record.Body.String())
			}
		})
	}
}
