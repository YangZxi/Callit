package chat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildDiffRelativePath(t *testing.T) {
	if got := buildDiffRelativePath("main.py"); got != "main.diff.py" {
		t.Fatalf("unexpected diff path: %s", got)
	}
	if got := buildDiffRelativePath("dir/main.js"); got != "dir/main.diff.js" {
		t.Fatalf("unexpected diff path: %s", got)
	}
	if got := buildDiffRelativePath("a"); got != "a.diff" {
		t.Fatalf("unexpected diff path: %s", got)
	}
}

func TestApplyAgentOutput(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "main.py")
	if err := os.WriteFile(existing, []byte("print('old')"), 0o644); err != nil {
		t.Fatalf("write existing failed: %v", err)
	}

	output := agentOutput{
		Files: []agentOutputFile{
			{Path: "new.py", Op: "create", Content: "print('new')"},
			{Path: "main.py", Op: "update", Content: "print('diff')"},
		},
	}

	results, err := applyAgentOutput(dir, output)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	raw, err := os.ReadFile(filepath.Join(dir, "new.py"))
	if err != nil || string(raw) != "print('new')" {
		t.Fatalf("unexpected new file content: %s, err=%v", string(raw), err)
	}

	diffRaw, err := os.ReadFile(filepath.Join(dir, "main.diff.py"))
	if err != nil || string(diffRaw) != "print('diff')" {
		t.Fatalf("unexpected diff file content: %s, err=%v", string(diffRaw), err)
	}
}

func TestParseAgentOutputFromMarkdown(t *testing.T) {
	raw := "好的，我来处理。\n\n```json\n{\n  \"files\": [\n    {\n      \"path\": \"main.py\",\n      \"op\": \"update\",\n      \"content\": \"print('ok')\"\n    }\n  ]\n}\n```\n"
	output, err := parseAgentOutput(raw)
	if err != nil {
		t.Fatalf("parseAgentOutput should parse markdown json block, err=%v", err)
	}
	if len(output.Files) != 1 {
		t.Fatalf("unexpected files len: %d", len(output.Files))
	}
	if output.Files[0].Path != "main.py" || output.Files[0].Op != "update" {
		t.Fatalf("unexpected parsed file: %#v", output.Files[0])
	}
}
