package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"callit/internal/admin"
	"callit/internal/config"
	"callit/internal/cron"
	"callit/internal/db"
	"callit/internal/executor"
	"callit/internal/mcp"
	"callit/internal/router"

	"golang.org/x/sync/errgroup"
)

func main() {
	cfg := config.Load()
	setupLogger(cfg.LogLevel)
	if err := ensureRuntimeDirs(cfg); err != nil {
		slog.Error("初始化运行目录失败", "err", err)
		os.Exit(1)
	}
	slog.Info("项目启动配置：", "config", cfg)

	store, err := db.Open(cfg.DatabasePath)
	if err != nil {
		slog.Error("初始化数据库失败", "err", err)
		os.Exit(1)
	}
	defer store.Close()
	if err := cfg.Sync(context.Background(), store.AppConfig); err != nil {
		slog.Error("加载应用配置失败", "err", err)
		os.Exit(1)
	}

	reg := router.New()
	funcs, err := store.Worker.ListEnabled(context.Background())
	if err != nil {
		slog.Error("加载已启用的 Worker 失败", "err", err)
		os.Exit(1)
	}
	reg.Reload(funcs)

	invoker := executor.NewService(store, cfg)
	cronManager := cron.NewManager(store, invoker, time.Local)
	if err := cronManager.Start(context.Background()); err != nil {
		slog.Error("启动 cron 调度器失败", "err", err)
		os.Exit(1)
	}

	routerEngine := router.NewEngine(store, reg, cfg.WorkersDir, invoker)
	adminEngine := admin.NewEngine(store, reg, cronManager, &cfg)
	var mcpHandler http.Handler
	if cfg.AppConfig.MCP_Enable {
		mcpHandler = mcp.NewHandler(store, reg, cronManager, &cfg)
	}
	handler := serverRouteHandler(adminEngine, mcpHandler, routerEngine, cfg.AdminPrefix, cfg.AppConfig.MCP_Enable)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.ServerPort),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	localIP := resolveLocalIPv4()
	slog.Info("服务启动", "url", fmt.Sprintf("http://%s:%d", localIP, cfg.ServerPort))
	slog.Info("Admin 服务入口", "url", fmt.Sprintf("http://%s:%d%s", localIP, cfg.ServerPort, cfg.AdminPrefix))
	slog.Info("AdminToken", "token", cfg.AdminToken)
	if cfg.AppConfig.MCP_Enable {
		slog.Info("MCP 服务入口", "url", fmt.Sprintf("http://%s:%d/mcp", localIP, cfg.ServerPort))
	}

	group, gctx := errgroup.WithContext(context.Background())
	group.Go(func() error {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("服务异常: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigCh)

		select {
		case <-gctx.Done():
			return nil
		case sig := <-sigCh:
			slog.Warn("收到退出信号", "signal", sig.String())
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("关闭服务失败: %w", err)
		}
		return nil
	})

	if err := group.Wait(); err != nil {
		slog.Error("服务退出异常", "err", err)
		os.Exit(1)
	}
	slog.Info("服务已退出")
}

func setupLogger(levelRaw string) {
	level := new(slog.LevelVar)
	level.Set(parseLogLevel(levelRaw))
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
}

func parseLogLevel(levelRaw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(levelRaw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func ensureRuntimeDirs(cfg config.Config) error {
	dirs := []string{
		cfg.DataDir,
		cfg.WorkersDir,
		cfg.WorkerRunningTempDir,
		cfg.RuntimeLibDir,
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func resolveLocalIPv4() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP == nil || ipNet.IP.IsLoopback() {
			continue
		}
		ip4 := ipNet.IP.To4()
		if ip4 == nil {
			continue
		}
		return ip4.String()
	}
	return "127.0.0.1"
}

func serverRouteHandler(adminHandler http.Handler, mcpHandler http.Handler, routerHandler http.Handler, adminPrefix string, mcpEnabled bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAdminPath(r.URL.Path, adminPrefix) {
			adminHandler.ServeHTTP(w, r)
			return
		}
		if mcpEnabled && mcpHandler != nil && isMCPPath(r.URL.Path) {
			mcpHandler.ServeHTTP(w, r)
			return
		}
		routerHandler.ServeHTTP(w, r)
	})
}

func isAdminPath(path string, adminPrefix string) bool {
	flag := path == adminPrefix || path == adminPrefix+"/" || strings.HasPrefix(path, adminPrefix+"/")
	return flag || strings.HasPrefix(path, "/admin/assets")
}

func isMCPPath(path string) bool {
	return path == "/mcp" || path == "/mcp/" || strings.HasPrefix(path, "/mcp/")
}
