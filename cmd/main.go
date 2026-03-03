package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"callit/internal/admin"
	"callit/internal/config"
	"callit/internal/db"
	"callit/internal/registry"
	"callit/internal/router"

	"golang.org/x/sync/errgroup"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	if err := ensureRuntimeDirs(cfg.DataDir); err != nil {
		log.Fatalf("创建运行目录失败: %v", err)
	}

	store, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	defer store.Close()

	reg := registry.New()
	funcs, err := store.ListEnabledWorkers(context.Background())
	if err != nil {
		log.Fatalf("加载启用函数失败: %v", err)
	}
	reg.Reload(funcs)

	routerEngine := router.NewEngine(store, reg, cfg.DataDir)
	adminEngine := admin.NewEngine(store, reg, cfg.DataDir, cfg.AdminToken, cfg.AI)

	routerSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.RouterPort),
		Handler:           routerEngine,
		ReadHeaderTimeout: 5 * time.Second,
	}
	adminSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.AdminPort),
		Handler:           adminEngine,
		ReadHeaderTimeout: 5 * time.Second,
	}

	localIP := resolveLocalIPv4()
	log.Printf("Router 服务启动: http://%s:%d", localIP, cfg.RouterPort)
	log.Printf("Admin 服务启动: http://%s:%d", localIP, cfg.AdminPort)

	group, gctx := errgroup.WithContext(context.Background())
	group.Go(func() error {
		if err := routerSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("router 服务异常: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		if err := adminSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("admin 服务异常: %w", err)
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
			log.Printf("收到退出信号: %s", sig.String())
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := routerSrv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("关闭 router 失败: %w", err)
		}
		if err := adminSrv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("关闭 admin 失败: %w", err)
		}
		return nil
	})

	if err := group.Wait(); err != nil {
		log.Fatalf("服务退出异常: %v", err)
	}
	log.Println("服务已退出")
}

func ensureRuntimeDirs(dataDir string) error {
	dirs := []string{
		dataDir,
		filepath.Join(dataDir, ".lib"),
		filepath.Join(dataDir, "workers"),
		filepath.Join(dataDir, "temps"),
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
