package admin

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"callit/internal/admin/storage"
	"callit/internal/config"
	magicDB "callit/internal/magic-api/db"
	"callit/internal/router"
)

func TestAdminStorageDBAPI(t *testing.T) {
	store := openAdminTestStore(t)
	workerDB, err := magicDB.OpenSQLiteService(filepath.Join(t.TempDir(), "worker.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workerDB.Close() })
	if _, err := workerDB.SQLDB().Exec(`CREATE TABLE records (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AdminPrefix: "/admin", AdminToken: "test-token", DataDir: t.TempDir()}
	engine := NewEngineWithOptions(store, router.New(), nil, &cfg, StorageStore{WorkerDB: workerDB.SQLDB()})
	for _, path := range []string{"/db/tables", "/db/query", "/db/rows/create", "/db/rows/update", "/db/rows/delete"} {
		method := http.MethodPost
		if path == "/db/tables" {
			method = http.MethodGet
		}
		response := doAdminJSONRequestWithoutAuth(t, engine, method, "/admin/api"+path, nil)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("接口必须认证: %s, %d", path, response.Code)
		}
	}
	response := doAdminJSONRequest(t, engine, http.MethodGet, "/admin/api/db/tables", nil)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	create := storage.DBWriteRequest{Table: "records", Values: map[string]storage.DBValue{"name": {Type: "text", Value: "新增"}}}
	response = doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/db/rows/create", create)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	response = doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/db/query", storage.DBQuery{Table: "records"})
	var payload struct {
		Data storage.DBQueryResult `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.Data.Total != 1 {
		t.Fatalf("查询响应异常: %s, %v", response.Body.String(), err)
	}
	update := storage.DBWriteRequest{Table: "records", DBRowReference: payload.Data.Rows[0].DBRowReference, Values: map[string]storage.DBValue{"name": {Type: "text", Value: "修改"}}}
	response = doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/db/rows/update", update)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	var name string
	if err := workerDB.SQLDB().QueryRow(`SELECT name FROM records`).Scan(&name); err != nil || name != "修改" {
		t.Fatalf("应直接修改 Worker 数据库: %s, %v", name, err)
	}
	response = doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/db/rows/delete", storage.DBDeleteRequest{Table: "records", Rows: []storage.DBRowReference{update.DBRowReference}})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("修改后的旧版本不应继续用于删除: %s", response.Body.String())
	}
	response = doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/db/query", storage.DBQuery{Table: "records"})
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.Data.Total != 1 {
		t.Fatalf("旧版本删除不应改变记录: %s, %v", response.Body.String(), err)
	}
	response = doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/db/rows/delete", storage.DBDeleteRequest{Table: "records", Rows: []storage.DBRowReference{payload.Data.Rows[0].DBRowReference}})
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	response = doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/db/query", storage.DBQuery{Table: "records", Where: "id ="})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("语法错误应返回 400: %s", response.Body.String())
	}
}
