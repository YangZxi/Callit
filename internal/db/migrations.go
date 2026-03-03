package db

const migrationSQL = `
CREATE TABLE IF NOT EXISTS worker (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    runtime TEXT NOT NULL CHECK(runtime IN ('python','node')),
    route TEXT NOT NULL UNIQUE,
    methods TEXT NOT NULL,
    timeout_ms INTEGER NOT NULL DEFAULT 5000,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER IF NOT EXISTS worker_updated_at_trigger
AFTER UPDATE ON worker
FOR EACH ROW
BEGIN
    UPDATE worker SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
END;

CREATE TABLE IF NOT EXISTS execution_logs (
    id TEXT PRIMARY KEY,
    worker_id TEXT NOT NULL,
    request_id TEXT NOT NULL,
    status INTEGER,
    stdout TEXT,
    stderr TEXT,
    error TEXT,
    duration_ms INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_execution_logs_worker_created
ON execution_logs(worker_id, created_at);

CREATE INDEX IF NOT EXISTS idx_execution_logs_request_id
ON execution_logs(request_id);
`
