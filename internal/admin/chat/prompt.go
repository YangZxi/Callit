package chat

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var promptRefPattern = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)

func parsePromptReferences(prompt string) []string {
	matches := promptRefPattern.FindAllStringSubmatch(prompt, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		rel := strings.TrimSpace(match[1])
		if rel == "" {
			continue
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		paths = append(paths, rel)
	}
	return paths
}

func resolveWorkerPath(workerDir string, rawPath string) (absolutePath string, relativePath string, err error) {
	normalized := strings.TrimSpace(rawPath)
	normalized = strings.ReplaceAll(normalized, "\\", "/")
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimLeft(normalized, "/")
	if normalized == "" {
		return "", "", fs.ErrNotExist
	}

	cleanPath := path.Clean(normalized)
	if cleanPath == "." || cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
		return "", "", fs.ErrNotExist
	}

	workerAbs, err := filepath.Abs(workerDir)
	if err != nil {
		return "", "", err
	}
	targetAbs, err := filepath.Abs(filepath.Join(workerAbs, filepath.FromSlash(cleanPath)))
	if err != nil {
		return "", "", err
	}

	if targetAbs != workerAbs && !strings.HasPrefix(targetAbs, workerAbs+string(os.PathSeparator)) {
		return "", "", fs.ErrNotExist
	}
	rel, err := filepath.Rel(workerAbs, targetAbs)
	if err != nil {
		return "", "", err
	}
	return targetAbs, filepath.ToSlash(rel), nil
}

func buildReferencedFilesPrefix(workerDir string, prompt string) string {
	refs := parsePromptReferences(prompt)
	if len(refs) == 0 {
		return ""
	}

	blocks := make([]string, 0, len(refs))
	for _, refPath := range refs {
		absPath, relPath, err := resolveWorkerPath(workerDir, refPath)
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(absPath)
		if err != nil || !utf8.Valid(raw) {
			continue
		}
		blocks = append(blocks, fmt.Sprintf("[引用文件] %s\n```text\n%s\n```", relPath, string(raw)))
	}
	if len(blocks) == 0 {
		return ""
	}
	return "以下是本次消息引用的本地文件，请优先参考这些文件：\n\n" + strings.Join(blocks, "\n\n")
}

func buildWorkerSnapshot(workerDir string, maxChars int) (string, []string, error) {
	paths := make([]string, 0, 16)
	if err := filepath.WalkDir(workerDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		paths = append(paths, p)
		return nil
	}); err != nil {
		return "", nil, err
	}

	sort.Strings(paths)
	blocks := make([]string, 0, len(paths))
	omitted := make([]string, 0)
	totalChars := 0

	for _, absPath := range paths {
		raw, err := os.ReadFile(absPath)
		if err != nil || !utf8.Valid(raw) {
			continue
		}

		rel, err := filepath.Rel(workerDir, absPath)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)

		block := fmt.Sprintf("[项目文件] %s\n```text\n%s\n```", rel, string(raw))
		if maxChars > 0 && totalChars+len(block) > maxChars {
			omitted = append(omitted, rel)
			continue
		}
		blocks = append(blocks, block)
		totalChars += len(block)
	}

	if len(blocks) == 0 {
		return "", omitted, nil
	}
	return "当前 Worker 已有代码如下，请基于这些文件继续改造：\n\n" + strings.Join(blocks, "\n\n"), omitted, nil
}

func buildAgentSystemPrompt(runtime string) (string, error) {
	templateFile := templateFilenameByRuntime(runtime)
	if templateFile == "" {
		return "", fmt.Errorf("runtime 不合法: %s", runtime)
	}

	templatePath := filepath.Join("resources/worker_templates", templateFile)
	templateRaw, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("读取模板失败: %w", err)
	}

	guideRaw, err := os.ReadFile(filepath.Join("resources", "code-agent.md"))
	if err != nil {
		return "", fmt.Errorf("读取 code-agent 引导文档失败: %w", err)
	}

	return fmt.Sprintf(
		"%s\n\n当前 Worker runtime: %s\n你必须严格遵循下面模板风格与入口契约：\n```text\n%s\n```\n\n最终输出必须是 JSON，格式如下，禁止输出任何额外解释：\n{\"files\":[{\"path\":\"文件路径\",\"op\":\"create|update\",\"content\":\"完整代码\"}]}",
		strings.TrimSpace(string(guideRaw)),
		runtime,
		string(templateRaw),
	), nil
}

func templateFilenameByRuntime(runtime string) string {
	switch runtime {
	case "python":
		return "python.py"
	case "node":
		return "node.js"
	default:
		return ""
	}
}
