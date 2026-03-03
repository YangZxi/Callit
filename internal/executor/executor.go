package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"callit/internal/model"
)

// ExecuteResult 表示脚本执行结果。
type ExecuteResult struct {
	Status     int
	Headers    map[string]string
	File       string
	Body       any
	Stdout     string
	Stderr     string
	DurationMS int64
	TimedOut   bool
	Err        error
}

// Run 执行函数脚本。
func Run(parent context.Context, fn model.Worker, workerDir string, input model.WorkerInput) (result ExecuteResult) {
	started := time.Now()
	result = ExecuteResult{Headers: map[string]string{}}
	defer func() {
		result.DurationMS = time.Since(started).Milliseconds()
	}()

	mainFile := mainFilenameByRuntime(fn.Runtime)
	if mainFile == "" {
		result.Err = fmt.Errorf("不支持的 runtime: %s", fn.Runtime)
		return
	}
	if _, err := os.Stat(filepath.Join(workerDir, mainFile)); err != nil {
		result.Err = fmt.Errorf("主文件不存在: %s", mainFile)
		return
	}

	payload, err := json.Marshal(input)
	if err != nil {
		result.Err = fmt.Errorf("序列化脚本上下文失败: %w", err)
		return
	}

	bridgeStdout, bridgeStderr, runErr := runWithHandlerBridge(parent, fn, workerDir, payload)
	result.Stdout = bridgeStdout
	result.Stderr = bridgeStderr

	if errors.Is(parent.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		result.Err = context.DeadlineExceeded
		return
	}
	if runErr != nil {
		fmt.Printf("脚本执行失败: %s, %v", bridgeStderr, runErr)
		result.Err = fmt.Errorf("脚本执行失败: %w", runErr)
		return
	}

	scriptOut, err := parseScriptOutput([]byte(result.Stdout))
	if err != nil {
		result.Err = fmt.Errorf("脚本 stdout 不是合法 JSON: %w", err)
		return
	}

	status := 200
	if scriptOut.Status != nil {
		status = *scriptOut.Status
	}
	if status < 100 || status > 599 {
		result.Err = fmt.Errorf("脚本返回非法 status: %d", status)
		return
	}
	result.Status = status
	if scriptOut.Headers != nil {
		result.Headers = scriptOut.Headers
	}
	result.File = scriptOut.File
	result.Body = scriptOut.Body
	return
}

func mainFilenameByRuntime(runtime string) string {
	switch runtime {
	case "python":
		return "main.py"
	case "node":
		return "main.js"
	default:
		return ""
	}
}

func parseScriptOutput(raw []byte) (model.WorkerOutput, error) {
	var scriptOut model.WorkerOutput
	if err := json.Unmarshal(raw, &scriptOut); err != nil {
		return model.WorkerOutput{}, err
	}
	return scriptOut, nil
}

func runWithHandlerBridge(parent context.Context, fn model.Worker, functionDir string, payload []byte) (stdout string, stderr string, err error) {
	bridgeCmd, err := buildBridgeCommand(parent, fn, functionDir)
	if err != nil {
		return "", "", err
	}
	bridgeCmd.Dir = functionDir
	bridgeCmd.Stdin = bytes.NewReader(payload)

	var out, errOut bytes.Buffer
	bridgeCmd.Stdout = &out
	bridgeCmd.Stderr = &errOut

	if err := bridgeCmd.Run(); err != nil {
		return out.String(), errOut.String(), err
	}
	return out.String(), errOut.String(), nil
}

func buildBridgeCommand(parent context.Context, fn model.Worker, functionDir string) (*exec.Cmd, error) {
	filename := ""
	switch fn.Runtime {
	case "python":
		filename = "python.py"
	case "node":
		filename = "node.js"
	default:
		return nil, fmt.Errorf("不支持的 runtime: %s", fn.Runtime)
	}
	path := filepath.Join("resources/worker_entrypoints", filename)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取入口脚本失败: %w", err)
	}
	code := string(content)

	switch fn.Runtime {
	case "python":
		cmd := exec.CommandContext(parent, "python3", "-c", code)
		cmd.Env = buildRuntimeEnv(functionDir, fn.Runtime)
		return cmd, nil
	case "node":
		cmd := exec.CommandContext(parent, "node", "-e", code)
		cmd.Env = buildRuntimeEnv(functionDir, fn.Runtime)
		return cmd, nil
	default:
		return nil, fmt.Errorf("不支持的 runtime: %s", fn.Runtime)
	}
}

func buildRuntimeEnv(functionDir string, runtime string) []string {
	env := os.Environ()

	workerAbs, err := filepath.Abs(functionDir)
	if err != nil {
		return env
	}
	workersDir := filepath.Dir(workerAbs)
	dataDir := filepath.Dir(workersDir)
	libDir := filepath.Join(dataDir, ".lib")

	switch runtime {
	case "node":
		nodePath := filepath.Join(libDir, "node", "node_modules")
		env = appendOrMergeEnvPath(env, "NODE_PATH", nodePath)
	case "python":
		sitePackages := detectPythonSitePackages(filepath.Join(libDir, "python"))
		if sitePackages != "" {
			env = appendOrMergeEnvPath(env, "PYTHONPATH", sitePackages)
		}
	}
	return env
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

func appendOrMergeEnvPath(env []string, key string, value string) []string {
	if strings.TrimSpace(value) == "" {
		return env
	}

	prefix := key + "="
	index := -1
	current := ""
	for i, item := range env {
		if strings.HasPrefix(item, prefix) {
			index = i
			current = strings.TrimPrefix(item, prefix)
			break
		}
	}

	parts := []string{}
	if strings.TrimSpace(current) != "" {
		parts = strings.Split(current, string(os.PathListSeparator))
	}
	for _, item := range parts {
		if item == value {
			return env
		}
	}

	var merged string
	if current == "" {
		merged = value
	} else {
		merged = current + string(os.PathListSeparator) + value
	}

	entry := prefix + merged
	if index >= 0 {
		env[index] = entry
		return env
	}
	return append(env, entry)
}
