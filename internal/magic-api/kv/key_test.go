package kv

import "testing"

func TestBuildRedisKey(t *testing.T) {
	got, err := BuildRedisKey("worker-1", "session")
	if err != nil {
		t.Fatalf("BuildRedisKey 返回错误: %v", err)
	}

	want := "callit:kv:worker-1:session"
	if got != want {
		t.Fatalf("Redis key 不正确，got=%q want=%q", got, want)
	}
}
