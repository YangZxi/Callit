package db

import (
	"context"
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
	CronTask  *CronTaskDAO
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
	if err := enableIncrementalAutoVacuumForEmptyDatabase(gormDB); err != nil {
		return nil, err
	}
	if err := gormDB.Exec("PRAGMA foreign_keys = ON;").Error; err != nil {
		return nil, fmt.Errorf("设置 PRAGMA 失败: %w", err)
	}
	if err := runMigrations(gormDB); err != nil {
		return nil, fmt.Errorf("执行迁移失败: %w", err)
	}

	return newStore(gormDB), nil
}

func newStore(gormDB *gorm.DB) *Store {
	store := &Store{gormDB: gormDB}
	store.Worker = &WorkerDAO{db: gormDB}
	store.WorkerLog = &WorkerLogDAO{db: gormDB}
	store.CronTask = &CronTaskDAO{db: gormDB}
	store.AppConfig = &AppConfigDAO{db: gormDB}
	return store
}

func enableIncrementalAutoVacuumForEmptyDatabase(gormDB *gorm.DB) error {
	var schemaObjectCount int64
	if err := gormDB.Raw("SELECT COUNT(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'").Scan(&schemaObjectCount).Error; err != nil {
		return fmt.Errorf("检查数据库结构失败: %w", err)
	}
	if schemaObjectCount > 0 {
		return nil
	}
	if err := gormDB.Exec("PRAGMA auto_vacuum = INCREMENTAL;").Error; err != nil {
		return fmt.Errorf("设置 auto_vacuum 失败: %w", err)
	}
	return nil
}

func (s *Store) Transaction(ctx context.Context, fn func(*Store) error) error {
	return s.gormDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(newStore(tx))
	})
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
