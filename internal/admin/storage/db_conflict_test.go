package storage

import (
	"context"
	"reflect"
	"testing"
)

func TestStorageDBRejectReusedRowIdentity(test *testing.T) {
	testCases := []struct {
		name        string
		definition  string
		original    string
		replacement string
	}{
		{
			name:        "非空文本主键",
			definition:  `CREATE TABLE records (id TEXT PRIMARY KEY NOT NULL, name TEXT)`,
			original:    `INSERT INTO records VALUES ('old', '原记录')`,
			replacement: `INSERT INTO records VALUES ('new', '新记录')`,
		},
		{
			name:        "可空文本主键",
			definition:  `CREATE TABLE records (id TEXT PRIMARY KEY, name TEXT)`,
			original:    `INSERT INTO records VALUES ('old', '原记录')`,
			replacement: `INSERT INTO records VALUES ('new', '新记录')`,
		},
		{
			name:        "复用整数主键",
			definition:  `CREATE TABLE records (id INTEGER PRIMARY KEY, name TEXT)`,
			original:    `INSERT INTO records VALUES (1, '原记录')`,
			replacement: `INSERT INTO records VALUES (1, '新记录')`,
		},
		{
			name:        "无主键复用内部行号",
			definition:  `CREATE TABLE records (name TEXT)`,
			original:    `INSERT INTO records VALUES ('原记录')`,
			replacement: `INSERT INTO records VALUES ('新记录')`,
		},
	}
	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			service := openStorageDB(test, testCase.definition, testCase.original)
			original := queryStorageDB(test, service, DBQuery{Table: "records"}).Rows[0]
			for _, statement := range []string{`DELETE FROM records`, testCase.replacement} {
				if _, err := service.database.Exec(statement); err != nil {
					test.Fatal(err)
				}
			}
			update := DBWriteRequest{Table: "records", DBRowReference: original.DBRowReference, Values: map[string]DBValue{"name": {Type: "text", Value: "误修改"}}}
			if err := service.Write(context.Background(), update, false); err == nil {
				test.Fatal("旧行标识不应修改替换后的记录")
			}
			if err := service.Delete(context.Background(), DBDeleteRequest{Table: "records", Rows: []DBRowReference{original.DBRowReference}}); err == nil {
				test.Fatal("旧行标识不应删除替换后的记录")
			}
			remaining := queryStorageDB(test, service, DBQuery{Table: "records"})
			if remaining.Total != 1 || remaining.Rows[0].Values["name"].Value != "新记录" {
				test.Fatalf("新记录被错误修改或删除: %+v", remaining.Rows)
			}
		})
	}
}

func TestStorageDBIdentitySelection(test *testing.T) {
	testCases := []struct {
		name       string
		definition string
		insert     string
		identity   []string
	}{
		{"整数主键", `CREATE TABLE records (id INTEGER PRIMARY KEY, name TEXT)`, `INSERT INTO records VALUES (1, '甲')`, []string{"id"}},
		{"非空文本主键", `CREATE TABLE records (id TEXT PRIMARY KEY NOT NULL, name TEXT)`, `INSERT INTO records VALUES ('a', '甲')`, []string{"id"}},
		{"非空复合主键", `CREATE TABLE records (tenant TEXT NOT NULL, id TEXT NOT NULL, name TEXT, PRIMARY KEY (tenant, id))`, `INSERT INTO records VALUES ('a', 'b', '甲')`, []string{"tenant", "id"}},
		{"可空主键重复空值", `CREATE TABLE records (id TEXT PRIMARY KEY, name TEXT)`, `INSERT INTO records VALUES (NULL, '甲'), (NULL, '乙')`, []string{"_rowid_"}},
		{"降序整数主键可为空", `CREATE TABLE records (id INTEGER PRIMARY KEY DESC, name TEXT)`, `INSERT INTO records VALUES (NULL, '甲'), (NULL, '乙')`, []string{"_rowid_"}},
	}
	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			service := openStorageDB(test, testCase.definition, testCase.insert)
			result := queryStorageDB(test, service, DBQuery{Table: "records"})
			if !reflect.DeepEqual(result.Table.IdentityColumns, testCase.identity) {
				test.Fatalf("行标识选择错误: %v", result.Table.IdentityColumns)
			}
			if err := service.Delete(context.Background(), DBDeleteRequest{Table: "records", Rows: []DBRowReference{result.Rows[0].DBRowReference}}); err != nil {
				test.Fatal(err)
			}
			if remaining := queryStorageDB(test, service, DBQuery{Table: "records"}); remaining.Total != result.Total-1 {
				test.Fatalf("未准确删除一行: %+v", remaining)
			}
		})
	}
}

func TestStorageDBChangedSnapshotRollsBackBatch(test *testing.T) {
	service := openStorageDB(test,
		`CREATE TABLE records (id INTEGER PRIMARY KEY, name TEXT COLLATE NOCASE)`,
		`INSERT INTO records VALUES (1, 'Alice'), (2, 'Bob')`,
	)
	original := queryStorageDB(test, service, DBQuery{Table: "records"})
	if _, err := service.database.Exec(`UPDATE records SET name = 'bob' WHERE id = 2`); err != nil {
		test.Fatal(err)
	}
	request := DBDeleteRequest{Table: "records", Rows: []DBRowReference{original.Rows[0].DBRowReference, original.Rows[1].DBRowReference}}
	if err := service.Delete(context.Background(), request); err == nil {
		test.Fatal("内容变化后应拒绝旧删除请求，包括大小写变化")
	}
	current := queryStorageDB(test, service, DBQuery{Table: "records"})
	if current.Total != 2 || current.Rows[0].Values["name"].Value != "Alice" || current.Rows[1].Values["name"].Value != "bob" {
		test.Fatalf("事务未完整回滚: %+v", current.Rows)
	}
	request.Rows = []DBRowReference{current.Rows[0].DBRowReference, current.Rows[1].DBRowReference}
	if err := service.Delete(context.Background(), request); err != nil {
		test.Fatalf("刷新后的版本应能提交: %v", err)
	}
}

func TestStorageDBRequiresSnapshotAndAllowsPrimaryKeyEdit(test *testing.T) {
	service := openStorageDB(test, `CREATE TABLE records (id INTEGER PRIMARY KEY, name TEXT)`, `INSERT INTO records VALUES (1, '甲')`)
	original := queryStorageDB(test, service, DBQuery{Table: "records"}).Rows[0]
	missingVersion := DBRowReference{Identity: original.Identity}
	if err := service.Delete(context.Background(), DBDeleteRequest{Table: "records", Rows: []DBRowReference{missingVersion}}); err == nil {
		test.Fatal("缺少版本时不能删除")
	}
	update := DBWriteRequest{Table: "records", DBRowReference: missingVersion, Values: map[string]DBValue{"id": {Type: "integer", Value: "2"}}}
	if err := service.Write(context.Background(), update, false); err == nil {
		test.Fatal("缺少版本时不能修改")
	}
	update.DBRowReference = original.DBRowReference
	if err := service.Write(context.Background(), update, false); err != nil {
		test.Fatal(err)
	}
	current := queryStorageDB(test, service, DBQuery{Table: "records"}).Rows[0]
	if current.Identity["id"].Value != "2" || current.Version == original.Version {
		test.Fatalf("主键修改后应更新行标识与版本: %+v", current)
	}
}
