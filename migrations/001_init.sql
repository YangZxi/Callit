CREATE TABLE worker (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    runtime TEXT NOT NULL CHECK(runtime IN ('python','node')),
    route TEXT NOT NULL UNIQUE,
    timeout_ms INTEGER NOT NULL DEFAULT 5000,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE worker_run_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    worker_id TEXT NOT NULL,
    request_id TEXT NOT NULL,
    trigger TEXT NOT NULL DEFAULT 'http',
    status INTEGER,
    stdin TEXT,
    stdout TEXT,
    stderr TEXT,
    result TEXT,
    error TEXT,
    duration_ms INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_worker_run_log_worker_created
ON worker_run_log(worker_id, created_at);

CREATE INDEX idx_worker_run_log_request_id
ON worker_run_log(request_id);

CREATE TABLE cron_task (
    id INTEGER PRIMARY KEY,
    cron TEXT NOT NULL,
    worker_id TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_cron_task_worker_id
ON cron_task(worker_id);
