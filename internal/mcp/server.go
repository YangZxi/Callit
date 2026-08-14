package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	adminsvc "callit/internal/admin"
	"callit/internal/config"
	"callit/internal/db"
	"callit/internal/router"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type searchWorkersInput struct {
	Keyword string `json:"keyword,omitempty"`
}

type workerIDInput struct {
	WorkerID string `json:"worker_id"`
}

type workerFileInput struct {
	WorkerID string `json:"worker_id"`
	Filename string `json:"filename"`
}

type workerFileWriteInput struct {
	WorkerID string `json:"worker_id"`
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

type createWorkerInput struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Runtime     string `json:"runtime"`
	Route       string `json:"route"`
	TimeoutMS   int    `json:"timeout_ms"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

type updateWorkerInput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Route       string `json:"route"`
	TimeoutMS   int    `json:"timeout_ms"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

type searchWorkersOutput struct {
	Workers any `json:"workers"`
}

type listWorkerFilesOutput struct {
	Files []string `json:"files"`
}

type getWorkerFileOutput struct {
	File adminsvc.FileContent `json:"file"`
}

type saveWorkerFileOutput struct {
	Files []string `json:"files"`
}

type deleteWorkerFileOutput struct {
	OK bool `json:"ok"`
}

type workerOutput struct {
	Worker any `json:"worker"`
}

type Handler struct {
	cfg     *config.Config
	service *adminsvc.WorkerService
	server  *sdkmcp.Server
}

func NewHandler(store *db.Store, reg *router.Registry, cronReloader interface{ Reload(context.Context) error }, cfg *config.Config) http.Handler {
	workersDir := cfg.WorkersDir
	if strings.TrimSpace(workersDir) == "" {
		workersDir = filepath.Join(cfg.DataDir, "workers")
	}
	workerTmpDir := cfg.WorkerRunningTempDir
	if strings.TrimSpace(workerTmpDir) == "" {
		workerTmpDir = filepath.Join(cfg.DataDir, "tmp")
	}
	runtimeLibDir := cfg.RuntimeLibDir
	if strings.TrimSpace(runtimeLibDir) == "" {
		runtimeLibDir = filepath.Join(cfg.DataDir, ".lib")
	}
	h := &Handler{
		cfg:     cfg,
		service: adminsvc.NewWorkerService(store, reg, cronReloader, workersDir, workerTmpDir, runtimeLibDir, cfg.WorkerMaxDirDepth),
	}
	h.server = h.newSDKServer()

	streamableHandler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server {
		return h.server
	}, &sdkmcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	})

	mux := http.NewServeMux()
	mux.Handle("/mcp", h.authorize(streamableHandler))
	mux.Handle("/mcp/", h.authorize(streamableHandler))
	return mux
}

func (h *Handler) authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.cfg.AppConfig.MCP_Enable {
			http.NotFound(w, r)
			return
		}
		if !h.isAuthorized(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": "unauthorized",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) isAuthorized(r *http.Request) bool {
	token := strings.TrimSpace(h.cfg.AppConfig.MCP_Token)
	if token == "" {
		return false
	}

	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if authorization != "" {
		return isBearerTokenMatch(authorization, token)
	}

	return strings.TrimSpace(r.URL.Query().Get("token")) == token
}

func isBearerTokenMatch(authorization string, token string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(authorization, prefix) {
		return false
	}
	return strings.TrimSpace(strings.TrimPrefix(authorization, prefix)) == token
}

func (h *Handler) newSDKServer() *sdkmcp.Server {
	server := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "callit-mcp",
		Version: "1.0.0",
	}, &sdkmcp.ServerOptions{
		Capabilities: &sdkmcp.ServerCapabilities{
			Tools: &sdkmcp.ToolCapabilities{
				ListChanged: false,
			},
		},
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "create_worker",
		Description: "创建一个新的 Worker。参数必须提供 name、runtime、route、timeout_ms，可选 description、env、enabled；env 为环境变量字符串，多个变量按换行分隔，每行一个 KEY=value。description 最多 200 字符。route 必须以 / 开头且通配符只支持结尾 /*，创建成功后会自动生成对应 runtime 的入口文件。",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input createWorkerInput) (*sdkmcp.CallToolResult, workerOutput, error) {
		worker, err := h.service.CreateWorker(ctx, adminsvc.CreateWorkerInput{
			Name:        input.Name,
			Description: input.Description,
			Runtime:     input.Runtime,
			Route:       input.Route,
			TimeoutMS:   input.TimeoutMS,
			Enabled:     input.Enabled,
		})
		if err != nil {
			return nil, workerOutput{}, err
		}
		return nil, workerOutput{Worker: worker}, nil
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "update_worker",
		Description: "更新已有 Worker 的基础信息。参数必须提供 id、name、route、timeout_ms，可选 description、env、enabled；env 为环境变量字符串，多个变量按换行分隔，每行一个 KEY=value。description 最多 200 字符；只能更新 name、description、route、timeout_ms、env、enabled",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input updateWorkerInput) (*sdkmcp.CallToolResult, workerOutput, error) {
		worker, err := h.service.UpdateWorker(ctx, adminsvc.UpdateWorkerInput{
			ID:          input.ID,
			Name:        input.Name,
			Description: input.Description,
			Route:       input.Route,
			TimeoutMS:   input.TimeoutMS,
			Enabled:     input.Enabled,
		})
		if err != nil {
			return nil, workerOutput{}, err
		}
		return nil, workerOutput{Worker: worker}, nil
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "search_workers",
		Description: "按 Worker 名称模糊搜索并返回匹配列表。参数 keyword 为可选关键词，结果包含 id、name、description、runtime、route、timeout_ms、enabled 等基础信息。",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input searchWorkersInput) (*sdkmcp.CallToolResult, searchWorkersOutput, error) {
		workers, err := h.service.SearchWorkers(ctx, input.Keyword)
		if err != nil {
			return nil, searchWorkersOutput{}, err
		}
		return nil, searchWorkersOutput{Workers: workers}, nil
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "list_worker_files",
		Description: "递归列出指定 Worker 代码目录中的相对路径。参数必须提供 worker_id；文件以相对路径返回，目录以 / 结尾返回，Worker 代码根目录按第 1 层计算。",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input workerIDInput) (*sdkmcp.CallToolResult, listWorkerFilesOutput, error) {
		files, err := h.service.ListWorkerFiles(ctx, input.WorkerID)
		if err != nil {
			return nil, listWorkerFilesOutput{}, err
		}
		return nil, listWorkerFilesOutput{Files: files}, nil
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "get_worker_file",
		Description: "读取指定 Worker 文件内容。参数必须提供 worker_id 和 filename；filename 为 list_worker_files 中得到的文件相对路径，不能是以 / 结尾的目录，返回文本内容或图片预览信息。",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input workerFileInput) (*sdkmcp.CallToolResult, getWorkerFileOutput, error) {
		file, err := h.service.GetWorkerFile(ctx, input.WorkerID, input.Filename)
		if err != nil {
			return nil, getWorkerFileOutput{}, err
		}
		return nil, getWorkerFileOutput{File: file}, nil
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "upload_worker_file",
		Description: "向指定 Worker 写入文件或创建目录。参数必须提供 worker_id、filename、content；filename 以 / 结尾时创建目录并忽略 content，否则创建或覆盖相对路径文件；缺少的父目录会自动创建。路径开头的 / 和每个路径段两端空格会自动移除，非法格式会被拒绝；默认最多 3 层且可由服务端环境变量调整。",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input workerFileWriteInput) (*sdkmcp.CallToolResult, saveWorkerFileOutput, error) {
		files, err := h.service.SaveWorkerFileContent(ctx, input.WorkerID, input.Filename, input.Content)
		if err != nil {
			return nil, saveWorkerFileOutput{}, err
		}
		return nil, saveWorkerFileOutput{Files: files}, nil
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "update_worker_file",
		Description: "更新指定 Worker 文件内容或创建目录。参数必须提供 worker_id、filename、content；filename 以 / 结尾时创建目录并忽略 content，否则直接覆盖相对路径文件，文件或父目录不存在时会自动创建。路径开头的 / 和每个路径段两端空格会自动移除，非法格式会被拒绝；默认最多 3 层且可由服务端环境变量调整。",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input workerFileWriteInput) (*sdkmcp.CallToolResult, saveWorkerFileOutput, error) {
		files, err := h.service.SaveWorkerFileContent(ctx, input.WorkerID, input.Filename, input.Content)
		if err != nil {
			return nil, saveWorkerFileOutput{}, err
		}
		return nil, saveWorkerFileOutput{Files: files}, nil
	})

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "delete_worker_file",
		Description: "删除指定 Worker 文件或目录。参数必须提供 worker_id 和 filename；filename 应来自 list_worker_files，目录路径以 / 结尾。删除目录会递归删除其中全部文件和子目录，执行前必须确认；不能删除 Worker 根入口文件 main.py 或 main.js。",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input workerFileInput) (*sdkmcp.CallToolResult, deleteWorkerFileOutput, error) {
		if err := h.service.DeleteWorkerFile(ctx, input.WorkerID, input.Filename); err != nil {
			return nil, deleteWorkerFileOutput{}, err
		}
		return nil, deleteWorkerFileOutput{OK: true}, nil
	})

	return server
}
