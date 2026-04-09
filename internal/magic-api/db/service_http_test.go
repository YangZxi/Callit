package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestServiceExecSupportsQuestionPlaceholderArguments(t *testing.T) {
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

	if _, err := service.Exec(context.Background(), "create table users (id integer primary key autoincrement, name text, age integer)", nil); err != nil {
		t.Fatalf("创建测试表失败: %v", err)
	}
	if _, err := service.Exec(context.Background(), "insert into users(name, age) values(?, ?)", []any{"alice", 18}); err != nil {
		t.Fatalf("写入第一条数据失败: %v", err)
	}
	if _, err := service.Exec(context.Background(), "insert into users(name, age) values(?, ?)", []any{"bob", 22}); err != nil {
		t.Fatalf("写入第二条数据失败: %v", err)
	}

	result, err := service.Exec(context.Background(), "select name from users where age > ?", []any{20})
	if err != nil {
		t.Fatalf("执行 select 失败: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("select 返回行数不正确，got=%d", len(result.Rows))
	}
	if got := result.Rows[0]["name"]; got != "bob" {
		t.Fatalf("select 返回 name 不正确，got=%v", got)
	}
}
