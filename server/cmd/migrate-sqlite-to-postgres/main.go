// Command migrate-sqlite-to-postgres copies a consistent, independently backed-up
// KPI SQLite database into an empty PostgreSQL schema. It never reads or changes
// the running application's SQLite database in place.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type column struct {
	name string
	dataType string
	pkOrder int
}

type foreignKey struct {
	name, childSchema, childTable, parentSchema, parentTable string
	matchType string
	childColumns, parentColumns []string
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "MIGRATION_RESULT=failed")
		fmt.Fprintln(os.Stderr, "迁移失败:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("migrate-sqlite-to-postgres", flag.ContinueOnError)
	source := flags.String("source", "", "SQLite 一致性快照文件；不是运行中的原库")
	expectedHash := flags.String("source-sha256", "", "快照文件的 SHA-256 十六进制摘要")
	expectedDB := flags.String("expect-database", "", "明确确认 PostgreSQL 目标数据库名")
	confirmEmpty := flags.Bool("confirm-empty-target", false, "明确确认目标是新的空数据库")
	if err := flags.Parse(args); err != nil { return err }
	if len(flags.Args()) != 0 { return errors.New("不接受额外参数") }
	if !*confirmEmpty || *expectedDB == "" || *source == "" || len(*expectedHash) != 64 {
		return errors.New("必须指定 --source、--source-sha256、--expect-database 和 --confirm-empty-target")
	}
	if _, err := hex.DecodeString(*expectedHash); err != nil { return errors.New("--source-sha256 必须是 64 位十六进制 SHA-256") }
	if strings.ToLower(strings.TrimSpace(os.Getenv("DB_DRIVER"))) != "postgres" {
		return errors.New("目标必须显式设置 DB_DRIVER=postgres")
	}
	if *expectedDB != os.Getenv("PGDATABASE") {
		return errors.New("--expect-database 与 PGDATABASE 不一致")
	}
	path, err := filepath.Abs(*source)
	if err != nil { return err }
	if err := validateSnapshot(path, *expectedHash); err != nil { return err }
	sqliteURL := (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro&immutable=1"}).String()
	sourceDB, err := sql.Open("sqlite3", sqliteURL)
	if err != nil { return fmt.Errorf("打开 SQLite 快照失败: %w", err) }
	defer sourceDB.Close()
	sourceDB.SetMaxOpenConns(1)
	if _, err := sourceDB.ExecContext(ctx, "PRAGMA query_only=ON"); err != nil { return err }
	if err := checkSQLite(ctx, sourceDB); err != nil { return err }
	sourceTables, err := listSQLiteTables(ctx, sourceDB)
	if err != nil { return err }
	if len(sourceTables) == 0 { return errors.New("SQLite 快照中没有业务表，拒绝迁移") }
	postgresDB, err := openPostgres()
	if err != nil { return err }
	defer postgresDB.Close()
	tx, err := postgresDB.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("开启 PostgreSQL 事务失败: %w", err) }
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(740194, 2026)"); err != nil { return err }
	var schema string
	if err := tx.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schema); err != nil { return err }
	if schema != "public" { return fmt.Errorf("目标 schema 为 %q，仅支持 public", schema) }
	targetTables, err := listPostgresTables(ctx, tx)
	if err != nil { return err }
	if len(targetTables) == 0 { return errors.New("PostgreSQL 尚未初始化 schema；先执行 kpi-server init-schema") }
	targetSet := make(map[string]bool, len(targetTables))
	for _, table := range targetTables {
		targetSet[table] = true
		var count int64
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM "+q(schema)+"."+q(table)).Scan(&count); err != nil { return err }
		if count != 0 { return fmt.Errorf("目标表 %s 已有 %d 行：拒绝覆盖或混入旧数据", table, count) }
	}
	for _, table := range sourceTables {
		if !targetSet[table] { return fmt.Errorf("SQLite 表 %s 在目标 schema 中不存在；为避免丢数据已停止", table) }
	}
	// GORM's constraints are not DEFERRABLE and the schema has cyclic employee /
	// department references. This setting is LOCAL to this transaction and needs
	// the dedicated migration PostgreSQL role to be a superuser. Every FK is
	// checked against the copied rows before commit.
	if _, err := tx.ExecContext(ctx, "SET LOCAL session_replication_role = replica"); err != nil {
		return errors.New("迁移角色无权暂时延迟外键触发器；请用全新 PostgreSQL 实例的管理员执行")
	}
	sequenceValues, err := readSQLiteSequences(ctx, sourceDB)
	if err != nil { return err }
	counts := make(map[string]int64, len(sourceTables))
	for _, table := range sourceTables {
		count, err := copyTable(ctx, sourceDB, tx, schema, table)
		if err != nil { return fmt.Errorf("复制表 %s 失败: %w", table, err) }
		counts[table] = count
		fmt.Printf("已复制 %-40s %d 行\n", table, count)
	}
	if _, err := tx.ExecContext(ctx, "SET LOCAL session_replication_role = origin"); err != nil { return err }
	if err := verifyCounts(ctx, tx, schema, counts); err != nil { return err }
	if err := verifyPrimaryKeys(ctx, sourceDB, tx, schema, sourceTables); err != nil { return err }
	if err := verifyForeignKeys(ctx, tx, schema); err != nil { return err }
	if err := restartSequences(ctx, tx, schema, sequenceValues); err != nil { return err }
	if err := validateSnapshot(path, *expectedHash); err != nil { return fmt.Errorf("迁移期间源快照发生变化: %w", err) }
	if err := tx.Commit(); err != nil { return fmt.Errorf("提交 PostgreSQL 迁移失败: %w", err) }
	report, err := json.Marshal(map[string]any{"status":"committed", "source_sha256":strings.ToLower(*expectedHash), "target_database":*expectedDB, "table_row_counts":counts, "checks":map[string]string{"row_counts":"passed", "primary_keys":"passed", "foreign_keys":"passed", "sequences":"passed"}})
	if err == nil { fmt.Printf("MIGRATION_REPORT_JSON=%s\n", report) }
	fmt.Println("MIGRATION_RESULT=committed")
	fmt.Printf("迁移完成：%d 张 SQLite 业务表，全部行数及 PostgreSQL 外键已核对。请继续核对附件备份并运行应用验收。\n", len(sourceTables))
	return nil
}

func q(s string) string { return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\"" }

func validateSnapshot(path, expectedHash string) error {
	info, err := os.Lstat(path)
	if err != nil { return fmt.Errorf("读取快照失败: %w", err) }
	if !info.Mode().IsRegular() || info.Size() == 0 { return errors.New("SQLite 快照必须是非空普通文件，不能使用符号链接") }
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(path + suffix); err == nil { return fmt.Errorf("快照存在 %s 辅助文件；先用 SQLite .backup 制作独立一致性快照", suffix) } else if !os.IsNotExist(err) { return err }
	}
	file, err := os.Open(path)
	if err != nil { return err }
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil { return err }
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), expectedHash) { return errors.New("SQLite 快照 SHA-256 与 --source-sha256 不一致") }
	return nil
}

func checkSQLite(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil { return err }
	defer rows.Close()
	seen := false
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil { return err }
		if result != "ok" { return fmt.Errorf("SQLite 完整性检查失败: %s", result) }
		seen = true
	}
	if err := rows.Err(); err != nil { return err }
	if !seen { return errors.New("SQLite 完整性检查没有返回结果") }
	if err := rows.Close(); err != nil { return err }
	fks, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil { return err }
	defer fks.Close()
	if fks.Next() {
		var table string
		var rowid, parent, id any
		if err := fks.Scan(&table, &rowid, &parent, &id); err != nil { return err }
		return fmt.Errorf("SQLite 原库已有外键孤儿：表 %s，行 %v，引用 %v", table, rowid, parent)
	}
	return fks.Err()
}

func listSQLiteTables(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil { return nil, err }
	defer rows.Close()
	var tables []string
	for rows.Next() { var name string; if err := rows.Scan(&name); err != nil { return nil, err }; tables = append(tables, name) }
	return tables, rows.Err()
}

func listPostgresTables(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT tablename FROM pg_tables WHERE schemaname=current_schema() ORDER BY tablename")
	if err != nil { return nil, err }
	defer rows.Close()
	var tables []string
	for rows.Next() { var name string; if err := rows.Scan(&name); err != nil { return nil, err }; tables = append(tables, name) }
	return tables, rows.Err()
}

func sqliteColumns(ctx context.Context, db *sql.DB, table string) ([]column, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+q(table)+")")
	if err != nil { return nil, err }
	defer rows.Close()
	var cols []column
	for rows.Next() {
		var ordinal, notNull, pk int
		var name, kind string
		var defaultValue sql.NullString
		if err := rows.Scan(&ordinal, &name, &kind, &notNull, &defaultValue, &pk); err != nil { return nil, err }
		cols = append(cols, column{name: name, dataType: kind, pkOrder: pk})
	}
	return cols, rows.Err()
}

func postgresColumns(ctx context.Context, tx *sql.Tx, schema, table string) (map[string]column, error) {
	rows, err := tx.QueryContext(ctx, "SELECT column_name, data_type FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2", schema, table)
	if err != nil { return nil, err }
	defer rows.Close()
	cols := map[string]column{}
	for rows.Next() { var c column; if err := rows.Scan(&c.name, &c.dataType); err != nil { return nil, err }; cols[c.name] = c }
	return cols, rows.Err()
}

func copyTable(ctx context.Context, source *sql.DB, tx *sql.Tx, schema, table string) (int64, error) {
	cols, err := sqliteColumns(ctx, source, table)
	if err != nil { return 0, err }
	if len(cols) == 0 { return 0, errors.New("源表没有列") }
	targetCols, err := postgresColumns(ctx, tx, schema, table)
	if err != nil { return 0, err }
	colNames := make([]string, len(cols))
	params := make([]string, len(cols))
	for i, col := range cols {
		if _, ok := targetCols[col.name]; !ok { return 0, fmt.Errorf("目标表缺少源列 %q；拒绝丢弃字段", col.name) }
		colNames[i] = q(col.name)
		params[i] = "$" + strconv.Itoa(i+1)
	}
	selectSQL := "SELECT " + strings.Join(colNames, ",") + " FROM " + q(table)
	insertSQL := "INSERT INTO " + q(schema) + "." + q(table) + " (" + strings.Join(colNames, ",") + ") VALUES (" + strings.Join(params, ",") + ")"
	rows, err := source.QueryContext(ctx, selectSQL)
	if err != nil { return 0, err }
	defer rows.Close()
	stmt, err := tx.PrepareContext(ctx, insertSQL)
	if err != nil { return 0, err }
	defer stmt.Close()
	values := make([]any, len(cols))
	refs := make([]any, len(cols))
	var count int64
	for rows.Next() {
		for i := range values { refs[i] = &values[i] }
		if err := rows.Scan(refs...); err != nil { return 0, err }
		for i, col := range cols {
			v, err := convertValue(values[i], targetCols[col.name].dataType)
			if err != nil { return 0, fmt.Errorf("第 %d 行列 %s: %w", count+1, col.name, err) }
			values[i] = v
		}
		if _, err := stmt.ExecContext(ctx, values...); err != nil { return 0, fmt.Errorf("第 %d 行写入失败: %w", count+1, err) }
		count++
	}
	if err := rows.Err(); err != nil { return 0, err }
	return count, nil
}

func convertValue(value any, dataType string) (any, error) {
	if value == nil { return nil, nil }
	if dataType == "boolean" {
		switch v := value.(type) {
		case bool: return v, nil
		case int64: if v == 0 || v == 1 { return v == 1, nil }
		case string: if v == "0" { return false, nil }; if v == "1" { return true, nil }; return strconv.ParseBool(v)
		case []byte: return convertValue(string(v), dataType)
		}
		return nil, fmt.Errorf("不是有效布尔值: %v", value)
	}
	if dataType == "timestamp without time zone" || dataType == "timestamp with time zone" || dataType == "date" {
		if _, ok := value.(time.Time); ok { return value, nil }
		var s string
		switch v := value.(type) { case string: s = v; case []byte: s = string(v); default: return nil, fmt.Errorf("不是有效日期类型: %T", value) }
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05-07:00", "2006-01-02 15:04:05.999999999 -0700 MST", "2006-01-02 15:04:05 -0700 MST", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05", "2006-01-02"} {
			if t, err := time.Parse(layout, s); err == nil { return t, nil }
		}
		return nil, fmt.Errorf("无法解析日期 %q", s)
	}
	if raw, ok := value.([]byte); ok && dataType != "bytea" { return string(raw), nil }
	return value, nil
}

func verifyCounts(ctx context.Context, tx *sql.Tx, schema string, expected map[string]int64) error {
	for table, want := range expected {
		var got int64
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM "+q(schema)+"."+q(table)).Scan(&got); err != nil { return err }
		if got != want { return fmt.Errorf("表 %s 行数不符：SQLite=%d PostgreSQL=%d", table, want, got) }
	}
	return nil
}

// Directly copied PK columns plus row counts already preserve identities. The
// independent, order-insensitive digest below catches mistakes in the copy or
// target schema without requiring source and target collation to agree.
func verifyPrimaryKeys(ctx context.Context, source *sql.DB, tx *sql.Tx, schema string, tables []string) error {
	for _, table := range tables {
		cols, err := sqliteColumns(ctx, source, table)
		if err != nil { return err }
		var keys []column
		for _, col := range cols { if col.pkOrder > 0 { keys = append(keys, col) } }
		if len(keys) == 0 { continue }
		sort.Slice(keys, func(i,j int) bool { return keys[i].pkOrder < keys[j].pkOrder })
		quoted := make([]string, len(keys))
		for i, col := range keys { quoted[i] = q(col.name) }
		selected := strings.Join(quoted, ",")
		sourceRows, err := source.QueryContext(ctx, "SELECT "+selected+" FROM "+q(table))
		if err != nil { return err }
		sourceCount, sourceDigest, err := digestPrimaryKeys(sourceRows, len(keys))
		if err != nil { return err }
		targetRows, err := tx.QueryContext(ctx, "SELECT "+selected+" FROM "+q(schema)+"."+q(table))
		if err != nil { return err }
		targetCount, targetDigest, err := digestPrimaryKeys(targetRows, len(keys))
		if err != nil { return err }
		if sourceCount != targetCount || sourceDigest != targetDigest {
			return fmt.Errorf("表 %s 主键集合不一致：SQLite=%d PostgreSQL=%d", table, sourceCount, targetCount)
		}
	}
	return nil
}

func digestPrimaryKeys(rows *sql.Rows, columnCount int) (int64, [32]byte, error) {
	defer rows.Close()
	var count int64
	var digest [32]byte
	values := make([]any, columnCount)
	refs := make([]any, columnCount)
	for rows.Next() {
		for i := range values { refs[i] = &values[i] }
		if err := rows.Scan(refs...); err != nil { return 0, digest, err }
		h := sha256.New()
		for _, value := range values {
			var bytes []byte
			switch v := value.(type) {
			case nil: bytes = []byte{0}
			case []byte: bytes = append([]byte{1}, v...)
			case string: bytes = append([]byte{1}, []byte(v)...)
			case time.Time: bytes = append([]byte{1}, []byte(v.UTC().Format(time.RFC3339Nano))...)
			default: bytes = append([]byte{1}, []byte(fmt.Sprint(v))...)
			}
			var length [8]byte
			binary.BigEndian.PutUint64(length[:], uint64(len(bytes)))
			h.Write(length[:]); h.Write(bytes)
		}
		rowHash := h.Sum(nil)
		for i := range digest { digest[i] ^= rowHash[i] }
		count++
	}
	return count, digest, rows.Err()
}

func listForeignKeys(ctx context.Context, tx *sql.Tx, schema string) ([]foreignKey, error) {
	const query = "SELECT c.oid, c.conname, child_ns.nspname, child.relname, parent_ns.nspname, parent.relname, c.confmatchtype::text, a.attname, b.attname " +
		"FROM pg_constraint c JOIN pg_class child ON child.oid=c.conrelid JOIN pg_namespace child_ns ON child_ns.oid=child.relnamespace " +
		"JOIN pg_class parent ON parent.oid=c.confrelid JOIN pg_namespace parent_ns ON parent_ns.oid=parent.relnamespace " +
		"JOIN LATERAL unnest(c.conkey,c.confkey) WITH ORDINALITY AS k(child_attnum,parent_attnum,ord) ON true " +
		"JOIN pg_attribute a ON a.attrelid=child.oid AND a.attnum=k.child_attnum " +
		"JOIN pg_attribute b ON b.attrelid=parent.oid AND b.attnum=k.parent_attnum " +
		"WHERE c.contype='f' AND child_ns.nspname=$1 ORDER BY c.oid,k.ord"
	rows, err := tx.QueryContext(ctx, query, schema)
	if err != nil { return nil, err }
	defer rows.Close()
	var result []foreignKey
	var previous int64 = -1
	for rows.Next() {
		var oid int64
		var fk foreignKey
		var childCol, parentCol string
		if err := rows.Scan(&oid, &fk.name, &fk.childSchema, &fk.childTable, &fk.parentSchema, &fk.parentTable, &fk.matchType, &childCol, &parentCol); err != nil { return nil, err }
		if oid != previous { result = append(result, fk); previous = oid }
		last := &result[len(result)-1]
		last.childColumns = append(last.childColumns, childCol)
		last.parentColumns = append(last.parentColumns, parentCol)
	}
	return result, rows.Err()
}

func verifyForeignKeys(ctx context.Context, tx *sql.Tx, schema string) error {
	fks, err := listForeignKeys(ctx, tx, schema)
	if err != nil { return err }
	for _, fk := range fks {
		var allNull, anyNull []string
		var pairs []string
		for i, col := range fk.childColumns {
			allNull = append(allNull, "c."+q(col)+" IS NULL")
			anyNull = append(anyNull, "c."+q(col)+" IS NULL")
			pairs = append(pairs, "p."+q(fk.parentColumns[i])+" = c."+q(col))
		}
		nullPass := "(" + strings.Join(anyNull, " OR ") + ")"
		if fk.matchType == "f" { nullPass = "(" + strings.Join(allNull, " AND ") + ")" }
		var check string
		if fk.matchType == "f" {
			check = "(" + nullPass + " OR ((NOT (" + strings.Join(anyNull, " OR ") + ")) AND EXISTS (SELECT 1 FROM " + q(fk.parentSchema) + "." + q(fk.parentTable) + " p WHERE " + strings.Join(pairs, " AND ") + ")))"
		} else {
			check = "(" + nullPass + " OR EXISTS (SELECT 1 FROM " + q(fk.parentSchema) + "." + q(fk.parentTable) + " p WHERE " + strings.Join(pairs, " AND ") + "))"
		}
		query := "SELECT count(*) FROM " + q(fk.childSchema) + "." + q(fk.childTable) + " c WHERE NOT " + check
		var invalid int64
		if err := tx.QueryRowContext(ctx, query).Scan(&invalid); err != nil { return err }
		if invalid != 0 { return fmt.Errorf("目标外键 %s (%s → %s) 有 %d 行不匹配", fk.name, fk.childTable, fk.parentTable, invalid) }
	}
	return nil
}

func readSQLiteSequences(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	var exists int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='sqlite_sequence'").Scan(&exists); err != nil { return nil, err }
	values := map[string]int64{}
	if exists == 0 { return values, nil }
	rows, err := db.QueryContext(ctx, "SELECT name,seq FROM sqlite_sequence")
	if err != nil { return nil, err }
	defer rows.Close()
	for rows.Next() { var name string; var seq int64; if err := rows.Scan(&name,&seq); err != nil { return nil, err }; values[name]=seq }
	return values, rows.Err()
}

func restartSequences(ctx context.Context, tx *sql.Tx, schema string, sqliteSequences map[string]int64) error {
	const query = "SELECT c.table_name,c.column_name,pg_get_serial_sequence(format('%I.%I',c.table_schema,c.table_name),c.column_name) " +
		"FROM information_schema.columns c WHERE c.table_schema=$1 AND (c.column_default LIKE 'nextval(%' OR c.is_identity='YES')"
	rows, err := tx.QueryContext(ctx, query, schema)
	if err != nil { return err }
	type sequence struct{ table, col, name string }
	var sequences []sequence
	for rows.Next() { var s sequence; var name sql.NullString; if err := rows.Scan(&s.table,&s.col,&name); err != nil { rows.Close(); return err }; if name.Valid { s.name=name.String; sequences=append(sequences,s) } }
	if err := rows.Err(); err != nil { rows.Close(); return err }
	rows.Close()
	for _, seq := range sequences {
		var max sql.NullInt64
		if err := tx.QueryRowContext(ctx, "SELECT max("+q(seq.col)+") FROM "+q(schema)+"."+q(seq.table)).Scan(&max); err != nil { return err }
		last := sqliteSequences[seq.table]
		if max.Valid && max.Int64 > last { last = max.Int64 }
		if last < 0 { return fmt.Errorf("序列 %s 的源值无效", seq.name) }
		var seqSchema, seqName string
		if err := tx.QueryRowContext(ctx, "SELECT n.nspname,c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.oid=$1::regclass", seq.name).Scan(&seqSchema,&seqName); err != nil { return err }
		// ALTER SEQUENCE RESTART is transactional, unlike setval().
		sequenceRelation := q(seqSchema)+"."+q(seqName)
		if _, err := tx.ExecContext(ctx, "ALTER SEQUENCE "+sequenceRelation+" RESTART WITH "+strconv.FormatInt(last+1,10)); err != nil { return fmt.Errorf("重置序列 %s 失败: %w", seq.name, err) }
		var actual int64
		var called bool
		if err := tx.QueryRowContext(ctx, "SELECT last_value,is_called FROM "+sequenceRelation).Scan(&actual,&called); err != nil { return err }
		if actual != last+1 || called { return fmt.Errorf("序列 %s 核对失败：期望下一 ID=%d", seq.name, last+1) }
	}
	return nil
}

func openPostgres() (*sql.DB, error) {
	host := strings.TrimSpace(os.Getenv("PGHOST"))
	user := strings.TrimSpace(os.Getenv("PGUSER"))
	database := strings.TrimSpace(os.Getenv("PGDATABASE"))
	passwordFile := strings.TrimSpace(os.Getenv("PGPASSWORD_FILE"))
	if host == "" || user == "" || database == "" || passwordFile == "" { return nil, errors.New("缺少 PGHOST、PGUSER、PGDATABASE 或 PGPASSWORD_FILE") }
	port := strings.TrimSpace(os.Getenv("PGPORT"))
	if port == "" { port = "5432" }
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 { return nil, errors.New("PGPORT 无效") }
	secret, err := os.ReadFile(passwordFile)
	if err != nil { return nil, errors.New("PGPASSWORD_FILE 无法读取") }
	password := strings.TrimRight(string(secret), "\r\n")
	if password == "" { return nil, errors.New("PGPASSWORD_FILE 为空") }
	sslMode := strings.TrimSpace(os.Getenv("PGSSLMODE"))
	if sslMode == "" { sslMode = "disable" }
	dsn := (&url.URL{Scheme:"postgres", User:url.UserPassword(user,password), Host:net.JoinHostPort(host,port), Path:"/"+database, RawQuery:(url.Values{"sslmode":{sslMode}, "connect_timeout":{"5"}}).Encode()}).String()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger:logger.Default.LogMode(logger.Silent)})
	if err != nil { return nil, errors.New("PostgreSQL 连接失败；请检查目标服务与口令文件") }
	return db.DB()
}
