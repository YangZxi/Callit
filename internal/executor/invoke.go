package executor

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"callit/internal/db"
	"callit/internal/model"
)

// Service 负责执行 Worker 并记录运行日志。
type Service struct {
	store   *db.Store
	dataDir string
}

func NewService(store *db.Store, dataDir string) *Service {
	return &Service{
		store:   store,
		dataDir: dataDir,
	}
}

func (s *Service) Execute(ctx context.Context, worker model.Worker, requestID string, input model.WorkerInput, asyncLog bool) ExecuteResult {
	workerDir := filepath.Join(s.dataDir, "workers", worker.ID)
	execResult := Run(ctx, worker, workerDir, input)
	s.recordRunningLog(worker.ID, requestID, input, execResult, asyncLog)
	return execResult
}

func (s *Service) recordRunningLog(workerID string, requestID string, input model.WorkerInput, execResult ExecuteResult, async bool) {
	entry := s.buildWorkerLog(workerID, requestID, input, execResult)
	if async {
		go func(logEntry model.WorkerLog) {
			persistCtx, persistCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer persistCancel()

			if err := s.store.WorkerLog.Insert(persistCtx, logEntry); err != nil {
				log.Printf("写入函数日志失败: %v", err)
			}
		}(entry)
		return
	}

	persistCtx, persistCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer persistCancel()
	if err := s.store.WorkerLog.Insert(persistCtx, entry); err != nil {
		log.Printf("写入函数日志失败: %v", err)
	}
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
		log.Printf("序列化 WorkerInput 失败: %v", err)
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
