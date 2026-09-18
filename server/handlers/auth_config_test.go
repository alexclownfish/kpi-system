package handlers

import "testing"

func TestInitAuthConfigRejectsMissingProductionSecret(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "")
	if err := InitAuthConfig(); err == nil {
		t.Fatal("missing production JWT secret was accepted")
	}
}

func TestInitAuthConfigRejectsWeakProductionSecret(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "too-short")
	if err := InitAuthConfig(); err == nil {
		t.Fatal("weak production JWT secret was accepted")
	}
}

func TestInitAuthConfigRejectsExampleProductionSecret(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "replace-with-a-random-secret-of-at-least-32-characters")
	if err := InitAuthConfig(); err == nil {
		t.Fatal("example production JWT secret was accepted")
	}
}

func TestInitAuthConfigAcceptsStrongProductionSecret(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "0123456789abcdefghijklmnopqrstuvwxyz-strong")
	if err := InitAuthConfig(); err != nil {
		t.Fatalf("strong production JWT secret rejected: %v", err)
	}
}
