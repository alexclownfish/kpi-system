package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"dootask-kpi-server/models"
	"github.com/gin-gonic/gin"
)

type auditLogView struct {
	ID         uint   `json:"id"`
	Action     string `json:"action"`
	Resource   string `json:"resource"`
	ResourceID string `json:"resource_id"`
	Result     string `json:"result"`
	Details    string `json:"details"`
	IPAddress  string `json:"ip_address"`
	CreatedAt  string `json:"created_at"`
	Actor      *struct {
		ID    uint   `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"actor,omitempty"`
}

// GetAuditLogs exposes only operational diagnostics. Authorization is applied
// by the route middleware; the handler keeps filters bounded and read-only.
func GetAuditLogs(c *gin.Context) {
	userID := c.GetUint("user_id")
	if !UserHasPermission(userID, "audit:view") && !UserHasPermission(userID, "system:edit") {
		respondError(c, http.StatusForbidden, "AUDIT_LOG_FORBIDDEN", "无权查看审计诊断", nil)
		return
	}
	if DataScopeForUser(userID) != "ALL" {
		respondError(c, http.StatusForbidden, "AUDIT_LOG_SCOPE_FORBIDDEN", "只有全公司数据范围的管理员可以查看审计诊断", nil)
		return
	}
	limit := 50
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			respondError(c, http.StatusBadRequest, "INVALID_AUDIT_LOG_LIMIT", "limit 必须在1到200之间", nil)
			return
		}
		limit = parsed
	}

	query := models.DB.Model(&models.AuditLog{}).Order("id DESC").Limit(limit)
	if prefix := strings.TrimSpace(c.Query("action_prefix")); prefix != "" {
		query = query.Where("action LIKE ?", prefix+"%")
	}
	if resource := strings.TrimSpace(c.Query("resource")); resource != "" {
		query = query.Where("resource = ?", resource)
	}
	if result := strings.TrimSpace(c.Query("result")); result != "" {
		query = query.Where("result = ?", result)
	}
	if before := strings.TrimSpace(c.Query("before_id")); before != "" {
		id, err := strconv.ParseUint(before, 10, 32)
		if err != nil || id == 0 {
			respondError(c, http.StatusBadRequest, "INVALID_AUDIT_LOG_CURSOR", "before_id 无效", nil)
			return
		}
		query = query.Where("id < ?", uint(id))
	}

	var rows []models.AuditLog
	if err := query.Find(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "AUDIT_LOG_READ_FAILED", "读取审计诊断失败", nil)
		return
	}
	actorIDs := make([]uint, 0, len(rows))
	seen := map[uint]struct{}{}
	for _, row := range rows {
		if row.UserID != 0 {
			if _, exists := seen[row.UserID]; !exists {
				seen[row.UserID] = struct{}{}
				actorIDs = append(actorIDs, row.UserID)
			}
		}
	}
	actors := make(map[uint]struct {
		ID    uint
		Name  string
		Email string
	}, len(actorIDs))
	if len(actorIDs) > 0 {
		var employees []models.Employee
		if err := models.DB.Select("id, name, email").Where("id IN ?", actorIDs).Find(&employees).Error; err != nil {
			respondError(c, http.StatusInternalServerError, "AUDIT_LOG_ACTOR_READ_FAILED", "读取审计操作者失败", nil)
			return
		}
		for _, employee := range employees {
			actors[employee.ID] = struct {
				ID    uint
				Name  string
				Email string
			}{ID: employee.ID, Name: employee.Name, Email: employee.Email}
		}
	}

	result := make([]auditLogView, 0, len(rows))
	for _, row := range rows {
		view := auditLogView{ID: row.ID, Action: row.Action, Resource: row.Resource, ResourceID: row.ResourceID, Result: row.Result, Details: row.Details, IPAddress: row.IPAddress, CreatedAt: row.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")}
		if actor, ok := actors[row.UserID]; ok {
			view.Actor = &struct {
				ID    uint   `json:"id"`
				Name  string `json:"name"`
				Email string `json:"email"`
			}{ID: actor.ID, Name: actor.Name, Email: actor.Email}
		}
		result = append(result, view)
	}
	respondData(c, http.StatusOK, map[string]any{"logs": result, "next_before_id": nextAuditLogCursor(rows)}, "审计诊断读取成功")
}

func nextAuditLogCursor(rows []models.AuditLog) any {
	if len(rows) == 0 {
		return nil
	}
	return rows[len(rows)-1].ID
}
