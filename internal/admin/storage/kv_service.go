package storage

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

const (
	kvScanLimit        int64 = 500
	redisKeyPrefix           = "callit:kv:"
	redisScanBatchSize       = 100
)

type KVStore interface {
	ScanKeys(ctx context.Context, pattern string, limit int64) ([]string, error)
	GetWithTTL(ctx context.Context, key string) (string, int64, bool, error)
	DeleteRawKey(ctx context.Context, key string) error
	Stats(ctx context.Context) (KVStats, error)
}

type KVStats struct {
	ConnectedClients int64  `json:"connected_clients"`
	UsedMemory       int64  `json:"used_memory"`
	UsedMemoryHuman  string `json:"used_memory_human"`
	KVKeys           int64  `json:"kv_keys"`
}

type KVService struct {
	store KVStore
}

type KVKeyItem struct {
	Key       string `json:"key"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type KVKeyDetail struct {
	Key       string `json:"key"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	TTL       int64  `json:"ttl"`
	Value     string `json:"value"`
	Exists    bool   `json:"exists"`
}

type RedisKVStore struct {
	client redis.UniversalClient
}

func NewKVService(store KVStore) *KVService {
	if store == nil {
		return nil
	}
	return &KVService{store: store}
}

func NewRedisKVStore(client redis.UniversalClient) *RedisKVStore {
	return &RedisKVStore{
		client: client,
	}
}

func isKVRedisKey(key string) bool {
	return strings.HasPrefix(strings.TrimSpace(key), redisKeyPrefix)
}

func parseKVRedisKey(key string) (string, string, error) {
	key = strings.TrimSpace(key)
	if !isKVRedisKey(key) {
		return "", "", fmt.Errorf("key 不属于 Callit KV")
	}
	rest := strings.TrimPrefix(key, redisKeyPrefix)
	separatorIndex := strings.LastIndex(rest, ":")
	if separatorIndex <= 0 || separatorIndex >= len(rest)-1 {
		return "", "", fmt.Errorf("key 格式不正确")
	}
	namespace := strings.TrimSpace(rest[:separatorIndex])
	name := strings.TrimSpace(rest[separatorIndex+1:])
	if namespace == "" || name == "" {
		return "", "", fmt.Errorf("key 格式不正确")
	}
	return namespace, name, nil
}

func kvRedisKeyPattern(keyword string) string {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return redisKeyPrefix + "*"
	}
	return redisKeyPrefix + "*" + keyword + "*"
}

func (s *KVService) Metrics(ctx context.Context) (KVStats, error) {
	return s.store.Stats(ctx)
}

func (s *KVService) ListKeys(ctx context.Context, keyword string) ([]KVKeyItem, error) {
	keys, err := s.store.ScanKeys(ctx, kvRedisKeyPattern(keyword), kvScanLimit)
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)

	items := make([]KVKeyItem, 0, len(keys))
	for _, key := range keys {
		namespace, name, err := parseKVRedisKey(key)
		if err != nil {
			continue
		}
		items = append(items, KVKeyItem{
			Key:       key,
			Namespace: namespace,
			Name:      name,
		})
	}
	return items, nil
}

func (s *KVService) Detail(ctx context.Context, key string) (KVKeyDetail, error) {
	key = strings.TrimSpace(key)
	namespace, name, err := parseKVRedisKey(key)
	if err != nil {
		return KVKeyDetail{}, err
	}

	value, ttl, exists, err := s.store.GetWithTTL(ctx, key)
	if err != nil {
		return KVKeyDetail{}, err
	}
	return KVKeyDetail{
		Key:       key,
		Namespace: namespace,
		Name:      name,
		TTL:       ttl,
		Value:     value,
		Exists:    exists,
	}, nil
}

func (s *KVService) Delete(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if !isKVRedisKey(key) {
		return fmt.Errorf("只能删除 Callit KV key")
	}
	if _, _, err := parseKVRedisKey(key); err != nil {
		return err
	}
	return s.store.DeleteRawKey(ctx, key)
}

func (s *RedisKVStore) ScanKeys(ctx context.Context, pattern string, limit int64) ([]string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		pattern = kvRedisKeyPattern("")
	}
	if limit <= 0 {
		limit = kvScanLimit
	}

	keys := make([]string, 0)
	var cursor uint64
	for {
		batch, nextCursor, err := s.client.Scan(ctx, cursor, pattern, redisScanBatchSize).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, batch...)
		if int64(len(keys)) >= limit {
			return keys[:limit], nil
		}
		if nextCursor == 0 {
			return keys, nil
		}
		cursor = nextCursor
	}
}

func (s *RedisKVStore) GetWithTTL(ctx context.Context, key string) (string, int64, bool, error) {
	value, err := s.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", -2, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}

	ttl, err := s.client.TTL(ctx, key).Result()
	if err != nil {
		return "", 0, false, err
	}
	return value, int64(ttl.Seconds()), true, nil
}

func (s *RedisKVStore) DeleteRawKey(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

func (s *RedisKVStore) Stats(ctx context.Context) (KVStats, error) {
	stats := KVStats{}

	kvKeys, err := s.countKeys(ctx, kvRedisKeyPattern(""))
	if err != nil {
		return stats, err
	}
	stats.KVKeys = kvKeys

	info, err := s.client.Info(ctx, "clients", "memory").Result()
	if err != nil {
		return stats, err
	}
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch key {
		case "connected_clients":
			stats.ConnectedClients, _ = strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		case "used_memory":
			stats.UsedMemory, _ = strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		case "used_memory_human":
			stats.UsedMemoryHuman = strings.TrimSpace(value)
		}
	}
	return stats, nil
}

func (s *RedisKVStore) countKeys(ctx context.Context, pattern string) (int64, error) {
	var total int64
	var cursor uint64
	for {
		keys, nextCursor, err := s.client.Scan(ctx, cursor, pattern, redisScanBatchSize).Result()
		if err != nil {
			return 0, err
		}
		total += int64(len(keys))
		if nextCursor == 0 {
			return total, nil
		}
		cursor = nextCursor
	}
}
