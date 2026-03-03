package admin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/gin-gonic/gin"
)

const dependencyTaskBusyMessage = "有其他安装或移除依赖请求在执行，请稍后再试。"

type dependencyInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type dependencyManageRequest struct {
	Runtime string `json:"runtime"`
	Action  string `json:"action"`
	Package string `json:"package"`
}

type dependencyLogEvent struct {
	Stream string `json:"stream"`
	Text   string `json:"text"`
}

type pnpmListItem struct {
	Dependencies map[string]struct {
		Version string `json:"version"`
	} `json:"dependencies"`
}

type pipListItem struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (s *Server) listDependencies(c *gin.Context) {
	runtime, err := normalizeDependencyRuntime(c.Query("runtime"))
	if err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}

	var result []dependencyInfo
	switch runtime {
	case "node":
		result, err = s.listNodeDependencies(c.Request.Context())
	case "python":
		result, err = s.listPythonDependencies(c.Request.Context())
	default:
		err = fmt.Errorf("runtime 仅支持 node 或 python")
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}

	apiSuccess(c, result)
}

func (s *Server) manageDependencies(c *gin.Context) {
	var req dependencyManageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "请求体格式错误")
		return
	}

	runtime, err := normalizeDependencyRuntime(req.Runtime)
	if err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}
	action, err := normalizeDependencyAction(req.Action)
	if err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}
	packageName, err := normalizeDependencyPackage(req.Package)
	if err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}

	if !s.tryAcquireDependencyTask() {
		apiError(c, http.StatusConflict, dependencyTaskBusyMessage)
		return
	}
	defer s.releaseDependencyTask()

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	cmd, err := s.buildDependencyManageCommand(c.Request.Context(), runtime, action, packageName)
	if err != nil {
		_ = writeSSEEvent(c, "error", gin.H{"message": err.Error()})
		_ = writeSSEEvent(c, "done", gin.H{"ok": false})
		return
	}

	_ = writeSSEEvent(c, "log", dependencyLogEvent{
		Stream: "stdout",
		Text:   "$ " + strings.Join(cmd.Args, " "),
	})

	if err := streamCommandLogs(c, cmd); err != nil {
		_ = writeSSEEvent(c, "error", gin.H{"message": err.Error()})
		_ = writeSSEEvent(c, "done", gin.H{"ok": false})
		return
	}
	_ = writeSSEEvent(c, "done", gin.H{"ok": true})
}

func (s *Server) tryAcquireDependencyTask() bool {
	s.dependencyTaskMu.Lock()
	defer s.dependencyTaskMu.Unlock()
	if s.dependencyTaskRunning {
		return false
	}
	s.dependencyTaskRunning = true
	return true
}

func (s *Server) releaseDependencyTask() {
	s.dependencyTaskMu.Lock()
	s.dependencyTaskRunning = false
	s.dependencyTaskMu.Unlock()
}

func normalizeDependencyRuntime(raw string) (string, error) {
	runtime := strings.ToLower(strings.TrimSpace(raw))
	if runtime != "node" && runtime != "python" {
		return "", fmt.Errorf("runtime 仅支持 node 或 python")
	}
	return runtime, nil
}

func normalizeDependencyAction(raw string) (string, error) {
	action := strings.ToLower(strings.TrimSpace(raw))
	if action != "install" && action != "remove" {
		return "", fmt.Errorf("action 仅支持 install 或 remove")
	}
	return action, nil
}

func normalizeDependencyPackage(raw string) (string, error) {
	pkg := strings.TrimSpace(raw)
	if pkg == "" {
		return "", fmt.Errorf("package 不能为空")
	}
	if strings.HasPrefix(pkg, "-") {
		return "", fmt.Errorf("package 非法")
	}
	if strings.IndexFunc(pkg, unicode.IsSpace) >= 0 {
		return "", fmt.Errorf("package 不能包含空白字符")
	}
	return pkg, nil
}

func (s *Server) listNodeDependencies(ctx context.Context) ([]dependencyInfo, error) {
	nodeDir, err := s.ensureNodeDependencyDir()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "pnpm", "list", "--depth", "0", "--json")
	cmd.Dir = nodeDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("读取 Node 依赖失败: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return parseNodeDependencies(output)
}

func (s *Server) listPythonDependencies(ctx context.Context) ([]dependencyInfo, error) {
	pipPath, _, err := s.ensurePythonDependencyEnv(ctx)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, pipPath, "list", "--format=json")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("读取 Python 依赖失败: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return parsePythonDependencies(output)
}

func parseNodeDependencies(raw []byte) ([]dependencyInfo, error) {
	var list []pnpmListItem
	if err := json.Unmarshal(raw, &list); err != nil {
		var one pnpmListItem
		if err2 := json.Unmarshal(raw, &one); err2 != nil {
			return nil, fmt.Errorf("解析 Node 依赖失败: %w", err)
		}
		list = []pnpmListItem{one}
	}

	result := make([]dependencyInfo, 0)
	if len(list) == 0 {
		return result, nil
	}

	seen := make(map[string]bool)
	for name, item := range list[0].Dependencies {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		result = append(result, dependencyInfo{
			Name:    trimmed,
			Version: strings.TrimSpace(item.Version),
		})
	}
	sortDependencyList(result)
	return result, nil
}

func parsePythonDependencies(raw []byte) ([]dependencyInfo, error) {
	var list []pipListItem
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("解析 Python 依赖失败: %w", err)
	}

	result := make([]dependencyInfo, 0, len(list))
	for _, item := range list {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		// venv 默认自带包不展示，避免干扰用户识别已安装依赖。
		switch strings.ToLower(name) {
		case "pip", "setuptools", "wheel":
			continue
		}
		result = append(result, dependencyInfo{
			Name:    name,
			Version: strings.TrimSpace(item.Version),
		})
	}
	sortDependencyList(result)
	return result, nil
}

func sortDependencyList(list []dependencyInfo) {
	sort.Slice(list, func(i, j int) bool {
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
}

func (s *Server) buildDependencyManageCommand(ctx context.Context, runtime string, action string, packageName string) (*exec.Cmd, error) {
	switch runtime {
	case "node":
		nodeDir, err := s.ensureNodeDependencyDir()
		if err != nil {
			return nil, err
		}
		args := []string{}
		if action == "install" {
			args = []string{"add", packageName}
		} else {
			args = []string{"remove", packageName}
		}
		cmd := exec.CommandContext(ctx, "pnpm", args...)
		cmd.Dir = nodeDir
		return cmd, nil
	case "python":
		pipPath, pythonDir, err := s.ensurePythonDependencyEnv(ctx)
		if err != nil {
			return nil, err
		}
		args := []string{}
		if action == "install" {
			args = []string{"install", packageName}
		} else {
			args = []string{"uninstall", "-y", packageName}
		}
		cmd := exec.CommandContext(ctx, pipPath, args...)
		cmd.Dir = pythonDir
		return cmd, nil
	default:
		return nil, fmt.Errorf("runtime 仅支持 node 或 python")
	}
}

func (s *Server) ensureNodeDependencyDir() (string, error) {
	nodeDir, err := filepath.Abs(filepath.Join(s.dataDir, ".lib", "node"))
	if err != nil {
		return "", fmt.Errorf("解析 Node 依赖目录失败: %w", err)
	}
	if err := os.MkdirAll(nodeDir, 0o755); err != nil {
		return "", fmt.Errorf("创建 Node 依赖目录失败: %w", err)
	}

	packagePath := filepath.Join(nodeDir, "package.json")
	if _, err := os.Stat(packagePath); err == nil {
		return nodeDir, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("检查 Node 依赖目录失败: %w", err)
	}

	content := []byte("{\n  \"name\": \"callit-runtime-deps\",\n  \"private\": true,\n  \"version\": \"1.0.0\"\n}\n")
	if err := os.WriteFile(packagePath, content, 0o644); err != nil {
		return "", fmt.Errorf("创建 package.json 失败: %w", err)
	}
	return nodeDir, nil
}

func (s *Server) ensurePythonDependencyEnv(ctx context.Context) (pipPath string, pythonDir string, err error) {
	pythonDir, err = filepath.Abs(filepath.Join(s.dataDir, ".lib", "python"))
	if err != nil {
		return "", "", fmt.Errorf("解析 Python 依赖目录失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(pythonDir), 0o755); err != nil {
		return "", "", fmt.Errorf("创建 Python 依赖目录失败: %w", err)
	}

	pipPath = resolveVenvPipPath(pythonDir)
	if pipPath != "" {
		return pipPath, pythonDir, nil
	}

	cmd := exec.CommandContext(ctx, "python3", "-m", "venv", pythonDir)
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return "", "", fmt.Errorf("初始化 Python 虚拟环境失败: %w: %s", runErr, strings.TrimSpace(string(output)))
	}
	pipPath = resolveVenvPipPath(pythonDir)
	if pipPath == "" {
		return "", "", fmt.Errorf("初始化 Python 虚拟环境失败: pip 不存在")
	}
	return pipPath, pythonDir, nil
}

func resolveVenvPipPath(pythonDir string) string {
	candidates := []string{
		filepath.Join(pythonDir, "bin", "pip"),
		filepath.Join(pythonDir, "bin", "pip3"),
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func streamCommandLogs(c *gin.Context, cmd *exec.Cmd) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	type logLine struct {
		stream string
		text   string
	}

	lineCh := make(chan logLine, 64)
	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	readPipe := func(stream string, reader io.ReadCloser) {
		defer wg.Done()
		defer reader.Close()

		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		for scanner.Scan() {
			lineCh <- logLine{
				stream: stream,
				text:   scanner.Text(),
			}
		}
		if scanErr := scanner.Err(); scanErr != nil {
			errCh <- scanErr
		}
	}

	wg.Add(2)
	go readPipe("stdout", stdout)
	go readPipe("stderr", stderr)
	go func() {
		wg.Wait()
		close(lineCh)
	}()

	var writeErr error
	for line := range lineCh {
		if writeErr != nil {
			continue
		}
		if err := writeSSEEvent(c, "log", dependencyLogEvent{
			Stream: line.stream,
			Text:   line.text,
		}); err != nil {
			writeErr = err
		}
	}
	close(errCh)

	var readErr error
	for err := range errCh {
		if readErr == nil {
			readErr = err
		}
	}

	waitErr := cmd.Wait()
	if writeErr != nil {
		return writeErr
	}
	if readErr != nil {
		return readErr
	}
	if waitErr != nil {
		return waitErr
	}
	return nil
}
