package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"callit/internal/config"
	"callit/internal/db"
	"callit/internal/model"
	workerpkg "callit/internal/worker"
)

// Service 负责执行 Worker 并记录运行日志。
type Service struct {
	store             *db.Store
	workersDir        string
	workerTempBaseDir string
	runtimeLibDir     string
	cfg               config.Config
}

func NewService(store *db.Store, cfg config.Config) *Service {
	return &Service{
		store:             store,
		workersDir:        cfg.WorkersDir,
		workerTempBaseDir: cfg.WorkerRunningTempDir,
		runtimeLibDir:     cfg.RuntimeLibDir,
		cfg:               cfg,
	}
}

func (s *Service) WorkerRunningTempDir() string {
	if s == nil {
		return ""
	}
	return s.workerTempBaseDir
}

func (s *Service) Execute(ctx context.Context, worker model.Worker, requestID string, workerTmpDir string, input model.WorkerInput) ExecuteResult {
	spec, err := workerpkg.NewRuntimeWorkerSpec(s.workersDir, s.workerTempBaseDir, s.runtimeLibDir, worker, requestID)
	if err != nil {
		return ExecuteResult{Err: err}
	}
	if strings.TrimSpace(workerTmpDir) != "" {
		spec.WorkerTempDir = workerTmpDir
	}
	execResult := Run(ctx, spec, s.cfg, input)
	if execResult.Err != nil {
		slog.Warn(fmt.Sprintf("Worker 执行失败[%s]", input.Event.Trigger), "request_id", requestID, "worker_id", worker.ID, "duration_ms", execResult.DurationMS, "err", execResult.Err)
	} else {
		slog.Debug(fmt.Sprintf("Worker 执行成功[%s]", input.Event.Trigger), "request_id", requestID, "worker_id", worker.ID, "duration_ms", execResult.DurationMS, "status", execResult.Status)
	}
	s.recordRunningLog(worker.ID, requestID, input, execResult)
	return execResult
}

func CreateWorkerRunningTempDir(baseDir string, requestID string) (string, func(), error) {
	workerTmpDir := filepath.Join(baseDir, requestID)
	cleanup := func() {}
	if workerTmpDir != "" {
		if err := os.MkdirAll(workerTmpDir, 0o755); err != nil {
			slog.Error("创建运行时目录失败", "request_id", requestID, "path", workerTmpDir, "err", err)
			return "", cleanup, err
		}
		slog.Debug("创建运行时目录", "request_id", requestID, "path", workerTmpDir)
	}
	cleanup = func() {
		if rmErr := os.RemoveAll(workerTmpDir); rmErr != nil {
			slog.Warn("删除请求临时目录失败", "path", workerTmpDir, "err", rmErr)
		}
	}
	return workerTmpDir, cleanup, nil
}

func (s *Service) recordRunningLog(workerID string, requestID string, input model.WorkerInput, execResult ExecuteResult) {
	if s == nil || s.store == nil || s.store.WorkerLog == nil {
		return
	}
	entry := s.buildWorkerLog(workerID, requestID, input, execResult)
	go func(logEntry model.WorkerLog) {
		persistCtx, persistCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer persistCancel()

		if err := s.store.WorkerLog.Insert(persistCtx, logEntry); err != nil {
			slog.Error("异步写入函数日志失败", "request_id", logEntry.RequestID, "worker_id", logEntry.WorkerID, "err", err)
		}
	}(entry)
}

func (s *Service) buildWorkerLog(workerID string, requestID string, input model.WorkerInput, execResult ExecuteResult) model.WorkerLog {
	statusForLog := execResult.Status
	if statusForLog == 0 {
		if execResult.TimedOut {
			statusForLog = http.StatusGatewayTimeout
		} else {
			statusForLog = http.StatusInternalServerError
		}
	}

	errMsg := ""
	if execResult.Err != nil {
		if execResult.TimedOut {
			errMsg = "timeout"
		} else {
			errMsg = execResult.Err.Error()
		}
	}

	stdinText := ""
	payload, err := json.Marshal(input)
	if err != nil {
		slog.Error("序列化 WorkerInput 失败", "request_id", requestID, "worker_id", workerID, "err", err)
	} else {
		stdinText = string(payload)
	}

	trigger := input.Event.Trigger
	if trigger == "" {
		trigger = model.WorkerTriggerHTTP
	}

	return model.WorkerLog{
		WorkerID:   workerID,
		RequestID:  requestID,
		Trigger:    trigger,
		Status:     statusForLog,
		Stdin:      stdinText,
		Stdout:     execResult.Stdout,
		Stderr:     execResult.Stderr,
		Result:     execResult.Result,
		Error:      errMsg,
		DurationMS: execResult.DurationMS,
	}
}
