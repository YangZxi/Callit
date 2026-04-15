package executor

import (
	"strings"
	"testing"
)

func TestExtractWorkerStdoutReturnsWorkerLogBlock(t *testing.T) {
	raw := strings.Join([]string{
		"[I] nsjail start",
		"===============",
		"worker line 1",
		"worker line 2",
		"**=====^=====**",
		`{"status":200,"body":"ok"}`,
		"**=====^=====**",
		"===============",
		"[I] nsjail end",
	}, "\n")

	logOutput := extractWorkerStdout(raw)
	want := "worker line 1\nworker line 2"
	if logOutput != want {
		t.Fatalf("worker stdout 提取不正确，got=%q want=%q", logOutput, want)
	}
}

func TestExtractWorkerStdoutReturnsEmptyWhenWorkerLogBlockIsEmpty(t *testing.T) {
	raw := strings.Join([]string{
		"[I] nsjail start",
		"===============",
		"**=====^=====**",
		`{"status":200,"body":"ok"}`,
		"**=====^=====**",
		"===============",
		"[I] nsjail end",
	}, "\n")

	logOutput := extractWorkerStdout(raw)
	if logOutput != "" {
		t.Fatalf("worker stdout 应为空，got=%q", logOutput)
	}
}

func TestExtractWorkerStdoutFallsBackToTrimmedRawWhenLogBlockMissing(t *testing.T) {
	raw := "[I] nsjail only"

	logOutput := extractWorkerStdout(raw)
	if logOutput != raw {
		t.Fatalf("缺少分隔块时应回退原始内容，got=%q want=%q", logOutput, raw)
	}
}
