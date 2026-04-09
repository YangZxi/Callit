package magicapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	magicKV "callit/internal/magic-api/kv"
)

func TestNewHandlerRejectsRequestWithoutWorkerContext(t *testing.T) {
	handler := NewHandler(Options{
		KVService: magicKV.NewService(newMagicTestStore()),
	})

	req := httptest.NewRequest(http.MethodPost, "/kv/get", bytes.NewBufferString(`{"namespace":"worker-1","key":"session"}`))
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("未授权请求应返回 401，实际为 %d", resp.Code)
	}
}

func TestNewHandlerRoutesKVPath(t *testing.T) {
	store := newMagicTestStore()
	if err := store.Set(context.Background(), "callit:kv:worker-1:session", []byte("value"), 0); err != nil {
		t.Fatalf("准备测试数据失败: %v", err)
	}
	handler := NewHandler(Options{
		KVService: magicKV.NewService(store),
	})

	req := httptest.NewRequest(http.MethodPost, "/kv/get", bytes.NewBufferString(`{"namespace":"worker-1","key":"session"}`))
	req.Header.Set("X-Callit-Worker-Id", "worker-1")
	req.Header.Set("X-Callit-Request-Id", "req-1")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("KV 请求应返回 200，实际为 %d，body=%s", resp.Code, resp.Body.String())
	}
	if got := resp.Body.String(); got != "{\"value\":\"value\"}" {
		t.Fatalf("返回值不正确，got=%q", got)
	}
}

func TestNewHandlerSetUsesSecondsField(t *testing.T) {
	store := newMagicTestStore()
	handler := NewHandler(Options{
		KVService: magicKV.NewService(store),
	})

	req := httptest.NewRequest(http.MethodPost, "/kv/set", bytes.NewBufferString(`{"namespace":"worker-1","key":"session","value":"value","seconds":3}`))
	req.Header.Set("X-Callit-Worker-Id", "worker-1")
	req.Header.Set("X-Callit-Request-Id", "req-1")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("set 请求应返回 200，实际为 %d，body=%s", resp.Code, resp.Body.String())
	}
	if got := store.ttl["callit:kv:worker-1:session"]; got != 3*time.Second {
		t.Fatalf("set 应使用秒作为过期时间，got=%s", got)
	}
}

func TestNewHandlerSetUsesExplicitNamespaceWhenProvided(t *testing.T) {
	store := newMagicTestStore()
	handler := NewHandler(Options{
		KVService: magicKV.NewService(store),
	})

	req := httptest.NewRequest(http.MethodPost, "/kv/set", bytes.NewBufferString(`{"namespace":"group1","key":"session","value":"value","seconds":3}`))
	req.Header.Set("X-Callit-Worker-Id", "worker-1")
	req.Header.Set("X-Callit-Request-Id", "req-1")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("set 请求应返回 200，实际为 %d，body=%s", resp.Code, resp.Body.String())
	}
	if got := store.ttl["callit:kv:group1:session"]; got != 3*time.Second {
		t.Fatalf("显式 namespace 应使用 group1，got=%s", got)
	}
}

func TestNewHandlerRejectsRequestWithoutNamespace(t *testing.T) {
	store := newMagicTestStore()
	handler := NewHandler(Options{
		KVService: magicKV.NewService(store),
	})

	req := httptest.NewRequest(http.MethodPost, "/kv/get", bytes.NewBufferString(`{"key":"session"}`))
	req.Header.Set("X-Callit-Worker-Id", "worker-1")
	req.Header.Set("X-Callit-Request-Id", "req-1")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("缺少 namespace 应返回 400，实际为 %d，body=%s", resp.Code, resp.Body.String())
	}
}

func TestNewHandlerTTLReturnsSecondsField(t *testing.T) {
	store := newMagicTestStore()
	store.ttlValue = 12
	handler := NewHandler(Options{
		KVService: magicKV.NewService(store),
	})

	req := httptest.NewRequest(http.MethodPost, "/kv/ttl", bytes.NewBufferString(`{"namespace":"worker-1","key":"session"}`))
	req.Header.Set("X-Callit-Worker-Id", "worker-1")
	req.Header.Set("X-Callit-Request-Id", "req-1")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("ttl 请求应返回 200，实际为 %d，body=%s", resp.Code, resp.Body.String())
	}
	if got := resp.Body.String(); got != "{\"seconds\":12}" {
		t.Fatalf("ttl 返回值不正确，got=%q", got)
	}
}

type magicTestStore struct {
	data     map[string][]byte
	ttl      map[string]time.Duration
	ttlValue int64
}

func newMagicTestStore() *magicTestStore {
	return &magicTestStore{
		data: map[string][]byte{},
		ttl:  map[string]time.Duration{},
	}
}

func (s *magicTestStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	value, ok := s.data[key]
	return value, ok, nil
}

func (s *magicTestStore) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	s.data[key] = value
	s.ttl[key] = ttl
	return nil
}

func (s *magicTestStore) Delete(_ context.Context, key string) error {
	delete(s.data, key)
	return nil
}

func (s *magicTestStore) Exists(_ context.Context, key string) (bool, error) {
	_, ok := s.data[key]
	return ok, nil
}

func (s *magicTestStore) Increment(_ context.Context, key string, step int64) (int64, error) {
	return step, nil
}

func (s *magicTestStore) Expire(_ context.Context, key string, ttl time.Duration) error {
	s.ttl[key] = ttl
	return nil
}

func (s *magicTestStore) TTL(_ context.Context, key string) (int64, error) {
	if s.ttlValue != 0 {
		return s.ttlValue, nil
	}
	return -1, nil
}
