package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Store 聚合各模型对应的 DAO。
type Store struct {
	gormDB    *gorm.DB
	Worker    *WorkerDAO
	WorkerLog *WorkerLogDAO
	AppConfig *AppConfigDAO
}

// Open 打开数据库并初始化各 DAO。
func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("创建数据库目录失败: %w", err)
	}

	gormDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	if err := gormDB.Exec("PRAGMA foreign_keys = ON;").Error; err != nil {
		return nil, fmt.Errorf("设置 PRAGMA 失败: %w", err)
	}
	if err := runMigrations(gormDB); err != nil {
		return nil, fmt.Errorf("执行迁移失败: %w", err)
	}

	store := &Store{gormDB: gormDB}
	store.Worker = &WorkerDAO{db: gormDB}
	store.WorkerLog = &WorkerLogDAO{db: gormDB}
	store.AppConfig = &AppConfigDAO{db: gormDB}
	return store, nil
}

// Close 关闭数据库连接。
func (s *Store) Close() error {
	if s == nil || s.gormDB == nil {
		return nil
	}
	sqlDB, err := s.gormDB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// IsNotFound 判断是否为未找到错误。
func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || errors.Is(err, gorm.ErrRecordNotFound)
}

func notFoundError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return sql.ErrNoRows
	}
	return err
}
