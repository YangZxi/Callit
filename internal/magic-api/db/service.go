package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

const sqliteBusyTimeoutMS = 5000

type Result struct {
	Rows         []map[string]any `json:"rows"`
	RowsAffected int64            `json:"rows_affected"`
	LastInsertID int64            `json:"last_insert_id"`
}

type Service struct {
	db *sql.DB
}

func OpenSQLiteService(dbPath string) (*Service, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("创建 worker.db 目录失败: %w", err)
	}

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开 worker.db 失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	service := &Service{db: sqlDB}
	if err := service.initialize(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return service, nil
}

func (s *Service) initialize() error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		fmt.Sprintf("PRAGMA busy_timeout = %d;", sqliteBusyTimeoutMS),
		"PRAGMA foreign_keys = OFF;",
	}
	for _, pragmaSQL := range pragmas {
		if _, err := s.db.Exec(pragmaSQL); err != nil {
			return fmt.Errorf("初始化 worker.db 失败: %w", err)
		}
	}
	return nil
}

func (s *Service) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Service) Exec(ctx context.Context, sqlText string, args []any) (Result, error) {
	if s == nil || s.db == nil {
		return Result{}, fmt.Errorf("worker db service unavailable")
	}

	if isQuerySQL(sqlText) {
		return s.query(ctx, sqlText, args)
	}
	return s.exec(ctx, sqlText, args)
}

func isQuerySQL(sqlText string) bool {
	firstToken := strings.ToLower(firstSQLToken(sqlText))
	switch firstToken {
	case "select", "pragma", "with", "explain":
		return true
	default:
		return false
	}
}

func firstSQLToken(sqlText string) string {
	trimmed := strings.TrimSpace(sqlText)
	if trimmed == "" {
		return ""
	}
	for index, r := range trimmed {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
			return trimmed[:index]
		}
	}
	return trimmed
}

func (s *Service) query(ctx context.Context, sqlText string, args []any) (Result, error) {
	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return Result{}, err
	}

	result := Result{Rows: make([]map[string]any, 0)}
	for rows.Next() {
		scanTargets := make([]any, len(columns))
		scanValues := make([]any, len(columns))
		for i := range columns {
			scanTargets[i] = &scanValues[i]
		}
		if err := rows.Scan(scanTargets...); err != nil {
			return Result{}, err
		}

		row := make(map[string]any, len(columns))
		for i, column := range columns {
			row[column] = normalizeScannedValue(scanValues[i])
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (s *Service) exec(ctx context.Context, sqlText string, args []any) (Result, error) {
	execResult, err := s.db.ExecContext(ctx, sqlText, args...)
	if err != nil {
		return Result{}, err
	}

	rowsAffected, err := execResult.RowsAffected()
	if err != nil {
		return Result{}, err
	}
	lastInsertID, err := execResult.LastInsertId()
	if err != nil {
		lastInsertID = 0
	}

	return Result{
		Rows:         []map[string]any{},
		RowsAffected: rowsAffected,
		LastInsertID: lastInsertID,
	}, nil
}

func normalizeScannedValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	case time.Time:
		return typed.Format(time.RFC3339Nano)
	default:
		return typed
	}
}
