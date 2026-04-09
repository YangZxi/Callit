package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestServiceExecSelectReturnsRows(t *testing.T) {
	baseDir := t.TempDir()
	service, err := OpenSQLiteService(filepath.Join(baseDir, "worker.db"))
	if err != nil {
		t.Fatalf("打开 worker.db 失败: %v", err)
	}
	defer func() {
		if closeErr := service.Close(); closeErr != nil {
			t.Fatalf("关闭 worker.db 失败: %v", closeErr)
		}
	}()

	if _, err := service.Exec(context.Background(), "create table users (id integer primary key autoincrement, name text, status integer)", nil); err != nil {
		t.Fatalf("创建测试表失败: %v", err)
	}
	if _, err := service.Exec(context.Background(), "insert into users(name, status) values(?, ?)", []any{"alice", 1}); err != nil {
		t.Fatalf("写入测试数据失败: %v", err)
	}

	result, err := service.Exec(context.Background(), "select id, name, status from users where status = ?", []any{1})
	if err != nil {
		t.Fatalf("执行 select 失败: %v", err)
	}

	if len(result.Rows) != 1 {
		t.Fatalf("select 返回行数不正确，got=%d", len(result.Rows))
	}
	if got := result.Rows[0]["name"]; got != "alice" {
		t.Fatalf("select 返回 name 不正确，got=%v", got)
	}
	if result.RowsAffected != 0 {
		t.Fatalf("select 不应返回 rows_affected，got=%d", result.RowsAffected)
	}
}

func TestServiceExecInsertReturnsAffectedAndLastInsertID(t *testing.T) {
	baseDir := t.TempDir()
	service, err := OpenSQLiteService(filepath.Join(baseDir, "worker.db"))
	if err != nil {
		t.Fatalf("打开 worker.db 失败: %v", err)
	}
	defer func() {
		if closeErr := service.Close(); closeErr != nil {
			t.Fatalf("关闭 worker.db 失败: %v", closeErr)
		}
	}()

	if _, err := service.Exec(context.Background(), "create table users (id integer primary key autoincrement, name text)", nil); err != nil {
		t.Fatalf("创建测试表失败: %v", err)
	}

	result, err := service.Exec(context.Background(), "insert into users(name) values(?)", []any{"bob"})
	if err != nil {
		t.Fatalf("执行 insert 失败: %v", err)
	}

	if result.RowsAffected != 1 {
		t.Fatalf("insert 返回 rows_affected 不正确，got=%d", result.RowsAffected)
	}
	if result.LastInsertID <= 0 {
		t.Fatalf("insert 应返回 last_insert_id，got=%d", result.LastInsertID)
	}
	if len(result.Rows) != 0 {
		t.Fatalf("insert 不应返回 rows，got=%d", len(result.Rows))
	}
}

func TestOpenSQLiteServiceDisablesForeignKeys(t *testing.T) {
	baseDir := t.TempDir()
	service, err := OpenSQLiteService(filepath.Join(baseDir, "worker.db"))
	if err != nil {
		t.Fatalf("打开 worker.db 失败: %v", err)
	}
	defer func() {
		if closeErr := service.Close(); closeErr != nil {
			t.Fatalf("关闭 worker.db 失败: %v", closeErr)
		}
	}()

	result, err := service.Exec(context.Background(), "pragma foreign_keys", nil)
	if err != nil {
		t.Fatalf("读取 foreign_keys 失败: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("pragma foreign_keys 返回行数不正确，got=%d", len(result.Rows))
	}
	if got := result.Rows[0]["foreign_keys"]; got != int64(0) {
		t.Fatalf("foreign_keys 应关闭，got=%v", got)
	}
}
