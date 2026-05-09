# AGENTS.md

本文件定义 `callit` 项目的代理协作规范。所有代理在本仓库执行任务时必须遵守。
除非我主动提到使用 Worktree，否则请直接在本仓库执行修改代码。

## 1. 语言与沟通

1. 仅使用中文输出（分析、说明、注释、文档）。
2. 回答先给结论，再给关键细节。
3. 遇到不确定项，先在仓库内搜索确认，再提问。

## 2. 项目概览

`callit` 是一个轻量级、自建、基于 Docker 的个人 Serverless 平台。

- Backend: Go + Gin
- Frontend: React + Vite + HeroUI
- Database: SQLite3
- Runtime: Python3 / Node.js

## 3. 当前目录约定

```text
cmd/main.go                 # 程序入口，启动 Router/Admin 双服务
internal/admin              # admin 服务，负责项目所有管理功能
internal/router             # worker 的 http 路由定义
internal/executor           # http、cron 触发调用 worker 时的执行器
internal/common             # 公共通用模块，JSON/form/multipart 解析
internal/db                 # 数据库层，使用 ORM 提供操作数据的接口
internal/model              # 数据库实体与 Go 实体
resources/worker_sdk        # Worker SDK 源文件
pages/                      # 前端源代码
.github/workflows           # CI（镜像构建推送）
```

## 4. 运行与开发命令

### 4.1 本地运行

先构建前端：

```bash
cd pages
pnpm run build
cd ..
rm -rf public/*
cp -r pages/dist/* public/
```

再启动后端：

```bash
export ADMIN_TOKEN=your-token
go run ./cmd
```

前端开发模式：

```bash
cd pages
npm run dev
```

### 4.2 单元/集成测试

```bash
go test ./...
```

要求：单次测试执行尽量控制在 60 秒内。

### 4.3 Docker 运行

```bash
docker compose -f docker-compose.dev.yml up --build
```

## 5. 数据与文件约定

- DB: `data/app.db`
- WorkerDB: `data/worker.db`，Worker 通过 DB 接口(Callit SDK)所操作的数据库文件
- Worker 目录: `data/workers/<worker_id>/`
- 临时上传目录: `data/tmp/<request_id>/`
- multipart 上传文件在请求结束后必须清理（`defer RemoveAll`）

## 6. 核心契约（禁止随意破坏）
更详细的协议定义在 `docs/worker_introduction.md` 中，以下是核心契约：

### 6.1 脚本输入（STDIN）

模型名：`WorkerInput` / `WorkerRequest`

```json
{
  "request": {
    "method": "POST",
    "uri": "/api/test?name=callit",
    "url": "http://127.0.0.1:3100/api/test?name=callit",
    "headers": {},
    "body": {},
    "body_str": "raw string"
  }
}
```

### 6.2 脚本输出（stdout JSON）

模型名：`WorkerOutput`

```json
{
  "status": 200,
  "headers": {"X-Trace": "abc"},
  "body": {"ok": true}
}
```

- `status` 默认 200
- `headers` 可选
- 非法 JSON 或非法协议返回 500

### 6.3 统一错误结构

```json
{
  "error": "message",
  "request_id": "uuid"
}
```

## 7. 代码规范

1. 优先保证正确性与可读性，不保留无用兼容代码。
2. 修改功能时同步更新相关测试。
3. 关键流程允许添加简洁中文注释，避免注释噪音。
4. 新增路径、包名、模型名时，必须全局搜索引用并一次性更新。
5. 禁止使用无意义命名和难以理解的随意缩写；允许使用行业内约定俗成、语义明确的常见缩写（如 `cfg`、`reg`、`dao`），但所有变量名、函数名、方法名都必须名如其意，看到名称就能理解其职责和用途。
6. 避免做不需要的兜底开发和过度设计，一般情况下，你只需要考虑当下就行，不必考虑到未来的各种参数值不合法问题。在不明确时，询问我是否需要进行兜底。
7. 修改 Worker SDK 时，必须同步检查对应的 `magic-api` 接口契约、参数结构和返回结构是否仍然兼容，确保 SDK 封装与服务端行为一致。
8. 修改 Worker SDK 的逻辑、命名、返回值或修复 BUG 时，必须同时检查 Node 与 Python 两个 runtime 的实现和对外行为，避免出现跨 runtime 不一致。
9. 方法名、变量名等命名时，永远不要使用 normalize*** 等这种没有明确语义的名称。要不就和调用方代码写在一起，要不命名就应该精确到 build、parse、format 等具体行为。

## 8. 变更流程（代理执行）

1. 先搜索并阅读相关代码，再动手修改。
2. 修改后至少执行：
   - `gofmt -w`（仅针对改动文件）
   - `go test ./...`
3. 在结果说明中明确：改了什么、为什么改、如何验证。

## 9. 风险操作规范

以下操作必须先得到明确确认：

- 删除文件/目录、批量替换关键代码
- `git commit` / `git push`
- `git reset --hard`、覆盖式回滚
- 修改生产环境配置或敏感凭据处理方式

## 10. CI / 镜像

- CI 文件：`.github/workflows/build_and_push.yaml`
- 构建与推送在 GitHub Actions 云端执行
- 默认镜像目标：`ghcr.io/<owner>/callit`
- Python 运行时版本固定为 `3.10`。这是项目明确约束，不允许随意升级、降级或改为跟随基础镜像默认版本；如需调整，必须明确评估 `venv` 兼容性、依赖恢复路径和现有运行环境迁移方案。
- Node.js 在镜像中采用手动安装方式，而不是直接通过 `apt` 安装。这样做是为了避免 `nsjail` 沙箱内执行 `node` 时出现失败；除非已经验证新的安装方式不会影响沙箱执行，否则不要改回 `apt` 安装。

## 11. 完成定义（DoD）

任务完成前需满足：

1. 功能实现与需求一致。
2. 相关测试通过。
3. 无明显回归风险与路径残留（重命名场景尤其注意）。
4. 文档或示例（如影响使用方式）已同步更新。
