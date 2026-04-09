package kv

import (
	"context"
	"testing"
	"time"
)

type fakeStore struct {
	data       map[string][]byte
	ttl        map[string]time.Duration
	ttlValue   map[string]int64
	deleteKeys []string
	hasKeys    map[string]bool
	increment  int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		data:     map[string][]byte{},
		ttl:      map[string]time.Duration{},
		ttlValue: map[string]int64{},
		hasKeys:  map[string]bool{},
	}
}

func (s *fakeStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	value, ok := s.data[key]
	return value, ok, nil
}

func (s *fakeStore) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	s.data[key] = value
	s.ttl[key] = ttl
	return nil
}

func (s *fakeStore) Delete(_ context.Context, key string) error {
	s.deleteKeys = append(s.deleteKeys, key)
	delete(s.data, key)
	return nil
}

func (s *fakeStore) Exists(_ context.Context, key string) (bool, error) {
	if value, ok := s.hasKeys[key]; ok {
		return value, nil
	}
	_, ok := s.data[key]
	return ok, nil
}

func (s *fakeStore) Increment(_ context.Context, key string, step int64) (int64, error) {
	s.increment += step
	return s.increment, nil
}

func (s *fakeStore) Expire(_ context.Context, key string, ttl time.Duration) error {
	s.ttl[key] = ttl
	return nil
}

func (s *fakeStore) TTL(_ context.Context, key string) (int64, error) {
	if value, ok := s.ttlValue[key]; ok {
		return value, nil
	}
	return -2, nil
}

func TestServiceSetAndGetUseWorkerNamespace(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)

	if err := svc.Set(context.Background(), "worker-1", "session", `{"id":1}`, 3*time.Second); err != nil {
		t.Fatalf("Set 返回错误: %v", err)
	}

	if got := string(store.data["callit:kv:worker-1:session"]); got != `{"id":1}` {
		t.Fatalf("写入 Redis 的值不正确，got=%q", got)
	}

	value, ok, err := svc.Get(context.Background(), "worker-1", "session")
	if err != nil {
		t.Fatalf("Get 返回错误: %v", err)
	}
	if !ok {
		t.Fatalf("应读取到已写入的值")
	}
	if value != `{"id":1}` {
		t.Fatalf("读取值不正确，got=%#v", value)
	}
}

func TestServiceSetRejectsNonStringValue(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)

	if err := svc.Set(context.Background(), "worker-1", "session", 123, 3*time.Second); err == nil {
		t.Fatalf("非 string value 应返回错误")
	}
}

func TestServiceIncrementUsesWorkerNamespace(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)

	got, err := svc.Increment(context.Background(), "worker-1", "counter", 2)
	if err != nil {
		t.Fatalf("Increment 返回错误: %v", err)
	}
	if got != 2 {
		t.Fatalf("Increment 返回值不正确，got=%d", got)
	}
}

func TestServiceUsesExplicitNamespaceWhenProvided(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)

	if err := svc.Set(context.Background(), "group1", "session", "value", 3*time.Second); err != nil {
		t.Fatalf("Set 返回错误: %v", err)
	}

	if got := string(store.data["callit:kv:group1:session"]); got != "value" {
		t.Fatalf("显式 namespace 写入 Redis 的值不正确，got=%q", got)
	}
}

func TestServiceRejectsEmptyNamespace(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)

	if err := svc.Set(context.Background(), "", "session", "value", 3*time.Second); err == nil {
		t.Fatalf("空 namespace 应返回错误")
	}
}
