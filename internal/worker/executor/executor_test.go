package executor

import (
	"testing"

	"callit/internal/model"
)

func TestBuildSandboxWorkerEnvKeepsSemicolonValueIntact(t *testing.T) {
	parsed := buildSandboxWorkerEnv(
		model.WorkerEnv{"API_KEY=test", "QQ_MUSIC_COOKIE=uin=o1282381264;wxuin=;euin=oK-FowoFoK-s7n**", "REGION=us"},
	)

	if len(parsed) != 3 {
		t.Fatalf("解析结果数量不正确: %#v", parsed)
	}
	if parsed[0] != "API_KEY=test" {
		t.Fatalf("API_KEY 解析失败: %#v", parsed)
	}
	if parsed[1] != "QQ_MUSIC_COOKIE=uin=o1282381264;wxuin=;euin=oK-FowoFoK-s7n**" {
		t.Fatalf("QQ_MUSIC_COOKIE 解析失败: %#v", parsed)
	}
	if parsed[2] != "REGION=us" {
		t.Fatalf("REGION 解析失败: %#v", parsed)
	}
}
