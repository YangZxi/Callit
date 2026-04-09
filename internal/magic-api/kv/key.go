package kv

import (
	"fmt"
	"strings"
	"unicode"
)

const redisKeyPrefix = "callit:kv"

// BuildRedisKey 构造 KV 的真实 Redis key。
func BuildRedisKey(namespace string, userKey string) (string, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return "", fmt.Errorf("namespace 不能为空")
	}

	normalizedKey, err := normalizeUserKey(userKey)
	if err != nil {
		return "", err
	}
	return redisKeyPrefix + ":" + namespace + ":" + normalizedKey, nil
}

func normalizeUserKey(userKey string) (string, error) {
	userKey = strings.TrimSpace(userKey)
	if userKey == "" {
		return "", fmt.Errorf("key 不能为空")
	}
	if len(userKey) > 128 {
		return "", fmt.Errorf("key 长度不能超过 128")
	}
	for _, r := range userKey {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("key 不能包含控制字符")
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case ':', '_', '-', '.', '/':
			continue
		default:
			return "", fmt.Errorf("key 包含非法字符: %q", r)
		}
	}
	return userKey, nil
}
