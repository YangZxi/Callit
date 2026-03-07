package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"callit/internal/model"

	_ "modernc.org/sqlite"
)

// Store 封装 SQLite 持久化操作。
type Store struct {
	db        *sql.DB
	AppConfig *AppConfigDao
}

// methods 字段保留在数据库中用于兼容历史表结构，不再参与业务逻辑。
const legacyMethodsValue = "*"

// Open 打开数据库并执行迁移。
func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("创建数据库目录失败: %w", err)
	}
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	if _, err = database.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		database.Close()
		return nil, fmt.Errorf("设置 PRAGMA 失败: %w", err)
	}
	if _, err = database.Exec(migrationSQL); err != nil {
		database.Close()
		return nil, fmt.Errorf("执行迁移失败: %w", err)
	}
	store := &Store{db: database}
	store.AppConfig = &AppConfigDao{store: database}
	return store, nil
}

// Close 关闭数据库连接。
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateWorker 创建函数记录。
func (s *Store) CreateWorker(ctx context.Context, fn model.Worker) (model.Worker, error) {
	if fn.TimeoutMS <= 0 {
		fn.TimeoutMS = 5000
	}

	query := `INSERT INTO worker(id, name, runtime, route, methods, timeout_ms, enabled)
VALUES(?,?,?,?,?,?,?)`
	_, err := s.db.ExecContext(ctx, query,
		fn.ID, fn.Name, fn.Runtime, fn.Route, legacyMethodsValue, fn.TimeoutMS, boolToInt(fn.Enabled),
	)
	if err != nil {
		return model.Worker{}, err
	}
	return s.GetWorkerByID(ctx, fn.ID)
}

// ListWorkers 列出全部函数。
func (s *Store) ListWorkers(ctx context.Context) ([]model.Worker, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, runtime, route, methods, timeout_ms, enabled, created_at, updated_at
FROM worker
ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]model.Worker, 0)
	for rows.Next() {
		fn, err := scanWorker(rows.Scan)
		if err != nil {
			return nil, err
		}
		result = append(result, fn)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// ListEnabledWorkers 列出启用函数。
func (s *Store) ListEnabledWorkers(ctx context.Context) ([]model.Worker, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, runtime, route, methods, timeout_ms, enabled, created_at, updated_at
FROM worker
WHERE enabled = 1
ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]model.Worker, 0)
	for rows.Next() {
		fn, err := scanWorker(rows.Scan)
		if err != nil {
			return nil, err
		}
		result = append(result, fn)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetWorkerByID 按 ID 获取函数。
func (s *Store) GetWorkerByID(ctx context.Context, id string) (model.Worker, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, runtime, route, methods, timeout_ms, enabled, created_at, updated_at
FROM worker
WHERE id = ?`, id)
	fn, err := scanWorker(row.Scan)
	if err != nil {
		return model.Worker{}, err
	}
	return fn, nil
}

// UpdateWorker 更新函数基础信息。
func (s *Store) UpdateWorker(ctx context.Context, fn model.Worker) (model.Worker, error) {
	if fn.TimeoutMS <= 0 {
		fn.TimeoutMS = 5000
	}

	result, err := s.db.ExecContext(ctx, `
UPDATE worker
SET name = ?, runtime = ?, route = ?, methods = ?, timeout_ms = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?`,
		fn.Name,
		fn.Runtime,
		fn.Route,
		legacyMethodsValue,
		fn.TimeoutMS,
		boolToInt(fn.Enabled),
		fn.ID,
	)
	if err != nil {
		return model.Worker{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return model.Worker{}, err
	}
	if affected == 0 {
		return model.Worker{}, sql.ErrNoRows
	}
	return s.GetWorkerByID(ctx, fn.ID)
}

// DeleteWorker 删除函数。
func (s *Store) DeleteWorker(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM worker WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetWorkerEnabled 更新函数启用状态。
func (s *Store) SetWorkerEnabled(ctx context.Context, id string, enabled bool) (model.Worker, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE worker SET enabled = ? WHERE id = ?`, boolToInt(enabled), id)
	if err != nil {
		return model.Worker{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return model.Worker{}, err
	}
	if affected == 0 {
		return model.Worker{}, sql.ErrNoRows
	}
	return s.GetWorkerByID(ctx, id)
}

// InsertWorkerLog 写入函数执行日志。
func (s *Store) InsertWorkerLog(ctx context.Context, log model.WorkerLog) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO execution_logs(id, worker_id, request_id, status, stdout, stderr, error, duration_ms)
VALUES(?,?,?,?,?,?,?,?)`,
		log.ID,
		log.WorkerID,
		log.RequestID,
		log.Status,
		log.Stdout,
		log.Stderr,
		log.Error,
		log.DurationMS,
	)
	return err
}

// ListWorkerLogsPaged 分页查询 Worker 执行日志。
func (s *Store) ListWorkerLogsPaged(ctx context.Context, workerID string, page int, pageSize int) ([]model.WorkerLog, int, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM execution_logs
WHERE worker_id = ?`, workerID).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	rows, err := s.db.QueryContext(ctx, `
SELECT id, worker_id, request_id, status, stdout, stderr, error, duration_ms, created_at
FROM execution_logs
WHERE worker_id = ?
ORDER BY created_at DESC, id DESC
LIMIT ? OFFSET ?`, workerID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	result := make([]model.WorkerLog, 0, pageSize)
	for rows.Next() {
		var item model.WorkerLog
		if err := rows.Scan(
			&item.ID,
			&item.WorkerID,
			&item.RequestID,
			&item.Status,
			&item.Stdout,
			&item.Stderr,
			&item.Error,
			&item.DurationMS,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

// IsNotFound 判断是否为未找到错误。
func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func scanWorker(scan func(dest ...any) error) (model.Worker, error) {
	var (
		fn         model.Worker
		methodsCSV string
		enabledInt int
	)
	if err := scan(
		&fn.ID,
		&fn.Name,
		&fn.Runtime,
		&fn.Route,
		&methodsCSV,
		&fn.TimeoutMS,
		&enabledInt,
		&fn.CreatedAt,
		&fn.UpdatedAt,
	); err != nil {
		return model.Worker{}, err
	}
	fn.Enabled = enabledInt == 1
	return fn, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
