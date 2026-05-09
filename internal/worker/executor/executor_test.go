package executor

import (
	"testing"
)

func TestParseWorkerEnvPairsKeepsSemicolonValueIntact(t *testing.T) {
	parsed := parseWorkerEnvPairs(
		"API_KEY=test\r\nQQ_MUSIC_COOKIE=uin=o1282381264;wxuin=;euin=oK-FowoFoK-s7n**\nREGION=us",
	)

	if len(parsed) != 3 {
		t.Fatalf("解析结果数量不正确: %#v", parsed)
	}
	if parsed["API_KEY"] != "test" {
		t.Fatalf("API_KEY 解析失败: %#v", parsed)
	}
	if parsed["QQ_MUSIC_COOKIE"] != "uin=o1282381264;wxuin=;euin=oK-FowoFoK-s7n**" {
		t.Fatalf("QQ_MUSIC_COOKIE 解析失败: %#v", parsed)
	}
	if parsed["REGION"] != "us" {
		t.Fatalf("REGION 解析失败: %#v", parsed)
	}
}
