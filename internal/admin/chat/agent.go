package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

type agentOutput struct {
	Files []agentOutputFile `json:"files"`
}

type agentOutputFile struct {
	Path    string `json:"path"`
	Op      string `json:"op"`
	Content string `json:"content"`
}

type agentApplyResult struct {
	Path     string `json:"path"`
	Op       string `json:"op"`
	Written  string `json:"written"`
	Conflict bool   `json:"conflict,omitempty"`
}

var agentJSONBlockPattern = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")

func parseAgentOutput(raw string) (agentOutput, error) {
	candidates := make([]string, 0, 4)
	clean := strings.TrimSpace(raw)
	if clean != "" {
		candidates = append(candidates, clean)
	}
	matches := agentJSONBlockPattern.FindAllStringSubmatch(raw, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		block := strings.TrimSpace(match[1])
		if block != "" {
			candidates = append(candidates, block)
		}
	}
	if balanced := findFirstBalancedJSONObject(raw); balanced != "" {
		candidates = append(candidates, strings.TrimSpace(balanced))
	}

	var lastErr error
	for _, candidate := range candidates {
		clean := strings.TrimSpace(candidate)
		if strings.HasPrefix(clean, "```") {
			clean = strings.TrimPrefix(clean, "```json")
			clean = strings.TrimPrefix(clean, "```")
			clean = strings.TrimSuffix(clean, "```")
			clean = strings.TrimSpace(clean)
		}

		var output agentOutput
		err := json.Unmarshal([]byte(clean), &output)
		if err == nil {
			if len(output.Files) == 0 {
				err = fmt.Errorf("files 不能为空")
			} else {
				for i, file := range output.Files {
					file.Path = strings.TrimSpace(file.Path)
					file.Op = strings.TrimSpace(strings.ToLower(file.Op))
					if file.Path == "" {
						err = fmt.Errorf("files[%d].path 不能为空", i)
						break
					}
					if file.Op != "create" && file.Op != "update" {
						err = fmt.Errorf("files[%d].op 必须是 create 或 update", i)
						break
					}
					output.Files[i] = file
				}
			}
		}
		if err == nil {
			return output, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return agentOutput{}, fmt.Errorf("Agent 输出不是合法 JSON: %w", lastErr)
	}
	return agentOutput{}, fmt.Errorf("Agent 输出不是合法 JSON")
}

func findFirstBalancedJSONObject(raw string) string {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '{' {
			continue
		}
		depth := 0
		inString := false
		escaped := false
		for j := i; j < len(raw); j++ {
			ch := raw[j]
			if inString {
				if escaped {
					escaped = false
					continue
				}
				if ch == '\\' {
					escaped = true
					continue
				}
				if ch == '"' {
					inString = false
				}
				continue
			}

			if ch == '"' {
				inString = true
				continue
			}
			if ch == '{' {
				depth++
				continue
			}
			if ch == '}' {
				depth--
				if depth == 0 {
					return raw[i : j+1]
				}
			}
		}
	}
	return ""
}

func buildDiffRelativePath(relativePath string) string {
	dir := path.Dir(relativePath)
	base := path.Base(relativePath)
	ext := path.Ext(base)
	name := strings.TrimSuffix(base, ext)

	diffBase := name + ".diff"
	if ext != "" {
		diffBase += ext
	}
	if dir == "." {
		return diffBase
	}
	return path.Join(dir, diffBase)
}

func applyAgentOutput(workerDir string, output agentOutput) ([]agentApplyResult, error) {
	results := make([]agentApplyResult, 0, len(output.Files))

	for _, file := range output.Files {
		targetAbs, targetRel, err := resolveWorkerPath(workerDir, file.Path)
		if err != nil {
			return nil, fmt.Errorf("非法文件路径 %s", file.Path)
		}

		if file.Op == "create" {
			if _, err := os.Stat(targetAbs); err == nil {
				return nil, fmt.Errorf("create 目标已存在: %s", targetRel)
			}
			if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(targetAbs, []byte(file.Content), 0o644); err != nil {
				return nil, err
			}
			results = append(results, agentApplyResult{
				Path:    targetRel,
				Op:      file.Op,
				Written: targetRel,
			})
			continue
		}

		if _, err := os.Stat(targetAbs); err != nil {
			return nil, fmt.Errorf("update 目标文件不存在: %s", targetRel)
		}
		diffRel := buildDiffRelativePath(targetRel)
		diffAbs, diffNormalized, err := resolveWorkerPath(workerDir, diffRel)
		if err != nil {
			return nil, fmt.Errorf("非法 diff 文件路径 %s", diffRel)
		}
		if err := os.MkdirAll(filepath.Dir(diffAbs), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(diffAbs, []byte(file.Content), 0o644); err != nil {
			return nil, err
		}
		results = append(results, agentApplyResult{
			Path:    targetRel,
			Op:      file.Op,
			Written: diffNormalized,
		})
	}

	return results, nil
}
