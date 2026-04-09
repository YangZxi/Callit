package magicapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	magicDB "callit/internal/magic-api/db"
)

func TestNewHandlerRoutesDBExecPath(t *testing.T) {
	service, err := magicDB.OpenSQLiteService(filepath.Join(t.TempDir(), "worker.db"))
	if err != nil {
		t.Fatalf("打开 worker.db 失败: %v", err)
	}
	defer func() {
		if closeErr := service.Close(); closeErr != nil {
			t.Fatalf("关闭 worker.db 失败: %v", closeErr)
		}
	}()
	if _, err := service.Exec(t.Context(), "create table users (id integer primary key autoincrement, name text)", nil); err != nil {
		t.Fatalf("创建测试表失败: %v", err)
	}
	if _, err := service.Exec(t.Context(), "insert into users(name) values(?)", []any{"alice"}); err != nil {
		t.Fatalf("写入测试数据失败: %v", err)
	}

	handler := NewHandler(Options{
		DBService: service,
	})

	req := httptest.NewRequest(http.MethodPost, "/db/exec", bytes.NewBufferString(`{"sql":"select name from users where id = ?","args":[1]}`))
	req.Header.Set("X-Callit-Worker-Id", "worker-1")
	req.Header.Set("X-Callit-Request-Id", "req-1")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("db exec 请求应返回 200，实际为 %d，body=%s", resp.Code, resp.Body.String())
	}
	if got := resp.Body.String(); got != "{\"rows\":[{\"name\":\"alice\"}],\"rows_affected\":0,\"last_insert_id\":0}" {
		t.Fatalf("db exec 返回值不正确，got=%q", got)
	}
}

func TestNewHandlerRejectsDBExecWhenServiceUnavailable(t *testing.T) {
	handler := NewHandler(Options{})

	req := httptest.NewRequest(http.MethodPost, "/db/exec", bytes.NewBufferString(`{"sql":"select 1"}`))
	req.Header.Set("X-Callit-Worker-Id", "worker-1")
	req.Header.Set("X-Callit-Request-Id", "req-1")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("未配置 DBService 时应返回 500，实际为 %d，body=%s", resp.Code, resp.Body.String())
	}
}
