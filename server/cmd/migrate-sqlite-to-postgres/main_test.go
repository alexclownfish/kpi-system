package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConvertValue(t *testing.T) {
	for _, tc := range []struct {
		value any
		kind  string
		want  any
	}{
		{int64(0), "boolean", false},
		{int64(1), "boolean", true},
		{"1", "boolean", true},
		{[]byte("0"), "boolean", false},
		{[]byte("hello"), "text", "hello"},
		{[]byte{0, 1}, "bytea", []byte{0, 1}},
	} {
		got, err := convertValue(tc.value, tc.kind)
		if err != nil {
			t.Fatalf("convertValue(%v,%s): %v", tc.value, tc.kind, err)
		}
		if fmtValue(got) != fmtValue(tc.want) {
			t.Errorf("convertValue(%v,%s) = %v; want %v", tc.value, tc.kind, got, tc.want)
		}
	}
	if _, err := convertValue(int64(2), "boolean"); err == nil {
		t.Fatal("invalid SQLite boolean accepted")
	}
	date, err := convertValue("2026-10-23 10:05:00+08:00", "timestamp with time zone")
	if err != nil {
		t.Fatal(err)
	}
	if got := date.(time.Time).UTC().Format(time.RFC3339); got != "2026-10-23T02:05:00Z" {
		t.Fatal(got)
	}
	if _, err := convertValue("not-a-date", "timestamp without time zone"); err == nil {
		t.Fatal("invalid date accepted")
	}
}

func fmtValue(v any) string {
	if raw, ok := v.([]byte); ok {
		return hex.EncodeToString(raw)
	}
	return fmt.Sprint(v)
}

func TestCheckSQLiteIntegrityAndForeignKeys(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:check-migration-source?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, statement := range []string{"CREATE TABLE parents(id INTEGER PRIMARY KEY)", "CREATE TABLE children(id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES parents(id))"} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkSQLite(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO children(id,parent_id) VALUES (1,999)"); err != nil {
		t.Fatal(err)
	}
	if err := checkSQLite(ctx, db); err == nil || !strings.Contains(err.Error(), "children") {
		t.Fatalf("expected orphan error, got %v", err)
	}
}

func TestCheckSQLiteIdentityIgnoresOnlyUnrelatedForeignKeys(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	for _, statement := range []string{
		"CREATE TABLE departments(id INTEGER PRIMARY KEY)",
		"CREATE TABLE employees(id INTEGER PRIMARY KEY, department_id INTEGER REFERENCES departments(id))",
		"CREATE TABLE kpi_evaluations(id INTEGER PRIMARY KEY, employee_id INTEGER REFERENCES employees(id))",
		"INSERT INTO kpi_evaluations(id,employee_id) VALUES (1,999)",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	skipped, err := checkSQLiteScope(ctx, db, scopeIdentity)
	if err != nil || skipped != 1 {
		t.Fatalf("identity check: skipped=%d, err=%v", skipped, err)
	}
	if err := checkSQLite(ctx, db); err == nil || !strings.Contains(err.Error(), "kpi_evaluations") {
		t.Fatalf("full check accepted orphan: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO employees(id,department_id) VALUES (1,999)"); err != nil {
		t.Fatal(err)
	}
	if _, err := checkSQLiteScope(ctx, db, scopeIdentity); err == nil || !strings.Contains(err.Error(), "employees") {
		t.Fatalf("identity check accepted identity orphan: %v", err)
	}
}

func TestSelectSourceTablesIdentity(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	createIdentityFixture(t, db)
	if _, err := db.ExecContext(ctx, "CREATE TABLE kpi_evaluations(id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO kpi_evaluations VALUES (42)"); err != nil {
		t.Fatal(err)
	}
	tables, err := listSQLiteTables(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	selected, excluded, err := selectSourceTables(ctx, db, tables, scopeIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 9 || len(excluded) != 1 || excluded["kpi_evaluations"] != 1 {
		t.Fatalf("selected=%v excluded=%v", selected, excluded)
	}
	full, empty, err := selectSourceTables(ctx, db, tables, scopeFull)
	if err != nil || len(full) != 10 || len(empty) != 0 {
		t.Fatalf("full=%v excluded=%v err=%v", full, empty, err)
	}
	if _, err := db.ExecContext(ctx, "DROP TABLE roles"); err != nil {
		t.Fatal(err)
	}
	tables, err = listSQLiteTables(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := selectSourceTables(ctx, db, tables, scopeIdentity); err == nil || !strings.Contains(err.Error(), "roles") {
		t.Fatalf("missing table accepted: %v", err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE roles(id INTEGER PRIMARY KEY,code TEXT,name TEXT,description TEXT,is_system INTEGER,requires_manager INTEGER,created_at TEXT,updated_at TEXT)"); err != nil {
		t.Fatal(err)
	}
	tables, err = listSQLiteTables(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := selectSourceTables(ctx, db, tables, scopeIdentity); err == nil || !strings.Contains(err.Error(), "roles.access_customized") {
		t.Fatalf("missing access_customized accepted: %v", err)
	}
}

func createIdentityFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, statement := range []string{
		"CREATE TABLE departments(id INTEGER PRIMARY KEY,name TEXT,description TEXT,parent_id INTEGER,leader_id INTEGER,created_at TEXT,updated_at TEXT)",
		"CREATE TABLE job_positions(id INTEGER PRIMARY KEY,department_id INTEGER,name TEXT,created_at TEXT,updated_at TEXT)",
		"CREATE TABLE employees(id INTEGER PRIMARY KEY,name TEXT,email TEXT,doo_task_user_id INTEGER,password TEXT,position TEXT,department_id INTEGER,manager_id INTEGER,role TEXT,is_active INTEGER,created_at TEXT,updated_at TEXT)",
		"CREATE TABLE roles(id INTEGER PRIMARY KEY,code TEXT,name TEXT,description TEXT,is_system INTEGER,requires_manager INTEGER,access_customized INTEGER,created_at TEXT,updated_at TEXT)",
		"CREATE TABLE permissions(id INTEGER PRIMARY KEY,code TEXT,name TEXT,resource TEXT,action TEXT,description TEXT,created_at TEXT,updated_at TEXT)",
		"CREATE TABLE user_roles(user_id INTEGER,role_id INTEGER,created_at TEXT,PRIMARY KEY(user_id,role_id))",
		"CREATE TABLE role_permissions(role_id INTEGER,permission_id INTEGER,created_at TEXT,PRIMARY KEY(role_id,permission_id))",
		"CREATE TABLE data_scopes(id INTEGER PRIMARY KEY,code TEXT,name TEXT,description TEXT)",
		"CREATE TABLE role_data_scopes(role_id INTEGER,data_scope_id INTEGER,created_at TEXT,PRIMARY KEY(role_id,data_scope_id))",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIdentityRelationshipsRejectMissingLinks(t *testing.T) {
	for _, tc := range []struct{ name, statement, field string }{
		{"department parent", "INSERT INTO departments(id,parent_id) VALUES (2,999)", "departments.parent_id"},
		{"department leader", "INSERT INTO departments(id,leader_id) VALUES (2,999)", "departments.leader_id"},
		{"job position department", "INSERT INTO job_positions(id,department_id) VALUES (2,999)", "job_positions.department_id"},
		{"employee department", "INSERT INTO employees(id,department_id) VALUES (2,999)", "employees.department_id"},
		{"employee manager", "INSERT INTO employees(id,department_id,manager_id) VALUES (2,1,999)", "employees.manager_id"},
		{"user role user", "INSERT INTO user_roles(user_id,role_id) VALUES (999,1)", "user_roles.user_id"},
		{"user role role", "INSERT INTO user_roles(user_id,role_id) VALUES (1,999)", "user_roles.role_id"},
		{"role permission permission", "INSERT INTO role_permissions(role_id,permission_id) VALUES (1,999)", "role_permissions.permission_id"},
		{"role scope scope", "INSERT INTO role_data_scopes(role_id,data_scope_id) VALUES (1,999)", "role_data_scopes.data_scope_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "source.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			createIdentityFixture(t, db)
			for _, statement := range []string{"INSERT INTO departments(id) VALUES (1)", "INSERT INTO employees(id,department_id,role,is_active) VALUES (1,1,'super_admin',1)", "INSERT INTO roles(id,code) VALUES (1,'super_admin')", "INSERT INTO data_scopes(id) VALUES (1)", tc.statement} {
				if _, err := db.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			if err := verifyIdentityRelationships(context.Background(), db, ""); err == nil || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("expected %s orphan, got %v", tc.field, err)
			}
		})
	}
}

func TestIdentityDepartmentHierarchyRejectsCycles(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createIdentityFixture(t, db)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "INSERT INTO departments(id,parent_id) VALUES (1,NULL),(2,1)"); err != nil {
		t.Fatal(err)
	}
	if err := verifyIdentityDepartmentHierarchy(ctx, db, ""); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, parent string }{
		{"self-reference", "1"},
		{"two-department cycle", "2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.ExecContext(ctx, "UPDATE departments SET parent_id="+tc.parent+" WHERE id=1"); err != nil {
				t.Fatal(err)
			}
			if err := verifyIdentityDepartmentHierarchy(ctx, db, ""); err == nil || !strings.Contains(err.Error(), "父级循环") {
				t.Fatalf("expected cycle rejection, got %v", err)
			}
		})
	}
}

func TestIdentityRolesRejectDriftAndScopeDuplicates(t *testing.T) {
	for _, tc := range []struct{ name, statement, expect string }{
		{"role drift", "UPDATE employees SET role='employee' WHERE id=1", "角色不一致"},
		{"missing super admin", "UPDATE employees SET is_active=0 WHERE id=1", "没有有效超级管理员"},
		{"duplicate scope", "INSERT INTO role_data_scopes(role_id,data_scope_id) VALUES (1,2)", "数据范围绑定数"},
		{"missing scope", "DELETE FROM role_data_scopes WHERE role_id=1", "数据范围绑定数"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "source.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			createIdentityFixture(t, db)
			for _, statement := range []string{"INSERT INTO departments(id) VALUES (1)", "INSERT INTO employees(id,department_id,role,is_active) VALUES (1,1,'super_admin',1)", "INSERT INTO roles(id,code) VALUES (1,'super_admin')", "INSERT INTO user_roles(user_id,role_id) VALUES (1,1)", "INSERT INTO data_scopes(id) VALUES (1),(2)", "INSERT INTO role_data_scopes(role_id,data_scope_id) VALUES (1,1)"} {
				if _, err := db.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			if err := verifyIdentityRoles(context.Background(), db, ""); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.statement); err != nil {
				t.Fatal(err)
			}
			if err := verifyIdentityRoles(context.Background(), db, ""); err == nil || !strings.Contains(err.Error(), tc.expect) {
				t.Fatalf("expected %q, got %v", tc.expect, err)
			}
		})
	}
}

func TestIdentityRoleCode(t *testing.T) {
	for input, want := range map[string]string{"hr": "hr_admin", "manager": "department_manager", "": "employee", "cto": "cto", "Super_Admin": "super_admin"} {
		if got := identityRoleCode(input); got != want {
			t.Errorf("%q maps to %q; want %q", input, got, want)
		}
	}
}

func TestValidateSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.db")
	if err := os.WriteFile(path, []byte("independent-test-snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("independent-test-snapshot"))
	if err := validateSnapshot(path, hex.EncodeToString(digest[:])); err != nil {
		t.Fatal(err)
	}
	if err := validateSnapshot(path, strings.Repeat("0", 64)); err == nil {
		t.Fatal("bad hash accepted")
	}
	if err := os.WriteFile(path+"-wal", []byte("pending WAL"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateSnapshot(path, hex.EncodeToString(digest[:])); err == nil {
		t.Fatal("WAL sidecar accepted")
	}
}

func TestQuoteIdentifier(t *testing.T) {
	if got := q("a\"b"); got != "\"a\"\"b\"" {
		t.Fatal(got)
	}
}

func TestPrimaryKeyDigestIgnoresRowOrder(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:pk-digest-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE ids(a INTEGER, b TEXT, PRIMARY KEY(a,b))"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO ids(a,b) VALUES (1,'x'),(2,'y'),(3,'z')"); err != nil {
		t.Fatal(err)
	}
	digest := func(query string) (int64, [32]byte) {
		rows, err := db.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		count, sum, err := digestPrimaryKeys(rows, 2)
		if err != nil {
			t.Fatal(err)
		}
		return count, sum
	}
	aCount, a := digest("SELECT a,b FROM ids ORDER BY a")
	bCount, b := digest("SELECT a,b FROM ids ORDER BY a DESC")
	cCount, c := digest("SELECT a,b FROM ids WHERE a<>2")
	if aCount != bCount || a != b {
		t.Fatal("row order changed the primary-key digest")
	}
	if aCount == cCount || a == c {
		t.Fatal("missing primary key went undetected")
	}
}
