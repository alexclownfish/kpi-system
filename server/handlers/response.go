package handlers

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type apiErrorEnvelope struct {
	Error     apiErrorBody `json:"error"`
	RequestID string       `json:"request_id"`
}

type apiSuccessEnvelope struct {
	Data      any    `json:"data"`
	Message   string `json:"message,omitempty"`
	RequestID string `json:"request_id"`
}

func requestID(c *gin.Context) string {
	if value, exists := c.Get("request_id"); exists {
		if id, ok := value.(string); ok && strings.TrimSpace(id) != "" {
			return id
		}
	}
	id := strings.TrimSpace(c.GetHeader("X-Request-ID"))
	if id == "" {
		id = "req_" + uuid.NewString()
	}
	c.Set("request_id", id)
	c.Header("X-Request-ID", id)
	return id
}

func respondData(c *gin.Context, status int, data any, message string) {
	c.JSON(status, apiSuccessEnvelope{Data: data, Message: message, RequestID: requestID(c)})
}

func respondError(c *gin.Context, status int, code, message string, details any) {
	c.JSON(status, apiErrorEnvelope{
		Error:     apiErrorBody{Code: code, Message: message, Details: details},
		RequestID: requestID(c),
	})
}
