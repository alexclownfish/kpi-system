package models

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPostgresDSNFromPasswordFile(t *testing.T) {
	password := "space and:@/#?%"
	passwordFile := filepath.Join(t.TempDir(), "postgres-password")
	if err := os.WriteFile(passwordFile, []byte(password+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("PGHOST", "::1")
	t.Setenv("PGPORT", "5433")
	t.Setenv("PGUSER", "kpi_user")
	t.Setenv("PGDATABASE", "kpi")
	t.Setenv("PGPASSWORD_FILE", passwordFile)
	t.Setenv("PGSSLMODE", "require")

	dialector, err := configuredDatabaseDialector()
	if err != nil || dialector.Name() != "postgres" {
		t.Fatalf("PostgreSQL dialector type: %T, error: %v", dialector, err)
	}
	dsn, err := postgresDSNFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	gotPassword, ok := parsed.User.Password()
	if !ok || gotPassword != password || parsed.Host != "[::1]:5433" || parsed.Path != "/kpi" || parsed.Query().Get("sslmode") != "require" {
		t.Fatalf("invalid PostgreSQL connection URL: host=%q, path=%q, sslmode=%q, password_matched=%v", parsed.Host, parsed.Path, parsed.Query().Get("sslmode"), gotPassword == password)
	}
}

func TestPostgresConfigurationFailsClosed(t *testing.T) {
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("PGHOST", "")
	t.Setenv("PGUSER", "")
	t.Setenv("PGDATABASE", "")
	t.Setenv("PGPASSWORD_FILE", "")
	if _, err := configuredDatabaseDialector(); err == nil {
		t.Fatal("missing PostgreSQL settings must fail without selecting SQLite")
	}
	t.Setenv("DB_DRIVER", "unknown")
	if _, err := configuredDatabaseDialector(); err == nil {
		t.Fatal("unknown database driver must fail")
	}
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("PGHOST", "postgres")
	t.Setenv("PGUSER", "kpi")
	t.Setenv("PGDATABASE", "kpi")
	t.Setenv("PGPORT", "not-a-port")
	t.Setenv("PGPASSWORD_FILE", "/missing/password/file")
	if _, err := configuredDatabaseDialector(); err == nil || strings.Contains(err.Error(), "postgres://") {
		t.Fatalf("invalid port must fail without exposing a DSN: %v", err)
	}
	t.Setenv("PGPORT", "5432")
	if _, err := configuredDatabaseDialector(); err == nil || strings.Contains(err.Error(), "/missing/password/file") {
		t.Fatalf("missing password file must fail without exposing its path: %v", err)
	}
}

func TestPostgresConnectionFailureDoesNotFallback(t *testing.T) {
	password := "never-log-this-password"
	passwordFile := filepath.Join(t.TempDir(), "postgres-password")
	if err := os.WriteFile(passwordFile, []byte(password), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("PGHOST", "127.0.0.1")
	t.Setenv("PGPORT", "1")
	t.Setenv("PGUSER", "kpi")
	t.Setenv("PGDATABASE", "kpi")
	t.Setenv("PGPASSWORD_FILE", passwordFile)
	t.Setenv("PGSSLMODE", "disable")
	db, err := openConfiguredDatabase()
	if err == nil || db != nil {
		t.Fatal("failed PostgreSQL connection must not open a database")
	}
	if strings.Contains(err.Error(), password) || strings.Contains(err.Error(), "postgres://") {
		t.Fatal("connection failure disclosed PostgreSQL credentials")
	}
}

func TestMigrateSchemaDoesNotSeedBusinessRecords(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:postgres-schema-only-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema(db); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"roles", "employees", "departments", "kpi_templates", "kpi_evaluations"} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("schema-only initialization seeded %s: %d rows", table, count)
		}
	}
}
