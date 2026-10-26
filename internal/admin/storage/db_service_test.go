package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	magicDB "callit/internal/magic-api/db"
)

func openStorageDB(t *testing.T, statements ...string) *DBService {
	t.Helper()
	workerDB, err := magicDB.OpenSQLiteService(filepath.Join(t.TempDir(), "worker.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workerDB.Close() })
	for _, statement := range statements {
		if _, err := workerDB.SQLDB().Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return NewDBService(workerDB.SQLDB())
}

func queryStorageDB(t *testing.T, service *DBService, query DBQuery) DBQueryResult {
	t.Helper()
	result, err := service.Query(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestStorageDBQueryAndSchema(t *testing.T) {
	service := openStorageDB(t,
		`CREATE TABLE records (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL DEFAULT '未命名', score REAL, display TEXT GENERATED ALWAYS AS (name || '!') VIRTUAL)`,
		`INSERT INTO records (name, score) VALUES ('甲', 10), ('乙', 20), ('丙', 30)`,
		`CREATE VIEW record_view AS SELECT * FROM records`,
	)
	tables, err := service.Tables(context.Background())
	if err != nil || !reflect.DeepEqual(tables, []string{"records"}) {
		t.Fatalf("表列表不正确: %v, %v", tables, err)
	}
	result := queryStorageDB(t, service, DBQuery{Table: "records", Where: "score >= 20", SortColumn: "score", SortDirection: "desc", PageSize: 1, Page: 2})
	if result.Total != 2 || result.Page != 2 || len(result.Rows) != 1 || result.Rows[0].Values["name"].Value != "乙" {
		t.Fatalf("筛选、排序或分页错误: %+v", result)
	}
	if !result.Table.Columns[3].Generated || !result.Table.Columns[1].NotNull || *result.Table.Columns[1].DefaultValue != "'未命名'" {
		t.Fatalf("表结构不正确: %+v", result.Table)
	}
	if result.Rows[0].Identity["id"].Value != "2" {
		t.Fatalf("行标识不正确: %+v", result.Rows[0])
	}
	empty := queryStorageDB(t, service, DBQuery{Table: "records", Where: "id < 0", Page: 10})
	if empty.Total != 0 || empty.Page != 1 || len(empty.Table.Columns) != 4 || empty.Rows == nil {
		t.Fatalf("空查询也应返回表结构和空数组: %+v", empty)
	}
}

func TestStorageDBTypedValuesAndWrites(t *testing.T) {
	service := openStorageDB(t,
		`CREATE TABLE records (id INTEGER PRIMARY KEY, label TEXT DEFAULT '默认', optional TEXT, payload BLOB, happened_at DATETIME, amount REAL, size INTEGER)`,
		`INSERT INTO records VALUES (9223372036854775807, '', NULL, X'00ff80', '2026-10-26 08:09:10', 1.25, 9007199254740993)`,
	)
	result := queryStorageDB(t, service, DBQuery{Table: "records"})
	row := result.Rows[0]
	expected := map[string]DBValue{
		"id":          {Type: "integer", Value: "9223372036854775807"},
		"label":       {Type: "text", Value: ""},
		"optional":    {Type: "null"},
		"payload":     {Type: "blob", Value: "00ff80"},
		"happened_at": {Type: "text", Value: "2026-10-26 08:09:10"},
		"amount":      {Type: "real", Value: "1.25"},
		"size":        {Type: "integer", Value: "9007199254740993"},
	}
	if !reflect.DeepEqual(row.Values, expected) {
		t.Fatalf("字段类型或精度丢失: %+v", row.Values)
	}
	if err := service.Write(context.Background(), DBWriteRequest{Table: "records", DBRowReference: row.DBRowReference, Values: map[string]DBValue{"label": {Type: "text", Value: "'; DELETE FROM records; --"}}}, false); err != nil {
		t.Fatal(err)
	}
	if err := service.Write(context.Background(), DBWriteRequest{Table: "records", Values: map[string]DBValue{"id": {Type: "integer", Value: "1"}}}, true); err != nil {
		t.Fatal(err)
	}
	result = queryStorageDB(t, service, DBQuery{Table: "records"})
	if result.Total != 2 || result.Rows[0].Values["label"].Value != "默认" || result.Rows[1].Values["label"].Value != "'; DELETE FROM records; --" {
		t.Fatalf("默认值或参数化写入错误: %+v", result.Rows)
	}
	if result.Rows[1].Values["happened_at"] != expected["happened_at"] || result.Rows[1].Values["size"] != expected["size"] {
		t.Fatalf("编辑污染了未修改字段: %+v", result.Rows[1])
	}
	for _, value := range []DBValue{{Type: "integer", Value: "9223372036854775808"}, {Type: "integer", Value: "abc"}, {Type: "real", Value: "NaN"}, {Type: "blob", Value: "xyz"}} {
		if err := service.Write(context.Background(), DBWriteRequest{Table: "records", DBRowReference: row.DBRowReference, Values: map[string]DBValue{"label": value}}, false); err == nil {
			t.Fatalf("非法值应拒绝: %+v", value)
		}
	}
}

func TestStorageDBRowIdentity(t *testing.T) {
	tests := []struct {
		name       string
		definition string
		insert     string
	}{
		{name: "无主键重复行", definition: `CREATE TABLE records (value TEXT)`, insert: `INSERT INTO records VALUES ('重复'), ('重复')`},
		{name: "复合主键", definition: `CREATE TABLE records (tenant TEXT, id INTEGER, value TEXT, PRIMARY KEY (tenant, id)) WITHOUT ROWID`, insert: `INSERT INTO records VALUES ('甲', 1, '重复'), ('乙', 1, '重复')`},
		{name: "行号被同名字段遮挡", definition: `CREATE TABLE records (_rowid_ TEXT, value TEXT)`, insert: `INSERT INTO records VALUES ('重复', '重复'), ('重复', '重复')`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := openStorageDB(t, test.definition, test.insert)
			result := queryStorageDB(t, service, DBQuery{Table: "records"})
			if err := service.Write(context.Background(), DBWriteRequest{Table: "records", DBRowReference: result.Rows[0].DBRowReference, Values: map[string]DBValue{"value": {Type: "text", Value: "已修改"}}}, false); err != nil {
				t.Fatal(err)
			}
			if err := service.Delete(context.Background(), DBDeleteRequest{Table: "records", Rows: []DBRowReference{result.Rows[1].DBRowReference}}); err != nil {
				t.Fatal(err)
			}
			remaining := queryStorageDB(t, service, DBQuery{Table: "records"})
			if remaining.Total != 1 || remaining.Rows[0].Values["value"].Value != "已修改" {
				t.Fatalf("操作未准确定位单行: %+v", remaining.Rows)
			}
		})
	}
}

func TestStorageDBDeleteRollback(t *testing.T) {
	service := openStorageDB(t,
		`CREATE TABLE records (id INTEGER PRIMARY KEY)`,
		`INSERT INTO records VALUES (1), (2), (3)`,
		`CREATE TRIGGER protect_record BEFORE DELETE ON records WHEN OLD.id = 2 BEGIN SELECT RAISE(ABORT, '禁止删除'); END`,
	)
	rows := queryStorageDB(t, service, DBQuery{Table: "records"}).Rows
	request := DBDeleteRequest{Table: "records", Rows: []DBRowReference{rows[0].DBRowReference, rows[1].DBRowReference}}
	if err := service.Delete(context.Background(), request); err == nil {
		t.Fatal("触发器拒绝时应返回错误")
	}
	if result := queryStorageDB(t, service, DBQuery{Table: "records"}); result.Total != 3 {
		t.Fatal("批量删除失败后未全部回滚")
	}
	request.Rows = []DBRowReference{rows[0].DBRowReference, rows[0].DBRowReference}
	if err := service.Delete(context.Background(), request); err == nil {
		t.Fatal("重复或失效的行标识应使整个事务回滚")
	}
	request.Rows = []DBRowReference{rows[0].DBRowReference, rows[2].DBRowReference}
	if err := service.Delete(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if result := queryStorageDB(t, service, DBQuery{Table: "records", Page: 3, PageSize: 1}); result.Total != 1 || result.Page != 1 {
		t.Fatalf("删除后应回到有效页: %+v", result)
	}
}

func TestStorageDBRejectInvalidQueries(t *testing.T) {
	service := openStorageDB(t, `CREATE TABLE records (id INTEGER PRIMARY KEY, text TEXT)`, `INSERT INTO records VALUES (1, 'a;--b')`)
	for _, expression := range []string{"1); DELETE FROM records; --", "1) OR 1=1 --", "1/*", "id = '", "id IN (1", "1; SELECT 2", "1) UNION SELECT 1", "1) ORDER BY id (", "missing = 1"} {
		if _, err := service.Query(context.Background(), DBQuery{Table: "records", Where: expression}); err == nil {
			t.Fatalf("应拒绝非法 WHERE: %s", expression)
		}
	}
	valid := queryStorageDB(t, service, DBQuery{Table: "records", Where: `text = 'a;--b' AND (id IN (SELECT id FROM records))`})
	if valid.Total != 1 {
		t.Fatal("合法字符串和子查询不应被拒绝")
	}
	for _, query := range []DBQuery{{Table: "records; DROP TABLE records"}, {Table: "sqlite_schema"}, {Table: "records", SortColumn: "id; DELETE FROM records", SortDirection: "asc"}, {Table: "records", SortColumn: "id", SortDirection: "desc; DELETE FROM records"}} {
		if _, err := service.Query(context.Background(), query); err == nil {
			t.Fatalf("应拒绝非法查询: %+v", query)
		}
	}
	if err := service.Write(context.Background(), DBWriteRequest{Table: "records", Values: map[string]DBValue{"text": {Type: "text", Value: "危险"}}}, false); err == nil {
		t.Fatal("缺少行标识时不得更新全表")
	}
}

func TestStorageDBQuotedIdentifiers(t *testing.T) {
	service := openStorageDB(t, `CREATE TABLE "订单""表" ("select" TEXT, "字段 空格" TEXT)`)
	name := "订单\"表"
	if err := service.Write(context.Background(), DBWriteRequest{Table: name, Values: map[string]DBValue{"select": {Type: "text", Value: "内容"}, "字段 空格": {Type: "null"}}}, true); err != nil {
		t.Fatal(err)
	}
	result := queryStorageDB(t, service, DBQuery{Table: name, Where: `"select" = '内容'`, SortColumn: "字段 空格", SortDirection: "asc"})
	if result.Total != 1 || !strings.Contains(result.Table.SQL, "订单") {
		t.Fatalf("特殊标识符查询错误: %+v", result)
	}
}
