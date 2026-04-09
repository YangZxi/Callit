package kv

import (
	"context"
	"fmt"
	"time"
)

type Store interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Increment(ctx context.Context, key string, step int64) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	TTL(ctx context.Context, key string) (int64, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Set(ctx context.Context, namespace string, userKey string, value any, ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("seconds 必须大于 0")
	}
	key, err := BuildRedisKey(namespace, userKey)
	if err != nil {
		return err
	}
	text, ok := value.(string)
	if !ok {
		return fmt.Errorf("value 必须是 string")
	}
	return s.store.Set(ctx, key, []byte(text), ttl)
}

func (s *Service) Get(ctx context.Context, namespace string, userKey string) (string, bool, error) {
	key, err := BuildRedisKey(namespace, userKey)
	if err != nil {
		return "", false, err
	}
	raw, ok, err := s.store.Get(ctx, key)
	if err != nil || !ok {
		return "", ok, err
	}
	return string(raw), true, nil
}

func (s *Service) Delete(ctx context.Context, namespace string, userKey string) error {
	key, err := BuildRedisKey(namespace, userKey)
	if err != nil {
		return err
	}
	return s.store.Delete(ctx, key)
}

func (s *Service) Has(ctx context.Context, namespace string, userKey string) (bool, error) {
	key, err := BuildRedisKey(namespace, userKey)
	if err != nil {
		return false, err
	}
	return s.store.Exists(ctx, key)
}

func (s *Service) Increment(ctx context.Context, namespace string, userKey string, step int64) (int64, error) {
	key, err := BuildRedisKey(namespace, userKey)
	if err != nil {
		return 0, err
	}
	if step == 0 {
		step = 1
	}
	return s.store.Increment(ctx, key, step)
}

func (s *Service) Expire(ctx context.Context, namespace string, userKey string, ttl time.Duration) error {
	key, err := BuildRedisKey(namespace, userKey)
	if err != nil {
		return err
	}
	if ttl <= 0 {
		return fmt.Errorf("seconds 必须大于 0")
	}
	return s.store.Expire(ctx, key, ttl)
}

func (s *Service) TTL(ctx context.Context, namespace string, userKey string) (int64, error) {
	key, err := BuildRedisKey(namespace, userKey)
	if err != nil {
		return 0, err
	}
	return s.store.TTL(ctx, key)
}
