package chat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePromptReferences(t *testing.T) {
	prompt := "请看 [main.py](main.py) 和 [img](image/a.jpg) 以及 [dup](main.py)"
	refs := parsePromptReferences(prompt)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[0] != "main.py" || refs[1] != "image/a.jpg" {
		t.Fatalf("unexpected refs: %#v", refs)
	}
}

func TestResolveWorkerPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "image"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	abs, rel, err := resolveWorkerPath(dir, "image/a.jpg")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if !strings.HasSuffix(abs, "/image/a.jpg") && !strings.HasSuffix(abs, "\\image\\a.jpg") {
		t.Fatalf("unexpected abs path: %s", abs)
	}
	if rel != "image/a.jpg" {
		t.Fatalf("unexpected rel path: %s", rel)
	}
	if _, _, err := resolveWorkerPath(dir, "../etc/passwd"); err == nil {
		t.Fatalf("expected path traversal error")
	}
}

func TestBuildReferencedFilesPrefix(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "main.py")
	if err := os.WriteFile(target, []byte("print('ok')"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	prefix := buildReferencedFilesPrefix(dir, "请看 [main.py](main.py) 和 [missing](missing.py)")
	if !strings.Contains(prefix, "[引用文件] main.py") {
		t.Fatalf("missing expected referenced header: %s", prefix)
	}
	if strings.Contains(prefix, "missing.py") {
		t.Fatalf("missing file should be ignored")
	}
}
