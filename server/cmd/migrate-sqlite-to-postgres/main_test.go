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
	for _, tc := range []struct{ value any; kind string; want any }{
		{int64(0), "boolean", false},
		{int64(1), "boolean", true},
		{"1", "boolean", true},
		{[]byte("0"), "boolean", false},
		{[]byte("hello"), "text", "hello"},
		{[]byte{0, 1}, "bytea", []byte{0, 1}},
	} {
		got, err := convertValue(tc.value, tc.kind)
		if err != nil { t.Fatalf("convertValue(%v,%s): %v", tc.value, tc.kind, err) }
		if fmtValue(got) != fmtValue(tc.want) { t.Errorf("convertValue(%v,%s) = %v; want %v", tc.value, tc.kind, got, tc.want) }
	}
	if _, err := convertValue(int64(2), "boolean"); err == nil { t.Fatal("invalid SQLite boolean accepted") }
	date, err := convertValue("2026-10-23 10:05:00+08:00", "timestamp with time zone")
	if err != nil { t.Fatal(err) }
	if got := date.(time.Time).UTC().Format(time.RFC3339); got != "2026-10-23T02:05:00Z" { t.Fatal(got) }
	if _, err := convertValue("not-a-date", "timestamp without time zone"); err == nil { t.Fatal("invalid date accepted") }
}

func fmtValue(v any) string {
	if raw, ok := v.([]byte); ok { return hex.EncodeToString(raw) }
	return fmt.Sprint(v)
}

func TestCheckSQLiteIntegrityAndForeignKeys(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:check-migration-source?mode=memory&cache=shared")
	if err != nil { t.Fatal(err) }
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, statement := range []string{"CREATE TABLE parents(id INTEGER PRIMARY KEY)", "CREATE TABLE children(id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES parents(id))"} {
		if _, err := db.ExecContext(ctx, statement); err != nil { t.Fatal(err) }
	}
	if err := checkSQLite(ctx, db); err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(ctx, "INSERT INTO children(id,parent_id) VALUES (1,999)"); err != nil { t.Fatal(err) }
	if err := checkSQLite(ctx, db); err == nil || !strings.Contains(err.Error(), "children") { t.Fatalf("expected orphan error, got %v", err) }
}

func TestValidateSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.db")
	if err := os.WriteFile(path, []byte("independent-test-snapshot"), 0600); err != nil { t.Fatal(err) }
	digest := sha256.Sum256([]byte("independent-test-snapshot"))
	if err := validateSnapshot(path, hex.EncodeToString(digest[:])); err != nil { t.Fatal(err) }
	if err := validateSnapshot(path, strings.Repeat("0", 64)); err == nil { t.Fatal("bad hash accepted") }
	if err := os.WriteFile(path+"-wal", []byte("pending WAL"), 0600); err != nil { t.Fatal(err) }
	if err := validateSnapshot(path, hex.EncodeToString(digest[:])); err == nil { t.Fatal("WAL sidecar accepted") }
}

func TestQuoteIdentifier(t *testing.T) {
	if got := q("a\"b"); got != "\"a\"\"b\"" { t.Fatal(got) }
}

func TestPrimaryKeyDigestIgnoresRowOrder(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:pk-digest-test?mode=memory&cache=shared")
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE ids(a INTEGER, b TEXT, PRIMARY KEY(a,b))"); err != nil { t.Fatal(err) }
	if _, err := db.Exec("INSERT INTO ids(a,b) VALUES (1,'x'),(2,'y'),(3,'z')"); err != nil { t.Fatal(err) }
	digest := func(query string) (int64, [32]byte) {
		rows, err := db.Query(query)
		if err != nil { t.Fatal(err) }
		count, sum, err := digestPrimaryKeys(rows, 2)
		if err != nil { t.Fatal(err) }
		return count, sum
	}
	aCount, a := digest("SELECT a,b FROM ids ORDER BY a")
	bCount, b := digest("SELECT a,b FROM ids ORDER BY a DESC")
	cCount, c := digest("SELECT a,b FROM ids WHERE a<>2")
	if aCount != bCount || a != b { t.Fatal("row order changed the primary-key digest") }
	if aCount == cCount || a == c { t.Fatal("missing primary key went undetected") }
}
