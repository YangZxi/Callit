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

	"callit/internal/admin/message"

	"github.com/gin-gonic/gin"
)

const dependencyTaskBusyMessage = "有其他安装或移除依赖请求在执行，请稍后再试。"

const fixedPythonCommand = "python3.10"

var execCommandContext = exec.CommandContext

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

	var result []message.DependencyInfo
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
	var req message.DependencyManageRequest
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

	_ = writeSSEEvent(c, "log", message.DependencyLogEvent{
		Stream: "stdout",
		Text:   "$ " + strings.Join(cmd.Args, " "),
	})

	if err := streamCommandLogs(c, cmd); err != nil {
		_ = writeSSEEvent(c, "error", gin.H{"message": err.Error()})
		_ = writeSSEEvent(c, "done", gin.H{"ok": false})
		return
	}
	if runtime == "python" {
		if err := s.writeToPythonRequirements(c.Request.Context()); err != nil {
			_ = writeSSEEvent(c, "error", gin.H{"message": err.Error()})
			_ = writeSSEEvent(c, "done", gin.H{"ok": false})
			return
		}
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

func (s *Server) listNodeDependencies(ctx context.Context) ([]message.DependencyInfo, error) {
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

func (s *Server) listPythonDependencies(ctx context.Context) ([]message.DependencyInfo, error) {
	pythonPath, _, err := s.ensurePythonDependencyEnv(ctx)
	if err != nil {
		return nil, err
	}
	cmd := execCommandContext(ctx, pythonPath, buildPythonPipArgs("list", "--format=json")...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("读取 Python 依赖失败: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return parsePythonDependencies(output)
}

func (s *Server) rebuildDependencies(c *gin.Context) {
	runtime, err := normalizeDependencyRuntime(c.Query("runtime"))
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

	if runtime == "node" {
		// none
	} else if runtime == "python" {
		pythonDir, err := s.pythonDependencyVersionDir()
		if err != nil {
			_ = writeSSEEvent(c, "error", gin.H{"message": fmt.Sprintf("解析 Python 依赖目录失败: %v", err)})
			_ = writeSSEEvent(c, "done", gin.H{"ok": false})
			return
		}

		if err := streamPythonDependencyRebuild(c, c.Request.Context(), pythonDir); err != nil {
			_ = writeSSEEvent(c, "error", gin.H{"message": err.Error()})
			_ = writeSSEEvent(c, "done", gin.H{"ok": false})
			return
		}
	}
	_ = writeSSEEvent(c, "done", gin.H{"ok": true})
}

func parseNodeDependencies(raw []byte) ([]message.DependencyInfo, error) {
	var list []pnpmListItem
	if err := json.Unmarshal(raw, &list); err != nil {
		var one pnpmListItem
		if err2 := json.Unmarshal(raw, &one); err2 != nil {
			return nil, fmt.Errorf("解析 Node 依赖失败: %w", err)
		}
		list = []pnpmListItem{one}
	}

	result := make([]message.DependencyInfo, 0)
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
		result = append(result, message.DependencyInfo{
			Name:    trimmed,
			Version: strings.TrimSpace(item.Version),
		})
	}
	sortDependencyList(result)
	return result, nil
}

func parsePythonDependencies(raw []byte) ([]message.DependencyInfo, error) {
	var list []pipListItem
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("解析 Python 依赖失败: %w", err)
	}

	result := make([]message.DependencyInfo, 0, len(list))
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
		result = append(result, message.DependencyInfo{
			Name:    name,
			Version: strings.TrimSpace(item.Version),
		})
	}
	sortDependencyList(result)
	return result, nil
}

func sortDependencyList(list []message.DependencyInfo) {
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
		pythonPath, pythonDir, err := s.ensurePythonDependencyEnv(ctx)
		if err != nil {
			return nil, err
		}
		args := []string{}
		if action == "install" {
			args = buildPythonPipArgs("install", packageName)
		} else {
			args = buildPythonPipArgs("uninstall", "-y", packageName)
		}
		cmd := execCommandContext(ctx, pythonPath, args...)
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

func (s *Server) ensurePythonDependencyEnv(ctx context.Context) (pythonPath string, pythonDir string, err error) {
	pythonDir, err = s.pythonDependencyVersionDir()
	if err != nil {
		return "", "", fmt.Errorf("解析 Python 依赖目录失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(pythonDir), 0o755); err != nil {
		return "", "", fmt.Errorf("创建 Python 依赖目录失败: %w", err)
	}

	if err := validatePythonDependencyEnv(ctx, pythonDir); err != nil {
		return "", "", fmt.Errorf("%s，请进行环境重建", "Python 运行环境已损坏")
	}
	pythonPath, _ = resolvePythonVenvPaths(pythonDir)
	return pythonPath, pythonDir, nil
}

func (s *Server) pythonDependencyVersionDir() (string, error) {
	return filepath.Abs(filepath.Join(s.dataDir, ".lib", "python", "venv"))
}

func resolvePythonVenvPaths(pythonDir string) (pythonPath string, requirementsPath string) {
	pythonRoot := filepath.Clean(strings.TrimSpace(pythonDir))
	return filepath.Join(pythonRoot, "bin", "python"), filepath.Join(filepath.Dir(pythonRoot), "requirements.txt")
}

func buildPythonPipArgs(args ...string) []string {
	result := make([]string, 0, len(args)+2)
	result = append(result, "-m", "pip")
	result = append(result, args...)
	return result
}

func validatePythonDependencyEnv(ctx context.Context, pythonDir string) error {
	pythonPath, _ := resolvePythonVenvPaths(pythonDir)
	info, err := os.Stat(pythonPath)
	if err != nil {
		return fmt.Errorf("Python 虚拟环境不可用: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("Python 虚拟环境不可用: %s 不是文件", pythonPath)
	}

	cmd := execCommandContext(ctx, pythonPath, buildPythonPipArgs("--version")...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Python 虚拟环境不可用: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if detectPythonSitePackages(pythonDir) == "" {
		return fmt.Errorf("Python 虚拟环境不可用: site-packages 缺失")
	}
	return nil
}

func rebuildPythonDependencyEnv(ctx context.Context, pythonDir string) error {
	_, requirementsPath := resolvePythonVenvPaths(pythonDir)
	requirementsContent, readErr := os.ReadFile(requirementsPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("读取 requirements.txt 失败: %w", readErr)
	}

	if err := os.RemoveAll(pythonDir); err != nil {
		return fmt.Errorf("删除损坏的 Python 虚拟环境失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(pythonDir), 0o755); err != nil {
		return fmt.Errorf("创建 Python 依赖目录失败: %w", err)
	}

	cmd := execCommandContext(ctx, fixedPythonCommand, "-m", "venv", pythonDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("初始化 Python 虚拟环境失败: %w: %s", err, strings.TrimSpace(string(output)))
	}

	if err := os.WriteFile(requirementsPath, requirementsContent, 0o644); err != nil {
		return fmt.Errorf("写入 requirements.txt 失败: %w", err)
	}
	if strings.TrimSpace(string(requirementsContent)) == "" {
		return nil
	}

	pythonPath, _ := resolvePythonVenvPaths(pythonDir)
	restoreCmd := execCommandContext(ctx, pythonPath, buildPythonPipArgs("install", "-r", requirementsPath)...)
	restoreOutput, err := restoreCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("按 requirements.txt 恢复 Python 依赖失败: %w: %s", err, strings.TrimSpace(string(restoreOutput)))
	}
	return nil
}

func streamPythonDependencyRebuild(c *gin.Context, ctx context.Context, pythonDir string) error {
	pythonPath, requirementsPath := resolvePythonVenvPaths(pythonDir)

	_ = writeSSEEvent(c, "log", message.DependencyLogEvent{
		Stream: "stdout",
		Text:   "开始重建 Python 虚拟环境",
	})
	if err := rebuildPythonDependencyEnv(ctx, pythonDir); err != nil {
		return err
	}

	_ = writeSSEEvent(c, "log", message.DependencyLogEvent{
		Stream: "stdout",
		Text:   "$ " + strings.Join(append([]string{fixedPythonCommand}, "-m", "venv", pythonDir), " "),
	})
	_ = writeSSEEvent(c, "log", message.DependencyLogEvent{
		Stream: "stdout",
		Text:   "Python 虚拟环境重建完成",
	})

	if content, err := os.ReadFile(requirementsPath); err == nil && strings.TrimSpace(string(content)) != "" {
		_ = writeSSEEvent(c, "log", message.DependencyLogEvent{
			Stream: "stdout",
			Text:   "$ " + strings.Join(append([]string{pythonPath}, buildPythonPipArgs("install", "-r", requirementsPath)...), " "),
		})
		_ = writeSSEEvent(c, "log", message.DependencyLogEvent{
			Stream: "stdout",
			Text:   "已按 requirements.txt 恢复 Python 依赖",
		})
	}
	return nil
}

func (s *Server) writeToPythonRequirements(ctx context.Context) error {
	pythonPath, pythonDir, err := s.ensurePythonDependencyEnv(ctx)
	if err != nil {
		return err
	}
	_, requirementsPath := resolvePythonVenvPaths(pythonDir)
	cmd := execCommandContext(ctx, pythonPath, buildPythonPipArgs("freeze")...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("刷新 requirements.txt 失败: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.WriteFile(requirementsPath, normalizeRequirementsOutput(output), 0o644); err != nil {
		return fmt.Errorf("写入 requirements.txt 失败: %w", err)
	}
	return nil
}

func normalizeRequirementsOutput(output []byte) []byte {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return []byte{}
	}
	return []byte(trimmed + "\n")
}

func detectPythonSitePackages(pythonEnvDir string) string {
	libDir := filepath.Join(pythonEnvDir, "lib")
	entries, err := os.ReadDir(libDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(entry.Name()))
		if !strings.HasPrefix(name, "python") {
			continue
		}
		sitePackages := filepath.Join(libDir, entry.Name(), "site-packages")
		stat, err := os.Stat(sitePackages)
		if err == nil && stat.IsDir() {
			return sitePackages
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
		if err := writeSSEEvent(c, "log", message.DependencyLogEvent{
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
