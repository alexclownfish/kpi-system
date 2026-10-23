package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dootask-kpi-server/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestLegacyBackupEndpointsRejectPostgres(t *testing.T) {
	db, err := gorm.Open(postgres.Open("host=127.0.0.1 user=test dbname=test sslmode=disable"), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	previous := models.DB
	models.DB = db
	defer func() { models.DB = previous }()

	router := gin.New()
	router.POST("/backups", CreateBackup)
	router.POST("/backups/restore/:filename", RestoreBackup)
	for _, path := range []string{"/backups", "/backups/restore/backup_old.sql"} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, nil)
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNotImplemented {
			t.Errorf("%s: status = %d, want 501", path, response.Code)
		}
		if !strings.Contains(response.Body.String(), "PostgreSQL") {
			t.Errorf("%s: missing PostgreSQL guidance: %s", path, response.Body.String())
		}
	}
}
