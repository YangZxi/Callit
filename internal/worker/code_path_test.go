package worker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCodePath(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantPath    string
		wantDir     bool
		wantInvalid bool
	}{
		{name: "根文件", input: "main.js", wantPath: "main.js"},
		{name: "移除开头斜杠", input: "/test/123.js", wantPath: "test/123.js"},
		{name: "移除多个开头斜杠", input: "///test/123.js", wantPath: "test/123.js"},
		{name: "移除每段两端空格", input: " / test / 123.js ", wantPath: "test/123.js"},
		{name: "规范化目录", input: " / test / ", wantPath: "test", wantDir: true},
		{name: "保留名称内部空格", input: "test/hello world.js", wantPath: "test/hello world.js"},
		{name: "三级目录中的文件", input: "src/lib/helper.js", wantPath: "src/lib/helper.js"},
		{name: "三级目录", input: "src/lib/", wantPath: "src/lib", wantDir: true},
		{name: "超过最大层级", input: "src/lib/deep/helper.js", wantInvalid: true},
		{name: "父目录", input: "../main.js", wantInvalid: true},
		{name: "空路径段", input: "src//helper.js", wantInvalid: true},
		{name: "控制字符", input: "src/hello\x00.js", wantInvalid: true},
		{name: "名称过长", input: "src/" + strings.Repeat("a", 256), wantInvalid: true},
		{name: "反斜杠", input: `src\helper.js`, wantInvalid: true},
		{name: "Windows 绝对路径", input: "C:/tmp/helper.js", wantInvalid: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseCodePath(test.input, 3)
			if test.wantInvalid {
				if !errors.Is(err, ErrInvalidCodePath) {
					t.Fatalf("应返回 ErrInvalidCodePath，got=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCodePath 失败: %v", err)
			}
			if got.RelativePath != test.wantPath || got.IsDirectory != test.wantDir {
				t.Fatalf("解析结果不正确: got=%#v", got)
			}
		})
	}
}

func TestEnsureCodePathHasNoSymlinkRejectsSymlinkComponent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("写入外部文件失败: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatalf("创建软链接失败: %v", err)
	}

	err := EnsureCodePathHasNoSymlink(root, CodePath{RelativePath: "linked/secret.txt"}, false)
	if !errors.Is(err, ErrInvalidCodePath) {
		t.Fatalf("软链接路径应被拒绝，got=%v", err)
	}
}
