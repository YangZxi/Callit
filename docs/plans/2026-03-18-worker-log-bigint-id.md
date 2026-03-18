# Worker Log Bigint ID Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将 `worker_run_log.id` 改为数据库自增的整数主键，并同步代码与前端类型。

**Architecture:** 不新增运行时迁移逻辑，默认由用户手动删除旧表后重新建表。后端将 `WorkerLog.ID` 改为 `int64` 自增主键，写入时不再生成 UUID，前端日志类型同步改为数字。

**Tech Stack:** Go, GORM, SQLite, React, TypeScript

---

### Task 1: 锁定数据库自增行为

**Files:**
- Create: `internal/db/worker_log_test.go`

**Step 1: Write the failing test**

编写数据库测试，插入两条 `WorkerLog`，断言查询结果中的 `id` 为正整数且后写入记录的 `id` 更大。

**Step 2: Run test to verify it fails**

Run: `go test ./internal/db -run TestWorkerLogInsertUsesAutoIncrementID -count=1`
Expected: FAIL，因为当前 `WorkerLog.ID` 是字符串主键。

### Task 2: 修改模型与建表定义

**Files:**
- Modify: `internal/model/worker_log.go`
- Modify: `migrations/001_init.sql`

**Step 1: Write minimal implementation**

将 `WorkerLog.ID` 改为 `int64` 并标记为自增主键；初始化 SQL 改为整数自增主键定义。

**Step 2: Run targeted test**

Run: `go test ./internal/db -run TestWorkerLogInsertUsesAutoIncrementID -count=1`
Expected: PASS

### Task 3: 移除手动分配 ID 并同步前端类型

**Files:**
- Modify: `internal/router/server.go`
- Modify: `pages/src/components/worker-log-list.tsx`
- Modify: `pages/src/pages/WorkerDetailPage.tsx`

**Step 1: Write minimal implementation**

删除日志写入时的 UUID 赋值；将前端 `worker_log.id`、展开状态集合和回调参数同步为数字类型。

**Step 2: Run full verification**

Run: `gofmt -w internal/model/worker_log.go internal/db/worker_log_test.go internal/router/server.go`
Run: `go test ./...`
Expected: PASS
