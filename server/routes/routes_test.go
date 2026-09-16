package routes

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetupRoutesRegistersRBACEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	group := r.Group("/api")
	SetupRoutes(group)
	for _, path := range []string{"/api/roles", "/api/permissions", "/api/employees/:id/roles"} {
		found := false
		for _, route := range r.Routes() {
			if route.Path == path {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("route %s was not registered", path)
		}
	}
}
