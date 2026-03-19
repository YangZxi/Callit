package snowflake

import "testing"

func TestGeneratorNextID(t *testing.T) {
	gen := NewGenerator(1)

	first, err := gen.NextID()
	if err != nil {
		t.Fatalf("生成首个雪花 ID 失败: %v", err)
	}
	second, err := gen.NextID()
	if err != nil {
		t.Fatalf("生成第二个雪花 ID 失败: %v", err)
	}

	if first <= 0 || second <= 0 {
		t.Fatalf("雪花 ID 应为正整数: first=%d second=%d", first, second)
	}
	if second <= first {
		t.Fatalf("雪花 ID 应递增: first=%d second=%d", first, second)
	}
}
